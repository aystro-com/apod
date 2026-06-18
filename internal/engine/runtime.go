package engine

import (
	"context"
	"fmt"
)

// This file unifies "run a command in a site's service" across the two runtimes
// apod supports — native per-site containers and compose-managed projects — so
// callers (backup, restore, clone, export, import, cron, db) express the
// operation once instead of branching on the driver type everywhere.

// containerNameFor returns the native container name for a site service.
func containerNameFor(domain, service string) string {
	return fmt.Sprintf("apod-%s-%s", domain, service)
}

// siteExec runs cmd in a site's service container and returns combined output,
// failing on a non-zero exit. Dispatches to compose or native docker.
func (e *Engine) siteExec(ctx context.Context, domain, owner, service string, isCompose bool, cmd []string) (string, error) {
	if isCompose {
		return e.ExecInComposeSite(ctx, domain, owner, service, cmd)
	}
	return e.docker.ExecInContainer(ctx, containerNameFor(domain, service), cmd)
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
