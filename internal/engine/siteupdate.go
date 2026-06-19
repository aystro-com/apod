package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aystro/apod/internal/models"
)

// UpdateSite pulls the latest image(s) for a site and recreates its containers
// so it runs the newest published version — the equivalent of "redeploy from
// latest" for image-based sites (e.g. apod-ui, or any compose app on a moving
// tag). Code-from-git deploys still go through Deploy; this refreshes images.
//
// For compose sites: `docker compose pull` then `up -d` (recreates only the
// services whose image changed). For native driver sites: each service image is
// pulled and its containers recreated in place, preserving name, labels, env
// (and the secrets baked into it), mounts, and resource limits.
func (e *Engine) UpdateSite(ctx context.Context, domain string) error {
	if err := e.locks.Acquire(domain, "updating"); err != nil {
		return err
	}
	defer e.locks.Release(domain)

	// Updating can recreate the very container serving this request (the apod-ui
	// panel updating itself): the client connection drops the moment we stop it,
	// cancelling the request context. Detach so the update runs to completion.
	ctx, cancel := detachCtx(ctx, 15*time.Minute)
	defer cancel()

	site, err := e.db.GetSite(domain)
	if err != nil {
		return fmt.Errorf("get site: %w", err)
	}
	if site == nil {
		return NotFound("site %q not found", domain)
	}
	driver, err := e.drivers.Load(site.Driver)
	if err != nil {
		return fmt.Errorf("load driver: %w", err)
	}
	// Expand ${variables} (e.g. ${odoo_version}) with this site's values, the
	// same as CreateSite — otherwise we'd try to pull a literal "odoo:${...}".
	ExpandDriverVariables(driver, e.siteVars(site))

	e.beginDeploy(domain, "Update")
	e.emitProgress(domain, "Pulling latest images", "running", "", 10)

	if driver.Type == "compose" {
		err = e.updateComposeSite(ctx, domain, site.Owner)
	} else {
		err = e.updateNativeSite(ctx, domain, driver)
	}
	if err != nil {
		e.emitProgress(domain, "Update failed", "error", sanitizeProgressLine(firstLine(err.Error())), 0)
		e.LogActivity(domain, "update", "image update failed", "failed")
		return err
	}

	// Recreated containers are new — re-attach them to any shared networks.
	e.reconnectSharedNetworks(ctx, domain)

	e.emitProgress(domain, "Ready", "done", domain+" updated to latest", 100)
	e.LogActivity(domain, "update", "updated to latest images", "success")
	return nil
}

// updateComposeSite pulls newer images and recreates the project.
func (e *Engine) updateComposeSite(ctx context.Context, domain, owner string) error {
	project := composeProjectName(domain)
	compDir := e.composeDir(owner, domain)

	// Pull first so `up -d` only swaps services whose image actually changed.
	pull := composeCmd(ctx, project, compDir, "pull")
	if out, err := pull.CombinedOutput(); err != nil {
		return Invalid("docker compose pull failed: %s", composeFailureMessage(strings.Split(string(out), "\n")))
	}

	e.emitProgress(domain, "Recreating containers", "running", "", 60)
	return e.composeUpStreaming(ctx, domain, project, compDir)
}

// updateNativeSite pulls each driver service image and recreates its containers
// in place. Recreation reuses the live container's config (via InspectReplica),
// so generated secrets in env, mounts, and limits all carry over — only the
// image is refreshed to the freshly-pulled tag.
func (e *Engine) updateNativeSite(ctx context.Context, domain string, driver *models.Driver) error {
	siteNetwork := fmt.Sprintf("apod-site-%s", strings.ReplaceAll(domain, ".", "-"))

	containers, err := e.docker.ListSiteContainers(ctx, domain)
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}

	// Pull every service image up front (a no-op when already current).
	for svcName, svc := range driver.Services {
		if err := e.docker.PullImage(ctx, svc.Image); err != nil {
			return fmt.Errorf("pull %s (%s): %w", svcName, svc.Image, err)
		}
	}

	total := len(containers)
	for i, c := range containers {
		svc, ok := driver.Services[c.Service]
		if !ok || c.Name == "" {
			continue // not a driver-managed service (or unnamed) — leave it alone
		}

		template, err := e.docker.InspectReplica(ctx, c.Name)
		if err != nil {
			return fmt.Errorf("inspect %s: %w", c.Name, err)
		}
		template.Name = c.Name
		template.Image = svc.Image // recreate on the freshly-pulled tag
		template.NetworkName = siteNetwork

		pct := 60
		if total > 0 {
			pct = 60 + (30*(i+1))/total
		}
		e.emitProgress(domain, "Recreating "+c.Service, "running", c.Name, pct)

		e.docker.StopContainer(ctx, c.Name)
		if err := e.docker.RemoveContainer(ctx, c.Name); err != nil {
			return fmt.Errorf("remove %s: %w", c.Name, err)
		}
		id, err := e.docker.CreateContainer(ctx, template)
		if err != nil {
			return fmt.Errorf("recreate %s: %w", c.Name, err)
		}
		if err := e.docker.StartContainer(ctx, id); err != nil {
			return fmt.Errorf("start %s: %w", c.Name, err)
		}
	}
	return nil
}
