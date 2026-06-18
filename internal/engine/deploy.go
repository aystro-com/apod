package engine

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func (e *Engine) Deploy(ctx context.Context, domain, branch string) error {
	if err := e.locks.Acquire(domain); err != nil {
		return err
	}
	defer e.locks.Release(domain)

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

	// Auto-backup before deploy (non-blocking — log warning on failure)
	e.locks.Release(domain) // release lock temporarily for backup
	backupID, err := e.CreateBackup(ctx, domain, "")
	if err != nil {
		e.LogActivity(domain, "deploy_backup", fmt.Sprintf("pre-deploy backup failed: %v", err), "warning")
	} else {
		e.LogActivity(domain, "deploy_backup", fmt.Sprintf("pre-deploy backup #%d created", backupID), "success")
	}
	if err := e.locks.Acquire(domain); err != nil {
		return err
	}

	// Record deployment
	siteRoot, _ := e.SiteDir(site.Owner, domain)

	// Git pull
	var commitHash string
	if site.Repo != "" {
		cmd := exec.CommandContext(ctx, "git", "-C", siteRoot, "fetch", "origin")
		cmd.Run()
		cmd = exec.CommandContext(ctx, "git", "-C", siteRoot, "reset", "--hard", "--", "origin/"+branch)
		if err := cmd.Run(); err != nil {
			// Maybe it's not a git repo yet, try clone
			exec.CommandContext(ctx, "rm", "-rf", siteRoot).Run()
			args := append(gitHardeningArgs(), "clone", "--branch", branch, "--", site.Repo, siteRoot)
			cmd = exec.CommandContext(ctx, "git", args...)
			if err := cmd.Run(); err != nil {
				e.LogActivity(domain, "deploy", "branch="+branch, "failed: git clone error")
				return fmt.Errorf("git clone: %w", err)
			}
		}
		// Get commit hash
		out, _ := exec.CommandContext(ctx, "git", "-C", siteRoot, "rev-parse", "HEAD").Output()
		commitHash = strings.TrimSpace(string(out))
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
	containerName := fmt.Sprintf("apod-%s-app", domain)
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
	if err := e.locks.Acquire(domain); err != nil {
		return err
	}
	defer e.locks.Release(domain)

	dep, err := e.db.GetLatestDeployment(domain)
	if err != nil {
		return NotFound("no deployment to rollback: %v", err)
	}

	site, _ := e.db.GetSite(domain)
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
	_ = site
	return nil
}

func (e *Engine) ListDeployments(ctx context.Context, domain string) (interface{}, error) {
	return e.db.ListDeployments(domain)
}
