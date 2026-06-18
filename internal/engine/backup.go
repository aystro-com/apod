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

	"github.com/aystro/apod/internal/db"
	"github.com/aystro/apod/internal/models"
	"github.com/aystro/apod/internal/storage"
)

// maxRestoreTotalBytes caps the total uncompressed bytes written during a
// restore to bound memory/disk impact from a malicious or corrupt archive.
const maxRestoreTotalBytes = 20 << 30 // 20 GiB

func dbDumpCommand(dbType, dbName, dbUser string) []string {
	switch dbType {
	case "mysql":
		// Use MYSQL_PASSWORD env var from container (set by driver)
		return []string{"sh", "-c", fmt.Sprintf("mysqldump -u%s -p\"$MYSQL_PASSWORD\" %s", dbUser, dbName)}
	case "postgres":
		return []string{"pg_dumpall", "-U", dbUser}
	case "mongo":
		return []string{"mongodump", "--archive", "--db", dbName}
	default:
		return nil
	}
}

// composeDumpCommand returns a dump command for compose-managed databases.
// Uses environment variables for credentials since compose handles auth via .env.
func composeDumpCommand(dbType string) []string {
	switch dbType {
	case "mysql":
		return []string{"sh", "-c", "mysqldump --all-databases -u root -p\"$MYSQL_ROOT_PASSWORD\""}
	case "postgres":
		// Use POSTGRES_USER env var (set by compose .env), fallback to postgres
		return []string{"sh", "-c", "pg_dumpall -U \"${POSTGRES_USER:-postgres}\""}
	case "mongo":
		return []string{"mongodump", "--archive"}
	default:
		return nil
	}
}

func dbRestoreCommand(dbType, dbName, dbUser, dumpFile string) []string {
	switch dbType {
	case "mysql":
		return []string{"sh", "-c", fmt.Sprintf("mysql -u%s -p\"$MYSQL_PASSWORD\" %s -e 'source %s'", dbUser, dbName, dumpFile)}
	case "postgres":
		return []string{"psql", "-U", dbUser, "-d", dbName, "-f", dumpFile}
	case "mongo":
		return []string{"mongorestore", "--archive=" + dumpFile, "--db", dbName}
	default:
		return nil
	}
}

// restoreZipEntry writes one zip entry to destPath, capped at limit bytes. It
// clears anything non-regular already at destPath (a directory or, commonly, a
// symlink such as Laravel's public/storage) so the write does not fail — that
// failure was previously misreported as a decompression bomb. Returns bytes
// written.
func restoreZipEntry(f *zip.File, destPath string, limit int64) (int64, error) {
	rc, err := f.Open()
	if err != nil {
		return 0, fmt.Errorf("open entry: %w", err)
	}
	defer rc.Close()
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return 0, err
	}
	if fi, lerr := os.Lstat(destPath); lerr == nil && !fi.Mode().IsRegular() {
		os.RemoveAll(destPath)
	}
	dest, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return 0, fmt.Errorf("create %s: %w", destPath, err)
	}
	defer dest.Close()
	return io.Copy(dest, io.LimitReader(rc, limit))
}

// dbVolumeDirs returns the set of top-level subdirectories under data_root that
// hold a database service's files (derived from the driver's backed-up DB
// services and their volume mounts). These are excluded from physical archiving
// and physical restore — the logical dump is the source of truth, and a hot copy
// of a live DB datadir is not crash-consistent.
func dbVolumeDirs(driver *models.Driver) map[string]bool {
	out := map[string]bool{}
	for _, dbCfg := range driver.Backup.Databases {
		svc, ok := driver.Services[dbCfg.Service]
		if !ok {
			continue
		}
		for _, vol := range svc.Volumes {
			host := vol
			if i := strings.Index(host, ":"); i >= 0 {
				host = host[:i]
			}
			host = strings.TrimPrefix(host, "${data_root}")
			host = strings.TrimPrefix(host, "/")
			if host == "" {
				continue
			}
			if i := strings.Index(host, "/"); i >= 0 {
				host = host[:i]
			}
			out[host] = true
		}
	}
	return out
}

