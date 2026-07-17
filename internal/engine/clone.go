package engine

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/aystro/apod/internal/models"
)

// Clone creates a new site that is a faithful copy of an existing one.
//
// For normal (non-compose) sites it performs a *physical* clone: it quiesces the
// source (so its on-disk state is consistent), copies the site's files and data
// volumes byte-for-byte, then brings the source back up and stands the target up
// on the copied data while preserving the source's database credentials. This is
// stack-agnostic by construction — there is no per-database dump/restore logic,
// so it works the same for MySQL, Postgres, Mongo, Redis, SQLite, or a site with
// no database at all. The trade-off is brief source downtime for the duration of
// the copy (a filesystem snapshot could remove even that; see docs/testing.md).
//
// Compose-managed sites keep their own networks and secrets, so they use a
// logical copy of declared paths plus a dump/restore of declared databases.
func (e *Engine) Clone(ctx context.Context, sourceDomain, targetDomain string) error {
	if err := ValidateDomain(targetDomain); err != nil {
		return err
	}
	if sourceDomain == targetDomain {
		return fmt.Errorf("source and target domain must be different")
	}

	if err := e.locks.Acquire(sourceDomain, "cloning"); err != nil {
		return err
	}
	defer e.locks.Release(sourceDomain)

	// Cloning copies the source's files, data and database into a brand-new
	// site — long-running, and must run to completion regardless of whether the
	// client that kicked it off is still connected.
	ctx, cancel := detachCtx(ctx, 15*time.Minute)
	defer cancel()

	source, err := e.db.GetSite(sourceDomain)
	if err != nil {
		return fmt.Errorf("get source site: %w", err)
	}
	if source == nil {
		return NotFound("source site %q not found", sourceDomain)
	}
	if existing, _ := e.db.GetSite(targetDomain); existing != nil {
		return Conflict("target site %q already exists", targetDomain)
	}

	driver, err := e.drivers.Load(source.Driver)
	if err != nil {
		return fmt.Errorf("load driver: %w", err)
	}

	// Stream clone progress (keyed by the source domain, which holds the lock)
	// so the panel shows live status like a deploy does.
	e.beginOp(sourceDomain, "Preparing clone")
	var cerr error
	if driver.Type == "compose" {
		cerr = e.cloneCompose(ctx, source, driver, targetDomain)
	} else {
		cerr = e.clonePhysical(ctx, source, driver, targetDomain)
	}
	e.finishOp(sourceDomain, "Cloned", targetDomain+" created", cerr)
	return cerr
}

// clonePhysical implements the consistent volume-copy clone described on Clone.
func (e *Engine) clonePhysical(ctx context.Context, source *models.Site, driver *models.Driver, targetDomain string) error {
	sourceRoot, sourceData := e.SiteDir(source.Owner, source.Domain)
	targetRoot, targetData := e.SiteDir(source.Owner, targetDomain)

	// Capture the source's DB credentials so the copied data directory stays
	// valid under the target (which would otherwise generate new ones).
	srcDBPass := e.sourceDBPassword(ctx, source.Domain, driver)
	if srcDBPass == "" {
		srcDBPass = readEnvFileValue(filepath.Join(sourceRoot, ".env"), "DB_PASSWORD")
	}
	srcDBName := strings.ReplaceAll(source.Domain, ".", "_")

	// Quiesce the source so files and databases are copied from a consistent,
	// at-rest state (a hot copy of a live DB is not crash-consistent).
	srcIDs, _ := e.docker.ListContainersByLabel(ctx, labelPrefix+"site", source.Domain)
	for _, id := range srcIDs {
		e.docker.StopContainer(ctx, id)
	}

	e.emitProgress(source.Domain, "Copying files & data", "running", "", 45)
	copyErr := copyDir(sourceRoot, targetRoot)
	if copyErr == nil {
		copyErr = copyDir(sourceData, targetData)
	}

	// Bring the source back up as soon as the copy is done — but only if it was
	// running before. Unconditionally starting it would silently resurrect a site
	// the operator had deliberately stopped.
	if source.Status == "running" {
		for _, id := range srcIDs {
			e.docker.StartContainer(ctx, id)
		}
	}
	if copyErr != nil {
		os.RemoveAll(targetRoot)
		os.RemoveAll(targetData)
		return fmt.Errorf("copy site data: %w", copyErr)
	}

	// Stand the target up on the copied data, preserving the source's DB
	// credentials and name and skipping the git clone (files are already
	// present). CreateSite's setup steps reconcile the app to its new
	// environment (e.g. clearing cached config that pinned the source).
	err := e.CreateSite(ctx, CreateSiteOpts{
		Domain:     targetDomain,
		Driver:     source.Driver,
		RAM:        source.RAM,
		CPU:        source.CPU,
		Owner:      source.Owner,
		Repo:       source.Repo,
		Branch:     source.Branch,
		SkipClone:  true,
		DBName:     srcDBName,
		DBPassword: srcDBPass,
	})
	if err != nil {
		return fmt.Errorf("create target site: %w", err)
	}

	if envs, _ := parseEnvJSON(source.Env); len(envs) > 0 {
		// Re-validate before persisting to the clone (a fresh persist boundary)
		// so an env value can't inject extra compose .env lines.
		if err := validateEnvMap(envs); err != nil {
			return fmt.Errorf("invalid env on source site: %w", err)
		}
		envJSON, _ := envToJSON(envs)
		e.db.UpdateSiteConfig(targetDomain, map[string]string{"env": envJSON})
	}

	e.LogActivity(source.Domain, "clone", fmt.Sprintf("cloned to %s", targetDomain), "success")
	e.LogActivity(targetDomain, "clone", fmt.Sprintf("cloned from %s", source.Domain), "success")
	return nil
}

