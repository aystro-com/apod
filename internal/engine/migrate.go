package engine

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ExportSite creates a self-contained backup zip for migration.
// Returns the path to the export file.
func (e *Engine) ExportSite(ctx context.Context, domain, outputDir, passphrase string) (string, error) {
	if err := e.locks.Acquire(domain, "exporting"); err != nil {
		return "", err
	}
	defer e.locks.Release(domain)

	site, err := e.db.GetSite(domain)
	if err != nil {
		return "", fmt.Errorf("get site: %w", err)
	}

	driver, err := e.drivers.Load(site.Driver)
	if err != nil {
		return "", fmt.Errorf("load driver: %w", err)
	}

	if outputDir == "" {
		outputDir = "."
	}

	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_export_%s.zip", domain, timestamp)
	outputPath := filepath.Join(outputDir, filename)

	// Build the archive in memory so it can be (optionally) encrypted before it
	// touches disk.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	siteRoot, dataRoot := e.SiteDir(site.Owner, domain)
	dbName := strings.ReplaceAll(domain, ".", "_")
	dbUser := dbName

	// Dump databases (gzip-compressed). Route through siteCapture with the right
	// credential mode: compose sites have no `apod-<domain>-<service>` container
	// and use compose-managed superuser creds. Hardcoding the native container +
	// siteCreds (the old behavior) silently produced an export with NO database
	// for every compose site, which then migrated as total DB loss.
	isCompose := driver.Type == "compose"
	dumpMode := siteCreds
	if isCompose {
		dumpMode = superCreds
	}
	for _, dbCfg := range driver.Backup.Databases {
		dumpCmd := dbDumpCmd(dbCfg.Type, dbName, dbUser, dumpMode)
		if dumpCmd == nil {
			continue
		}
		// Capture stdout only — stderr warnings and exec frame headers would
		// corrupt the SQL.
		output, err := e.siteCapture(ctx, domain, site.Owner, dbCfg.Service, isCompose, dumpCmd)
		if err != nil {
			e.LogActivity(domain, "export_warning", fmt.Sprintf("db dump failed for %s: %v", dbCfg.Service, err), "warning")
			continue
		}
		if len(bytes.TrimSpace(output)) == 0 {
			continue
		}
		w, _ := zw.Create(fmt.Sprintf("databases/%s_%s.sql.gz", dbCfg.Service, dbCfg.Type))
		gz := gzip.NewWriter(w)
		gz.Write(output)
		gz.Close()
	}

	// Copy site files
	for _, p := range driver.Backup.Paths {
		expanded := strings.ReplaceAll(p, "${site_root}", siteRoot)
		expanded = strings.ReplaceAll(expanded, "${data_root}", dataRoot)
		addDirToZip(zw, expanded, "files")
	}

	// Copy data root (volume data)
	if info, err := os.Stat(dataRoot); err == nil && info.IsDir() {
		addDirToZip(zw, dataRoot, "data")
	}

	// Export metadata with storage info for migration
	envs, _ := parseEnvJSON(site.Env)
	domains, _ := e.db.ListDomains(site.ID)

	meta := backupMetadata{
		Domain:     site.Domain,
		Driver:     site.Driver,
		DriverType: driver.Type,
		RAM:        site.RAM,
		CPU:        site.CPU,
		Env:        envs,
		Domains:    domains,
		CreatedAt:  time.Now().Format(time.RFC3339),
	}
	metaJSON, _ := json.MarshalIndent(meta, "", "  ")
	w, _ := zw.Create("metadata.json")
	w.Write(metaJSON)

	// Include compose .env for migration
	if driver.Type == "compose" {
		compDir := e.composeDir(site.Owner, domain)
		if data, err := os.ReadFile(filepath.Join(compDir, ".env")); err == nil {
			w, _ := zw.Create("compose_env")
			w.Write(data)
		}
	}

	zw.Close()

	payload := buf.Bytes()
	if len(payload) < 100 {
		return "", fmt.Errorf("export verification failed: archive is empty")
	}
	// Optionally encrypt the export at rest with a passphrase-derived key.
	if passphrase != "" {
		enc, err := encryptWithPassphrase(passphrase, payload)
		if err != nil {
			return "", fmt.Errorf("encrypt export: %w", err)
		}
		payload = enc
	}
	if err := os.WriteFile(outputPath, payload, 0600); err != nil {
		return "", fmt.Errorf("write export: %w", err)
	}

	e.LogActivity(domain, "export", fmt.Sprintf("exported to %s (%d bytes)", outputPath, len(payload)), "success")
	return outputPath, nil
}

