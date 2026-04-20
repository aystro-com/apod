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

	driver, err := e.drivers.Load(site.Driver)
	if err != nil {
		return fmt.Errorf("load driver: %w", err)
	}

	// Record deployment
	siteRoot := fmt.Sprintf("%s/sites/%s/files", e.dataDir, domain)

	// Git pull
	var commitHash string
	if site.Repo != "" {
		cmd := exec.CommandContext(ctx, "git", "-C", siteRoot, "fetch", "origin")
		cmd.Run()
		cmd = exec.CommandContext(ctx, "git", "-C", siteRoot, "reset", "--hard", "origin/"+branch)
		if err := cmd.Run(); err != nil {
			// Maybe it's not a git repo yet, try clone
			exec.CommandContext(ctx, "rm", "-rf", siteRoot).Run()
			cmd = exec.CommandContext(ctx, "git", "clone", "--branch", branch, site.Repo, siteRoot)
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

	// Restart containers (simple deploy — stop/start)
	ids, _ := e.docker.ListContainersByLabel(ctx, labelPrefix+"site", domain)
	for _, id := range ids {
		e.docker.StopContainer(ctx, id)
		e.docker.StartContainer(ctx, id)
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
		return fmt.Errorf("no deployment to rollback: %w", err)
	}

	site, _ := e.db.GetSite(domain)
	siteRoot := fmt.Sprintf("%s/sites/%s/files", e.dataDir, domain)

	// Rollback git to previous commit
	if dep.CommitHash != "" {
		cmd := exec.CommandContext(ctx, "git", "-C", siteRoot, "reset", "--hard", "HEAD~1")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git rollback: %w", err)
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