// sourceDBPassword returns a site's database password. The authoritative source
// is the secrets store; for legacy sites created before it existed, it falls
// back to reading the live DB service container's env. Returns "" if neither
// has it.
func (e *Engine) sourceDBPassword(ctx context.Context, domain string, driver *models.Driver) string {
	if v, ok, _ := e.db.GetSiteSecret(domain, "db_password"); ok && v != "" {
		return v
	}
	keys := []string{"MYSQL_PASSWORD", "MARIADB_PASSWORD", "POSTGRES_PASSWORD", "DB_PASSWORD"}
	for _, dbCfg := range driver.Backup.Databases {
		cname := fmt.Sprintf("apod-%s-%s", domain, dbCfg.Service)
		cfg, err := e.docker.InspectReplica(ctx, cname)
		if err != nil {
			continue
		}
		for _, key := range keys {
			prefix := key + "="
			for _, env := range cfg.Env {
				if strings.HasPrefix(env, prefix) {
					if v := strings.TrimPrefix(env, prefix); v != "" {
						return v
					}
				}
			}
		}
	}
	return ""
}

// readEnvFileValue returns the value of key from a dotenv-style file, or "" if
// the file or key is absent. Surrounding quotes are trimmed.
func readEnvFileValue(path, key string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	prefix := key + "="
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			v := strings.TrimSpace(strings.TrimPrefix(line, prefix))
			v = strings.Trim(v, `"'`)
			return v
		}
	}
	return ""
}

type backupMetadata struct {
	Domain     string            `json:"domain"`
	Driver     string            `json:"driver"`
	DriverType string            `json:"driver_type,omitempty"`
	RAM        string            `json:"ram"`
	CPU        string            `json:"cpu"`
	Env        map[string]string `json:"env"`
	Domains    []string          `json:"domains"`
	CreatedAt  string            `json:"created_at"`
	// DBPassword is the source site's database password, captured so a clone
	// (restore-as-new-site) can keep the same credentials and restore the raw
	// data directory as-is — instead of regenerating credentials, which would
	// not match the restored datadir. Name/user derive from Domain.
	DBPassword string `json:"db_password,omitempty"`
}

// backupDir returns the local backup directory for a site based on ownership.
// User-owned sites: /home/<owner>/backups/  (counts against disk quota)
// Admin sites: /var/lib/apod/backups/
func (e *Engine) backupDir(owner string) string {
	if owner != "" {
		return filepath.Join("/home", owner, "backups")
	}
	return filepath.Join(e.dataDir, "backups")
}

func (e *Engine) getStorage(ctx context.Context, storageName, owner string) (storage.Storage, error) {
	if storageName == "" || storageName == "local" {
		return storage.NewLocal(e.backupDir(owner)), nil
	}

	sc, err := e.db.GetStorageConfig(storageName)
	if err != nil {
		return nil, fmt.Errorf("get storage config: %w", err)
	}

	var config map[string]string
	if err := json.Unmarshal([]byte(sc.Config), &config); err != nil {
		return nil, fmt.Errorf("parse storage config: %w", err)
	}

	return storage.New(sc.Driver, config)
}

// dirHasContent reports whether path exists and contains at least one entry.
func dirHasContent(path string) bool {
	entries, err := os.ReadDir(path)
	return err == nil && len(entries) > 0
}