// ImportSite creates a new site from an export zip file.
// The zip must contain metadata.json with the site config.
// Optionally override the domain with newDomain (empty = use domain from metadata).
// reapplyDriverSetup re-runs a driver's setup steps against an imported site so
// the restored application reconciles to its new environment (clearing caches
// that pin source-specific values, fixing permissions, ensuring keys, …). Setup
// steps are idempotent by design. Best-effort: per-step failures are ignored so
// a single failing step does not abort an otherwise-successful import.
func (e *Engine) reapplyDriverSetup(ctx context.Context, domain, owner, driverName, dbName, dbPass string) {
	driver, err := e.drivers.Load(driverName)
	if err != nil || len(driver.Setup) == 0 {
		return
	}
	siteRoot, dataRoot := e.SiteDir(owner, domain)
	vars := map[string]string{
		"site_root":    siteRoot,
		"data_root":    dataRoot,
		"site_domain":  domain,
		"site_db_name": dbName,
		"site_db_user": dbName,
		"site_db_pass": dbPass,
	}
	for _, step := range driver.Setup {
		cmd := expandVariables(step.Command, vars)
		cname := fmt.Sprintf("apod-%s-%s", domain, step.Service)
		e.docker.ExecInContainerAs(ctx, cname, []string{"sh", "-c", cmd}, step.User)
	}
}

// dbPasswordFromZip recovers DB_PASSWORD from the .env captured in a backup
// archive (files/.env), for older backups whose metadata predates the
// db_password field. Returns "" when absent.
func dbPasswordFromZip(zr *zip.Reader) string {
	for _, f := range zr.File {
		if f.Name != "files/.env" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return ""
		}
		data, _ := io.ReadAll(rc)
		rc.Close()
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "DB_PASSWORD=") {
				v := strings.TrimSpace(strings.TrimPrefix(line, "DB_PASSWORD="))
				return strings.Trim(v, `"'`)
			}
		}
	}
	return ""
}

// waitForDBReady polls a freshly-started database service until it accepts a
// credentialed connection (or a timeout elapses), so a subsequent dump restore
// does not race container initialization. It works for BOTH runtimes: native
// sites exec into apod-<domain>-<service> with the per-site creds; compose sites
// exec via `docker compose exec` with the superuser creds. Best-effort: it never
// returns an error, callers proceed regardless.
func (e *Engine) waitForDBReady(ctx context.Context, domain, owner, service, dbType, dbUser, dbName string, isCompose bool) {
	mode := siteCreds
	if isCompose {
		mode = superCreds
	}
	probe := dbProbeCmd(dbType, dbName, dbUser, mode)
	if probe == nil {
		return
	}
	container := containerNameFor(domain, service)
	for i := 0; i < 45; i++ {
		var out string
		var err error
		if isCompose {
			var b []byte
			b, err = e.execInComposeSiteStdout(ctx, domain, owner, service, probe)
			out = string(b)
		} else {
			out, err = e.docker.ExecInContainer(ctx, container, probe)
		}
		low := strings.ToLower(out)
		if err == nil && !strings.Contains(low, "error") &&
			!strings.Contains(low, "denied") && !strings.Contains(low, "refused") &&
			!strings.Contains(low, "can't connect") && !strings.Contains(low, "starting up") {
			return
		}
		time.Sleep(2 * time.Second)
	}
}

