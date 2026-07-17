package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (e *Engine) Deploy(ctx context.Context, domain, branch string) error {
	if err := e.locks.Acquire(domain, "deploying"); err != nil {
		return err
	}
	defer e.locks.Release(domain)

	// A deploy pulls images and recreates containers — longer than a browser or
	// proxy holds an idle connection, and recreating the panel's own container
	// drops the request connection outright. Detach so a started deploy always
	// finishes instead of being cancelled half-way, leaving the site broken.
	ctx, cancel := detachCtx(ctx, 15*time.Minute)
	defer cancel()

	site, err := e.db.GetSite(domain)
	if err != nil {
		return fmt.Errorf("get site: %w", err)
	}

	if branch == "" {
		branch = site.Branch
		if branch == "" {
			branch = "main"
		}
	}
	if err := ValidateBranch(branch); err != nil {
		return err
	}
	if err := ValidateRepo(site.Repo); err != nil {
		return err
	}

	driver, err := e.drivers.Load(site.Driver)
	if err != nil {
		return fmt.Errorf("load driver: %w", err)
	}
	// Expand ${variables} in the freshly-loaded driver (deploy hooks, etc.) with
	// this site's values, so hooks can reference the domain, db creds and paths.
	ExpandDriverVariables(driver, e.siteVars(site))

	// Auto-backup before deploy (non-blocking — log warning on failure). Use the
	// already-locked variant: the deploy holds the lock for its whole duration so
	// nothing else can interleave. (The previous release/re-acquire dance could
	// hand the lock to another operation mid-deploy and then release *its* lock.)
	backupID, err := e.createBackup(ctx, domain, "")
	if err != nil {
		e.LogActivity(domain, "deploy_backup", fmt.Sprintf("pre-deploy backup failed: %v", err), "warning")
	} else {
		e.LogActivity(domain, "deploy_backup", fmt.Sprintf("pre-deploy backup #%d created", backupID), "success")
	}

	// Record deployment
	siteRoot, _ := e.SiteDir(site.Owner, domain)

	// Git pull
	var commitHash string
	if site.Repo != "" {
		// Re-check egress at deploy time (guards against DNS rebinding since
		// create) and apply the same transport/redirect hardening to fetch.
		if err := validateRepoEgress(site.Repo); err != nil {
			return Invalid("repository host is not allowed: %v", err)
		}
		isRepo := false
		if _, statErr := os.Stat(filepath.Join(siteRoot, ".git")); statErr == nil {
			isRepo = true
		}
		fetchArgs := append(gitHardeningArgs(), "-C", siteRoot, "fetch", "origin")
		fetchErr := exec.CommandContext(ctx, "git", fetchArgs...).Run()

		if fetchErr != nil && isRepo {
			// An existing checkout but the fetch failed (network/auth). Refuse the
			// deploy rather than (a) resetting to a stale cached ref or (b) wiping
			// a working tree we can't yet replace — either would be worse than
			// surfacing the transient error.
			e.LogActivity(domain, "deploy", "branch="+branch, "failed: git fetch error")
			return fmt.Errorf("git fetch failed; refusing to deploy stale code: %w", fetchErr)
		}

		var resetErr error
		if fetchErr == nil {
			// No "--" here: `git reset --hard -- <ref>` treats the ref as a
			// pathspec and always fails ("Cannot do hard reset with paths"),
			// which used to shunt EVERY redeploy into the rm -rf + re-clone
			// fallback below — destroying the site's .env. The ref is safe to
			// pass positionally: ValidateBranch rejects leading dashes.
			resetErr = exec.CommandContext(ctx, "git", "-C", siteRoot, "reset", "--hard", "origin/"+branch).Run()
		}
		// Not a git repo yet (first deploy), or the reset failed (corrupt repo):
		// do a fresh clone.
		if !isRepo || resetErr != nil {
			exec.CommandContext(ctx, "rm", "-rf", siteRoot).Run()
			args := append(gitHardeningArgs(), "clone", "--branch", branch, "--", site.Repo, siteRoot)
			if err := exec.CommandContext(ctx, "git", args...).Run(); err != nil {
				e.LogActivity(domain, "deploy", "branch="+branch, "failed: git clone error")
				return fmt.Errorf("git clone: %w", err)
			}
		}
		// Resolve the deployed commit. Refuse to record a deployment with an
		// unknown commit — an empty hash makes a later rollback a silent no-op.
		out, err := exec.CommandContext(ctx, "git", "-C", siteRoot, "rev-parse", "HEAD").Output()
		if err != nil {
			return fmt.Errorf("resolve deployed commit: %w", err)
		}
		commitHash = strings.TrimSpace(string(out))
		if commitHash == "" {
			return fmt.Errorf("resolve deployed commit: empty HEAD")
		}
	}

	// Create deployment record
	depID, _ := e.db.CreateDeployment(domain, commitHash, branch)

	// Restart containers BEFORE running hooks. The git update above may have
	// re-created siteRoot (the non-fast-forward path does rm -rf + clone), which
	// orphans the containers' bind mount of it — Linux binds pin the inode, not
	// the path, so the containers would otherwise see an empty /app. Restarting
	// re-binds the mount to the fresh code so before_deploy (composer install,
	// etc.) actually sees it.
	ids, _ := e.docker.ListContainersByLabel(ctx, labelPrefix+"site", domain)
	for _, id := range ids {
		e.docker.StopContainer(ctx, id)
		e.docker.StartContainer(ctx, id)
	}

	// Run before_deploy hooks
	containerName := containerNameFor(domain, primaryServiceName(driver))
	for _, hook := range driver.Deploy.BeforeDeploy {
		_, err := e.docker.ExecInContainer(ctx, containerName, []string{"sh", "-c", hook})
		if err != nil {
			e.db.UpdateDeploymentStatus(depID, "failed")
			e.LogActivity(domain, "deploy", "hook failed: "+hook, "failed")
			return fmt.Errorf("before_deploy hook %q: %w", hook, err)
		}
	}

	// Run after_deploy hooks
	for _, hook := range driver.Deploy.AfterDeploy {
		_, err := e.docker.ExecInContainer(ctx, containerName, []string{"sh", "-c", hook})
		if err != nil {
			e.LogActivity(domain, "deploy", "after_deploy hook failed: "+hook, "warning")
		}
	}

	e.db.UpdateDeploymentStatus(depID, "success")
	e.LogActivity(domain, "deploy", fmt.Sprintf("branch=%s commit=%s", branch, commitHash), "success")
	return nil
}