func (e *Engine) CreateBackup(ctx context.Context, domain, storageName string) (int64, error) {
	if err := e.locks.Acquire(domain); err != nil {
		return 0, err
	}
	defer e.locks.Release(domain)

	site, err := e.db.GetSite(domain)
	if err != nil {
		return 0, fmt.Errorf("get site: %w", err)
	}

	driver, err := e.drivers.Load(site.Driver)
	if err != nil {
		return 0, fmt.Errorf("load driver: %w", err)
	}

	// Refuse to back up a site with nothing to store (e.g. a stateless driver
	// like apod-ui): no databases, no declared file paths, and no data on disk.
	// Otherwise we'd produce a useless ~200-byte archive whose "restore" just
	// stops and restarts the container.
	if len(driver.Backup.Databases) == 0 && len(driver.Backup.Paths) == 0 {
		_, dataRoot := e.SiteDir(site.Owner, domain)
		if !dirHasContent(dataRoot) {
			return 0, fmt.Errorf("nothing to back up: %q stores no data (stateless driver %q)", domain, site.Driver)
		}
	}

	store, err := e.getStorage(ctx, storageName, site.Owner)
	if err != nil {
		return 0, err
	}

	timestamp := time.Now().Format("20060102_150405")
	zipKey := fmt.Sprintf("%s/%s_%s.zip", domain, domain, timestamp)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	siteRoot, dataRoot := e.SiteDir(site.Owner, domain)
	dbName := strings.ReplaceAll(domain, ".", "_")
	dbUser := dbName

	// Dump databases (gzip-compressed)
	isCompose := driver.Type == "compose"
	for _, dbCfg := range driver.Backup.Databases {
		var dumpCmd []string
		if isCompose {
			// Compose sites: use superuser for dump (credentials come from compose .env)
			dumpCmd = composeDumpCommand(dbCfg.Type)
		} else {
			dumpCmd = dbDumpCommand(dbCfg.Type, dbName, dbUser)
		}
		if dumpCmd == nil {
			continue
		}

		var output []byte
		var err error
		// Retry up to 6 times with 10s delay (container may still be starting).
		// Capture stdout ONLY — a dump must not be polluted by stderr warnings
		// (e.g. mysqldump's password notice) or exec stream frame headers.
		for attempt := 0; attempt < 6; attempt++ {
			if isCompose {
				var s string
				s, err = e.ExecInComposeSite(ctx, domain, site.Owner, dbCfg.Service, dumpCmd)
				output = []byte(s)
			} else {
				containerName := fmt.Sprintf("apod-%s-%s", domain, dbCfg.Service)
				output, err = e.docker.ExecCaptureStdout(ctx, containerName, dumpCmd)
			}
			if err == nil {
				break
			}
			time.Sleep(10 * time.Second)
		}
		if err != nil {
			return 0, fmt.Errorf("dump %s database: %w", dbCfg.Type, err)
		}
		if len(bytes.TrimSpace(output)) == 0 {
			e.LogActivity(domain, "backup_warning", fmt.Sprintf("empty %s dump from %s", dbCfg.Type, dbCfg.Service), "warning")
			continue
		}
		w, _ := zw.Create(fmt.Sprintf("databases/%s_%s.sql.gz", dbCfg.Service, dbCfg.Type))
		gz := gzip.NewWriter(w)
		gz.Write(output)
		gz.Close()
	}

	// Collect backup paths — driver-defined paths + data_root (if not already included)
	backupPaths := make(map[string]string) // expanded -> prefix in zip
	for _, p := range driver.Backup.Paths {
		expanded := strings.ReplaceAll(p, "${site_root}", siteRoot)
		expanded = strings.ReplaceAll(expanded, "${data_root}", dataRoot)
		backupPaths[expanded] = "files"
	}
	// Auto-include data_root for volume data if not already covered
	if _, ok := backupPaths[dataRoot]; !ok {
		covered := false
		for p := range backupPaths {
			if strings.HasPrefix(dataRoot, p) || strings.HasPrefix(p, dataRoot) {
				covered = true
				break
			}
		}
		if !covered {
			if info, err := os.Stat(dataRoot); err == nil && info.IsDir() {
				backupPaths[dataRoot] = "data"
			}
		}
	}

	// A database service's raw data directory is intentionally NOT archived: the
	// logical dump is the source of truth, and a hot copy of a live datadir is
	// not crash-consistent (and never restored). Exclude those subdirs of
	// data_root from the physical archive.
	dbDirs := dbVolumeDirs(driver)

	// Copy files from all backup paths
	for expanded, prefix := range backupPaths {
		filepath.Walk(expanded, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			relPath, _ := filepath.Rel(expanded, path)
			if prefix == "data" {
				top := relPath
				if i := strings.IndexByte(top, filepath.Separator); i >= 0 {
					top = top[:i]
				}
				if dbDirs[top] {
					return nil // skip DB datadir — covered by the logical dump
				}
			}
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

	// Export metadata
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
		// Capture the live DB password so a clone can reuse it (see backupMetadata).
		// The backup already contains the full database and the site's secrets, so
		// recording the password here does not change its sensitivity.
		DBPassword: e.sourceDBPassword(ctx, domain, driver),
	}
	if meta.DBPassword == "" {
		meta.DBPassword = readEnvFileValue(filepath.Join(siteRoot, ".env"), "DB_PASSWORD")
	}
	metaJSON, _ := json.MarshalIndent(meta, "", "  ")
	w, _ := zw.Create("metadata.json")
	w.Write(metaJSON)

	// For compose sites: include the .env file with all secrets (JWT keys, passwords, etc.)
	// This is critical for restore/migration — without it, the site can't reconnect to its data.
	if isCompose {
		compDir := e.composeDir(site.Owner, domain)
		envFile := filepath.Join(compDir, ".env")
		if data, err := os.ReadFile(envFile); err == nil {
			w, _ := zw.Create("compose_env")
			w.Write(data)
		}
	}

	zw.Close()

	// Verify backup is not empty (metadata.json alone is ~200 bytes)
	if buf.Len() < 100 {
		e.LogActivity(domain, "backup", "backup appears empty", "failed")
		return 0, fmt.Errorf("backup verification failed: backup is empty")
	}

	// Ensure backup directory exists and is owned by the user
	bkDir := e.backupDir(site.Owner)
	os.MkdirAll(bkDir, 0755)
	if site.Owner != "" {
		if user, err := e.db.GetUserByName(site.Owner); err == nil {
			os.Chown(bkDir, user.UID, user.UID)
		}
	}

	// Encrypt the archive at rest (it contains databases and secrets).
	payload := buf.Bytes()
	if key, kerr := e.backupKey(); kerr == nil {
		if enc, eerr := encryptBackup(key, payload); eerr == nil {
			payload = enc
		} else {
			return 0, fmt.Errorf("encrypt backup: %w", eerr)
		}
	} else {
		return 0, fmt.Errorf("backup key: %w", kerr)
	}

	// Upload
	if err := store.Upload(ctx, zipKey, bytes.NewReader(payload)); err != nil {
		return 0, fmt.Errorf("upload backup: %w", err)
	}

	// Set ownership on backup file for user-owned sites
	if site.Owner != "" && (storageName == "" || storageName == "local") {
		backupFile := filepath.Join(bkDir, zipKey)
		if user, err := e.db.GetUserByName(site.Owner); err == nil {
			// Own the domain subdirectory too
			os.Chown(filepath.Dir(backupFile), user.UID, user.UID)
			os.Chown(backupFile, user.UID, user.UID)
		}
	}

	if storageName == "" {
		storageName = "local"
	}
	id, err := e.db.CreateBackup(domain, storageName, zipKey, int64(buf.Len()))
	if err != nil {
		return 0, fmt.Errorf("record backup: %w", err)
	}

	e.LogActivity(domain, "backup", fmt.Sprintf("created backup #%d (%d bytes)", id, buf.Len()), "success")
	return id, nil
}

// CreateSiteFromBackup provisions a brand-new site from an existing backup,
// leaving the original untouched. The backup archive already carries the same
// layout as an export (metadata.json + files/ + data/ + databases/), so this
// downloads it and reuses the import path with a fresh domain. Owner defaults to
// the source site's owner when empty.
func (e *Engine) CreateSiteFromBackup(ctx context.Context, backupID int64, newDomain, owner string) error {
	if err := ValidateDomain(newDomain); err != nil {
		return err
	}

	backup, err := e.db.GetBackup(backupID)
	if err != nil {
		return fmt.Errorf("get backup: %w", err)
	}
	if newDomain == backup.SiteDomain {
		return fmt.Errorf("new domain must differ from the source site %q", backup.SiteDomain)
	}
	if existing, _ := e.db.GetSite(newDomain); existing != nil {
		return fmt.Errorf("site %q already exists", newDomain)
	}

	source, err := e.db.GetSite(backup.SiteDomain)
	if err != nil || source == nil {
		return fmt.Errorf("source site %q not found", backup.SiteDomain)
	}
	if owner == "" {
		owner = source.Owner
	}

	store, err := e.getStorage(ctx, backup.StorageName, source.Owner)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := store.Download(ctx, backup.Path, &buf); err != nil {
		return fmt.Errorf("download backup: %w", err)
	}
	plain, err := e.decryptBackupBytes(buf.Bytes())
	if err != nil {
		return err
	}

	// ImportSite reads from a file path, so stage the archive in a temp file.
	tmp, err := os.CreateTemp("", "apod-backup-*.zip")
	if err != nil {
		return fmt.Errorf("stage backup: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(plain); err != nil {
		tmp.Close()
		return fmt.Errorf("stage backup: %w", err)
	}
	tmp.Close()

	if err := e.ImportSite(ctx, tmp.Name(), newDomain, owner); err != nil {
		return fmt.Errorf("create site from backup: %w", err)
	}
	e.LogActivity(newDomain, "site_from_backup", fmt.Sprintf("from backup %d of %s", backupID, backup.SiteDomain), "success")
	return nil
}

func (e *Engine) RestoreBackup(ctx context.Context, domain string, backupID int64) error {
	if err := e.locks.Acquire(domain); err != nil {
		return err
	}
	defer e.locks.Release(domain)

	backup, err := e.db.GetBackup(backupID)
	if err != nil {
		return fmt.Errorf("get backup: %w", err)
	}
	if backup.SiteDomain != domain {
		return fmt.Errorf("backup %d belongs to %q, not %q", backupID, backup.SiteDomain, domain)
	}

	site, err := e.db.GetSite(domain)
	if err != nil {
		return fmt.Errorf("get site: %w", err)
	}

	store, err := e.getStorage(ctx, backup.StorageName, site.Owner)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := store.Download(ctx, backup.Path, &buf); err != nil {
		return fmt.Errorf("download backup: %w", err)
	}
	plain, err := e.decryptBackupBytes(buf.Bytes())
	if err != nil {
		return err
	}

	zr, err := zip.NewReader(bytes.NewReader(plain), int64(len(plain)))
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}

	// Refuse a backup with no restorable content *before* touching the site —
	// otherwise we'd stop it and put nothing back, breaking it (e.g. restoring
	// a stateless-site backup).
	hasContent := false
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "files/") || strings.HasPrefix(f.Name, "data/") || strings.HasPrefix(f.Name, "databases/") {
			hasContent = true
			break
		}
	}
	if !hasContent {
		return fmt.Errorf("backup %d has no restorable data; refusing to restore", backupID)
	}

	// Stop site
	ids, _ := e.docker.ListContainersByLabel(ctx, labelPrefix+"site", domain)
	for _, id := range ids {
		e.docker.StopContainer(ctx, id)
	}

	siteRoot, dataRoot := e.SiteDir(site.Owner, domain)

	// Guard against decompression bombs: cap total bytes written during restore.
	var written int64
	for _, f := range zr.File {
		// Restore site files
		if strings.HasPrefix(f.Name, "files/") {
			relPath := strings.TrimPrefix(f.Name, "files/")
			if relPath == "" {
				continue
			}
			destPath := filepath.Join(siteRoot, relPath)
			if !strings.HasPrefix(filepath.Clean(destPath), filepath.Clean(siteRoot)+string(filepath.Separator)) {
				continue
			}
			n, err := restoreZipEntry(f, destPath, maxRestoreTotalBytes-written+1)
			written += n
			if written > maxRestoreTotalBytes {
				return fmt.Errorf("restore aborted: archive exceeds %d bytes (possible decompression bomb)", maxRestoreTotalBytes)
			}
			if err != nil {
				// Don't abort the whole restore (the site is already stopped) for
				// one bad entry — record it and carry on.
				e.LogActivity(domain, "restore_warning", fmt.Sprintf("%s: %v", f.Name, err), "warning")
			}
		}
		// Restore data directory (volumes)
		if strings.HasPrefix(f.Name, "data/") {
			relPath := strings.TrimPrefix(f.Name, "data/")
			if relPath == "" {
				continue
			}
			destPath := filepath.Join(dataRoot, relPath)
			if !strings.HasPrefix(filepath.Clean(destPath), filepath.Clean(dataRoot)+string(filepath.Separator)) {
				continue
			}
			n, err := restoreZipEntry(f, destPath, maxRestoreTotalBytes-written+1)
			written += n
			if written > maxRestoreTotalBytes {
				return fmt.Errorf("restore aborted: archive exceeds %d bytes (possible decompression bomb)", maxRestoreTotalBytes)
			}
			if err != nil {
				e.LogActivity(domain, "restore_warning", fmt.Sprintf("%s: %v", f.Name, err), "warning")
			}
		}
		if f.Name == "metadata.json" {
			rc, _ := f.Open()
			data, _ := io.ReadAll(rc)
			rc.Close()
			var meta backupMetadata
			json.Unmarshal(data, &meta)
			envJSON, _ := envToJSON(meta.Env)
			e.db.UpdateSiteConfig(domain, map[string]string{"env": envJSON})
		}
		// Restore compose .env (secrets, JWT keys, passwords)
		if f.Name == "compose_env" {
			compDir := e.composeDir(site.Owner, domain)
			envPath := filepath.Join(compDir, ".env")
			rc, _ := f.Open()
			data, _ := io.ReadAll(rc)
			rc.Close()
			os.MkdirAll(compDir, 0755)
			os.WriteFile(envPath, data, 0600)
		}
	}

	// Restart — use compose for compose sites, docker for normal
	driver, _ := e.drivers.Load(site.Driver)
	if driver != nil && driver.Type == "compose" {
		e.StartComposeSite(ctx, domain, site.Owner)
	} else {
		for _, id := range ids {
			e.docker.StartContainer(ctx, id)
		}
	}
	e.db.UpdateSiteStatus(domain, "running")
	return nil
}