// cloneCompose clones a compose-managed site: the target gets its own fresh
// compose project (networks, secrets), so only declared file paths are copied
// and declared databases are moved via a logical dump/restore.
func (e *Engine) cloneCompose(ctx context.Context, source *models.Site, driver *models.Driver, targetDomain string) error {
	if err := e.CreateSite(ctx, CreateSiteOpts{
		Domain: targetDomain,
		Driver: source.Driver,
		RAM:    source.RAM,
		CPU:    source.CPU,
		Owner:  source.Owner,
		Repo:   source.Repo,
		Branch: source.Branch,
	}); err != nil {
		return fmt.Errorf("create target site: %w", err)
	}

	sourceRoot, sourceData := e.SiteDir(source.Owner, source.Domain)
	target, err := e.db.GetSite(targetDomain)
	if err != nil || target == nil {
		// CreateSite just succeeded, so this is a transient DB failure — bail
		// rather than dereferencing nil and panicking the daemon.
		return fmt.Errorf("load freshly created target site: %w", err)
	}
	targetRoot, targetData := e.SiteDir(target.Owner, targetDomain)

	// Copy only driver-declared paths (e.g. uploads) — not raw DB data or the
	// compose config, which the target provisions itself.
	for _, p := range driver.Backup.Paths {
		srcPath := strings.ReplaceAll(p, "${site_root}", sourceRoot)
		srcPath = strings.ReplaceAll(srcPath, "${data_root}", sourceData)
		dstPath := strings.ReplaceAll(p, "${site_root}", targetRoot)
		dstPath = strings.ReplaceAll(dstPath, "${data_root}", targetData)
		copyDir(srcPath, dstPath)
	}

	if envs, _ := parseEnvJSON(source.Env); len(envs) > 0 {
		// Re-validate before persisting to the clone (a fresh persist boundary)
		// so an env value can't inject extra compose .env lines.
		if err := validateEnvMap(envs); err != nil {
			return fmt.Errorf("invalid env on source site: %w", err)
		}
		envJSON, _ := envToJSON(envs)
		e.db.UpdateSiteConfig(targetDomain, map[string]string{"env": envJSON})
	}

	for _, dbCfg := range driver.Backup.Databases {
		dumpCmd := dbDumpCmd(dbCfg.Type, "", "", superCreds)
		if dumpCmd == nil {
			continue
		}
		dump, err := e.siteCapture(ctx, source.Domain, source.Owner, dbCfg.Service, true, dumpCmd)
		if err != nil {
			continue
		}
		e.restoreDatabase(ctx, targetDomain, target.Owner, dbCfg.Service, dbCfg.Type, "", "", true, dump)
	}

	e.LogActivity(source.Domain, "clone", fmt.Sprintf("cloned to %s", targetDomain), "success")
	e.LogActivity(targetDomain, "clone", fmt.Sprintf("cloned from %s", source.Domain), "success")
	return nil
}

// copyDir recursively copies src to dst, preserving file mode, ownership
// (uid/gid) and symlinks. Ownership matters for database data directories,
// whose files are owned by the in-container DB uid (e.g. mysql, postgres) and
// would be unreadable to the copied DB if flattened to root.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Don't silently skip an unreadable entry — that produces a clone with
			// missing files (e.g. a broken DB data dir) reported as success.
			return fmt.Errorf("walk %s: %w", path, err)
		}
		relPath, _ := filepath.Rel(src, path)
		dstPath := filepath.Join(dst, relPath)

		uid, gid := -1, -1
		if st, ok := info.Sys().(*syscall.Stat_t); ok {
			uid, gid = int(st.Uid), int(st.Gid)
		}

		switch {
		case info.IsDir():
			if err := os.MkdirAll(dstPath, info.Mode().Perm()); err != nil {
				return err
			}
		case info.Mode()&os.ModeSymlink != 0:
			target, lerr := os.Readlink(path)
			if lerr != nil {
				return nil
			}
			os.Remove(dstPath)
			if err := os.Symlink(target, dstPath); err != nil {
				return nil
			}
			os.Lchown(dstPath, uid, gid)
			return nil // don't chmod/chown-follow a symlink below
		default:
			srcFile, oerr := os.Open(path)
			if oerr != nil {
				return fmt.Errorf("open %s: %w", path, oerr)
			}
			defer srcFile.Close()
			os.MkdirAll(filepath.Dir(dstPath), 0755)
			dstFile, cerr := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
			if cerr != nil {
				return fmt.Errorf("create %s: %w", dstPath, cerr)
			}
			if _, err := io.Copy(dstFile, srcFile); err != nil {
				dstFile.Close()
				return err
			}
			dstFile.Close()
		}
		os.Chown(dstPath, uid, gid)
		return nil
	})
}