func (e *Engine) Rollback(ctx context.Context, domain string) error {
	if err := e.locks.Acquire(domain, "rolling back"); err != nil {
		return err
	}
	defer e.locks.Release(domain)

	// Mutating op: resets the working tree and bounces containers, so it must
	// complete even if the requesting client disconnects mid-way.
	ctx, cancel := detachCtx(ctx, 5*time.Minute)
	defer cancel()

	dep, err := e.db.GetLatestDeployment(domain)
	if err != nil {
		return NotFound("no deployment to rollback: %v", err)
	}

	// A deployment record can outlive its site (e.g. the site was destroyed);
	// GetSite returns (nil, err) for a missing row, so check it before deref.
	site, err := e.db.GetSite(domain)
	if err != nil || site == nil {
		return NotFound("site %q not found", domain)
	}
	siteRoot, _ := e.SiteDir(site.Owner, domain)

	// Rollback git to previous commit
	if dep.CommitHash != "" {
		cmd := exec.CommandContext(ctx, "git", "-C", siteRoot, "reset", "--hard", "HEAD~1")
		if err := cmd.Run(); err != nil {
			return Invalid("could not roll back (no previous deployment to revert to): %v", err)
		}
	}

	// Restart containers
	ids, _ := e.docker.ListContainersByLabel(ctx, labelPrefix+"site", domain)
	for _, id := range ids {
		e.docker.StopContainer(ctx, id)
		e.docker.StartContainer(ctx, id)
	}

	e.db.UpdateDeploymentStatus(dep.ID, "rolled_back")
	e.LogActivity(domain, "rollback", fmt.Sprintf("rolled back from %s", dep.CommitHash), "success")
	return nil
}

func (e *Engine) ListDeployments(ctx context.Context, domain string) (interface{}, error) {
	return e.db.ListDeployments(domain)
}
