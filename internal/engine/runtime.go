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
	if isCompose {
		return e.execInComposeSiteInput(ctx, domain, owner, service, cmd, dump)
	}
	container := containerNameFor(domain, service)
	e.waitForDBReady(ctx, container, dbType, dbUser, dbName)
	_, err := e.docker.ExecWithInput(ctx, container, cmd, dump)
	return err
}
