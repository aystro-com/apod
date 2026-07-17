package engine

import (
	"context"

	"fmt"
	"github.com/aystro/apod/internal/models"
)

// This file unifies "run a command in a site's service" across the two runtimes
// apod supports — native per-site containers and compose-managed projects — so
// callers (backup, restore, clone, export, import, cron, db) express the
// operation once instead of branching on the driver type everywhere.

// containerNameFor returns the native container name for a site service.
func containerNameFor(domain, service string) string {
	return fmt.Sprintf("apod-%s-%s", domain, service)
}

// primaryServiceName returns a site's primary (web/HTTP) service — the one that
// deploy hooks, logs, the terminal and monitoring target. It is the web-role
// service (which includes a service literally named "app" with no role), and
// falls back to the first service by name so the result is deterministic. This
// keeps those operations stack-agnostic instead of assuming a service called
// "app".
func primaryServiceName(driver *models.Driver) string {
	first := ""
	for name, svc := range driver.Services {
		if effectiveRole(name, svc.Role) == roleWeb {
			return name
		}
		if first == "" || name < first {
			first = name
		}
	}
	return first
}

// primaryServiceContainer returns the native container name of a site's primary
// service, loading the driver to find it (falling back to "app").
func (e *Engine) primaryServiceContainer(domain string) string {
	name := "app"
	if site, err := e.db.GetSite(domain); err == nil && site != nil {
		if driver, derr := e.drivers.Load(site.Driver); derr == nil {
			if p := primaryServiceName(driver); p != "" {
				name = p
			}
		}
	}
	return containerNameFor(domain, name)
}

// siteCapture runs cmd in a site's service container and returns stdout only —
// required when capturing binary/structured output such as a database dump,
// where stderr (and, natively, the exec stream frame headers) would corrupt it.
func (e *Engine) siteCapture(ctx context.Context, domain, owner, service string, isCompose bool, cmd []string) ([]byte, error) {
	if isCompose {
		return e.execInComposeSiteStdout(ctx, domain, owner, service, cmd)
	}
	return e.docker.ExecCaptureStdout(ctx, containerNameFor(domain, service), cmd)
}

// restoreDatabase replays a logical dump into a site's database, streaming the
// dump via stdin (no argv length limit). For native sites it first waits for the
// freshly-created DB to accept connections. Used by every restore path (backup
// restore, new-site, clone, db import) so the behaviour is identical and
// correct everywhere.
func (e *Engine) restoreDatabase(ctx context.Context, domain, owner, service, dbType, dbName, dbUser string, isCompose bool, dump []byte) error {
	mode := siteCreds
	if isCompose {
		mode = superCreds
	}
	cmd := dbRestoreCmd(dbType, dbName, dbUser, mode)
	if cmd == nil {
		return Invalid("unsupported database type: %s", dbType)
	}
	// Wait for the DB to accept connections first — for BOTH runtimes. Compose
	// restores previously skipped this and raced the DB container's init, so a
	// restore/import right after StartComposeSite failed with "connection
	// refused" and the site came up without its data.
	e.waitForDBReady(ctx, domain, owner, service, dbType, dbUser, dbName, isCompose)
	if isCompose {
		return e.execInComposeSiteInput(ctx, domain, owner, service, cmd, dump)
	}
	container := containerNameFor(domain, service)
	_, err := e.docker.ExecWithInput(ctx, container, cmd, dump)
	return err
}