func (e *Engine) DeleteBackup(ctx context.Context, domain string, backupID int64) error {
	backup, err := e.db.GetBackup(backupID)
	if err != nil {
		return err
	}
	if backup.SiteDomain != domain {
		return fmt.Errorf("backup %d belongs to %q, not %q", backupID, backup.SiteDomain, domain)
	}
	site, _ := e.db.GetSite(domain)
	owner := ""
	if site != nil {
		owner = site.Owner
	}
	store, err := e.getStorage(ctx, backup.StorageName, owner)
	if err != nil {
		return err
	}
	store.Delete(ctx, backup.Path)
	return e.db.DeleteBackup(backupID)
}

func (e *Engine) ListBackups(ctx context.Context, domain string) ([]db.Backup, error) {
	return e.db.ListBackups(domain)
}

func (e *Engine) GetBackupPath(ctx context.Context, domain string, backupID int64) (string, error) {
	backup, err := e.db.GetBackup(backupID)
	if err != nil {
		return "", err
	}
	if backup.SiteDomain != domain {
		return "", fmt.Errorf("backup does not belong to this site")
	}
	site, _ := e.db.GetSite(domain)
	owner := ""
	if site != nil {
		owner = site.Owner
	}
	// Validate path stays within backup directory to prevent path traversal
	bkDir := e.backupDir(owner)
	cleanPath := filepath.Clean(filepath.Join(bkDir, backup.Path))
	if !strings.HasPrefix(cleanPath, bkDir+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid backup path")
	}
	return cleanPath, nil
}