func (e *Engine) ImportSite(ctx context.Context, zipPath, newDomain, owner, passphrase string) error {
	// Importing provisions a whole new site and replays its database — a long,
	// destructive operation that must finish regardless of the client
	// connection that started it.
	ctx, cancel := detachCtx(ctx, 15*time.Minute)
	defer cancel()

	data, err := os.ReadFile(zipPath)
	if err != nil {
		return fmt.Errorf("read export file: %w", err)
	}

	// Decrypt a passphrase-encrypted export (a plaintext export is unchanged).
	if isPassphraseEncrypted(data) {
		if passphrase == "" {
			return Invalid("this export is encrypted; a passphrase is required to import it")
		}
		data, err = decryptWithPassphrase(passphrase, data)
		if err != nil {
			return err
		}
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}

	// Read metadata
	var meta backupMetadata
	for _, f := range zr.File {
		if f.Name == "metadata.json" {
			rc, _ := f.Open()
			metaData, _ := io.ReadAll(rc)
			rc.Close()
			if err := json.Unmarshal(metaData, &meta); err != nil {
				return fmt.Errorf("parse metadata: %w", err)
			}
			break
		}
	}
	if meta.Domain == "" {
		return fmt.Errorf("export file has no metadata — not a valid apod export")
	}

	// meta.Domain is fully attacker-controlled (it comes from the uploaded
	// archive) and is used to derive the database name/user, which flow into
	// `sh -c` mysql/psql commands. Validate it unconditionally here — not just
	// the effective `domain` below, which may instead come from newDomain — so a
	// crafted metadata domain like "x$(touch /tmp/pwn)" can never reach a shell.
	if err := ValidateDomain(meta.Domain); err != nil {
		return fmt.Errorf("invalid domain in export metadata: %w", err)
	}
	// Env from the archive is persisted directly; enforce the same key/no-newline
	// rules SetEnv uses so it can't inject extra compose .env lines.
	if err := validateEnvMap(meta.Env); err != nil {
		return fmt.Errorf("invalid env in export metadata: %w", err)
	}

	domain := newDomain
	if domain == "" {
		domain = meta.Domain
	}

	// The domain comes from an untrusted export archive — validate before it
	// reaches container names, file paths, or shell commands.
	if err := ValidateDomain(domain); err != nil {
		return err
	}
	if err := ValidateOwner(owner); err != nil {
		return err
	}

	// Database restore strategy: always rebuild DB data from the logical dump
	// (databases/), never from the raw data directory — a hot file copy of a
	// live DB is not crash-consistent (Postgres refuses to start from one). To
	// keep the cloned app, its restored .env and the DB mutually consistent, we
	// *preserve* the source's DB credentials and name so the freshly-initialised
	// DB the dump loads into matches what the app expects. The DB lives on an
	// isolated per-site network, so reusing the password is not a security
	// concern. The password comes from metadata, falling back to the .env
	// captured in the backup for older archives.
	srcDBPass := meta.DBPassword
	if srcDBPass == "" {
		srcDBPass = dbPasswordFromZip(zr)
	}
	preserveDB := srcDBPass != ""
	srcDBName := strings.ReplaceAll(meta.Domain, ".", "_")

	// Create the site using the driver and config from metadata
	createOpts := CreateSiteOpts{
		Domain: domain,
		Driver: meta.Driver,
		RAM:    meta.RAM,
		CPU:    meta.CPU,
		Owner:  owner,
	}
	if preserveDB {
		createOpts.DBName = srcDBName
		createOpts.DBPassword = srcDBPass
	}
	if err = e.CreateSite(ctx, createOpts); err != nil {
		return fmt.Errorf("create site: %w", err)
	}

	// Wait for containers to be ready
	time.Sleep(3 * time.Second)

	// Get the site to find paths
	site, err := e.db.GetSite(domain)
	if err != nil {
		return fmt.Errorf("get created site: %w", err)
	}
	siteRoot, dataRoot := e.SiteDir(site.Owner, domain)

	// Restore compose .env if present (must happen before restart)
	for _, f := range zr.File {
		if f.Name == "compose_env" {
			compDir := e.composeDir(site.Owner, domain)
			rc, _ := f.Open()
			data, _ := io.ReadAll(rc)
			rc.Close()
			os.MkdirAll(compDir, 0755)
			os.WriteFile(filepath.Join(compDir, ".env"), data, 0600)
		}
	}

	// Never restore a database service's raw data directory — see the strategy
	// note above. The DB is rebuilt from its logical dump below; restoring the
	// datadir would make the new container boot on a non-empty volume (skipping
	// init) and, for Postgres, panic on an inconsistent hot copy. (New backups
	// no longer archive these dirs at all; this also guards older archives.)
	skipDataDirs := map[string]bool{}
	if drv, derr := e.drivers.Load(meta.Driver); derr == nil {
		skipDataDirs = dbVolumeDirs(drv)
	}

	// Extract files and data. Cap the total bytes written so a malicious upload
	// can't fill the disk via a decompression bomb (imports are reachable by
	// non-admin users), mirroring RestoreBackup.
	var written int64
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "files/") {
			relPath := strings.TrimPrefix(f.Name, "files/")
			if relPath == "" {
				continue
			}
			destPath := filepath.Join(siteRoot, relPath)
			if !strings.HasPrefix(filepath.Clean(destPath), filepath.Clean(siteRoot)+string(filepath.Separator)) {
				continue
			}
			n, _ := restoreZipEntry(f, siteRoot, destPath, maxRestoreTotalBytes-written+1)
			written += n
			if written > maxRestoreTotalBytes {
				return Invalid("import aborted: archive exceeds %d bytes (possible decompression bomb)", maxRestoreTotalBytes)
			}
		}
		if strings.HasPrefix(f.Name, "data/") {
			relPath := strings.TrimPrefix(f.Name, "data/")
			if relPath == "" {
				continue
			}
			top := relPath
			if i := strings.Index(top, "/"); i >= 0 {
				top = top[:i]
			}
			if skipDataDirs[top] {
				// Database datadir: rely on fresh init + the logical dump.
				continue
			}
			destPath := filepath.Join(dataRoot, relPath)
			if !strings.HasPrefix(filepath.Clean(destPath), filepath.Clean(dataRoot)+string(filepath.Separator)) {
				continue
			}
			n, _ := restoreZipEntry(f, dataRoot, destPath, maxRestoreTotalBytes-written+1)
			written += n
			if written > maxRestoreTotalBytes {
				return Invalid("import aborted: archive exceeds %d bytes (possible decompression bomb)", maxRestoreTotalBytes)
			}
		}
	}

	// Rebuild databases from their logical dumps. When credentials are preserved
	// the dump loads into the source-named DB the new container initialised with;
	// otherwise it targets the new domain's derived name.
	driver, err := e.drivers.Load(meta.Driver)
	if err == nil {
		isCompose := driver.Type == "compose"
		dbName := strings.ReplaceAll(domain, ".", "_")
		if preserveDB {
			dbName = srcDBName
		}
		dbUser := dbName

		for _, dbCfg := range driver.Backup.Databases {
			// readDumpFromZip caps the decompressed size, guarding against a
			// gzip bomb in the uploaded archive.
			dump := readDumpFromZip(zr, dbCfg.Service, dbCfg.Type)
			if len(dump) == 0 {
				continue
			}
			e.restoreDatabase(ctx, domain, site.Owner, dbCfg.Service, dbCfg.Type, dbName, dbUser, isCompose, dump)
		}
	}

	// Restore env vars
	if len(meta.Env) > 0 {
		envJSON, _ := envToJSON(meta.Env)
		e.db.UpdateSiteConfig(domain, map[string]string{"env": envJSON})
	}

	// Add alias domains
	for _, d := range meta.Domains {
		if d != domain && d != meta.Domain {
			e.AddDomain(ctx, domain, d)
		}
	}

	// Re-run the driver's setup steps so the restored app reconciles to its new
	// environment. Crucially this clears any cached config baked at the source
	// (e.g. Laravel's bootstrap/cache/config.php, which pins the source DB host).
	// Setup steps are idempotent by design.
	reDBName := strings.ReplaceAll(domain, ".", "_")
	reDBPass := ""
	if preserveDB {
		reDBName = srcDBName
		reDBPass = srcDBPass
	}
	e.reapplyDriverSetup(ctx, domain, site.Owner, meta.Driver, reDBName, reDBPass)

	// Restart containers to pick up restored files and cleared caches
	ids, _ := e.docker.ListContainersByLabel(ctx, labelPrefix+"site", domain)
	for _, id := range ids {
		e.docker.StopContainer(ctx, id)
		e.docker.StartContainer(ctx, id)
	}

	e.LogActivity(domain, "import", fmt.Sprintf("imported from %s (originally %s)", zipPath, meta.Domain), "success")
	return nil
}

func addDirToZip(zw *zip.Writer, dir, prefix string) {
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		// Don't follow symlinks — a tenant could symlink a host file (e.g.
		// /etc/shadow) into their site dir and exfiltrate it via the export.
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		relPath, _ := filepath.Rel(dir, path)
		w, _ := zw.Create(filepath.Join(prefix, relPath))
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		io.Copy(w, f)
		return nil
	})
}
