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
	"github.com/aystro/apod/internal/storage"
)

func dbDumpCommand(dbType, dbName, dbUser, dbPass string) []string {
	switch dbType {
	case "mysql":
		return []string{"mysqldump", "-u" + dbUser, "-p" + dbPass, dbName}
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

func dbRestoreCommand(dbType, dbName, dbUser, dbPass, dumpFile string) []string {
	switch dbType {
	case "mysql":
		return []string{"mysql", "-u" + dbUser, "-p" + dbPass, dbName, "-e", "source " + dumpFile}
	case "postgres":
		return []string{"psql", "-U", dbUser, "-d", dbName, "-f", dumpFile}
	case "mongo":
		return []string{"mongorestore", "--archive=" + dumpFile, "--db", dbName}
	default:
		return nil
	}
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
			dumpCmd = dbDumpCommand(dbCfg.Type, dbName, dbUser, "backup")
		}
		if dumpCmd == nil {
			continue
		}

		var output string
		var err error
		if isCompose {
			output, err = e.ExecInComposeSite(ctx, domain, site.Owner, dbCfg.Service, dumpCmd)
		} else {
			containerName := fmt.Sprintf("apod-%s-%s", domain, dbCfg.Service)
			output, err = e.docker.ExecInContainer(ctx, containerName, dumpCmd)
		}
		if err != nil {
			return 0, fmt.Errorf("dump %s database: %w", dbCfg.Type, err)
		}
		if len(strings.TrimSpace(output)) == 0 {
			e.LogActivity(domain, "backup_warning", fmt.Sprintf("empty %s dump from %s", dbCfg.Type, dbCfg.Service), "warning")
			continue
		}
		w, _ := zw.Create(fmt.Sprintf("databases/%s_%s.sql.gz", dbCfg.Service, dbCfg.Type))
		gz := gzip.NewWriter(w)
		gz.Write([]byte(output))
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

	// Copy files from all backup paths
	for expanded, prefix := range backupPaths {
		filepath.Walk(expanded, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			relPath, _ := filepath.Rel(expanded, path)
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

	// Upload
	if err := store.Upload(ctx, zipKey, bytes.NewReader(buf.Bytes())); err != nil {
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

	// Stop site
	ids, _ := e.docker.ListContainersByLabel(ctx, labelPrefix+"site", domain)
	for _, id := range ids {
		e.docker.StopContainer(ctx, id)
	}

	// Extract
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}

	siteRoot, dataRoot := e.SiteDir(site.Owner, domain)

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
			os.MkdirAll(filepath.Dir(destPath), 0755)
			rc, _ := f.Open()
			dest, _ := os.Create(destPath)
			io.Copy(dest, rc)
			dest.Close()
			rc.Close()
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
			os.MkdirAll(filepath.Dir(destPath), 0755)
			rc, _ := f.Open()
			dest, _ := os.Create(destPath)
			io.Copy(dest, rc)
			dest.Close()
			rc.Close()
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
