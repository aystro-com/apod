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
	Domain    string            `json:"domain"`
	Driver    string            `json:"driver"`
	RAM       string            `json:"ram"`
	CPU       string            `json:"cpu"`
	Env       map[string]string `json:"env"`
	Domains   []string          `json:"domains"`
	CreatedAt string            `json:"created_at"`
}

func (e *Engine) getStorage(ctx context.Context, storageName string) (storage.Storage, error) {
	if storageName == "" || storageName == "local" {
		return storage.NewLocal(filepath.Join(e.dataDir, "backups")), nil
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

	store, err := e.getStorage(ctx, storageName)
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
	for _, dbCfg := range driver.Backup.Databases {
		containerName := fmt.Sprintf("apod-%s-%s", domain, dbCfg.Service)
		dumpCmd := dbDumpCommand(dbCfg.Type, dbName, dbUser, "backup")
		if dumpCmd == nil {
			continue
		}
		output, err := e.docker.ExecInContainer(ctx, containerName, dumpCmd)
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
		Domain:    site.Domain,
		Driver:    site.Driver,
		RAM:       site.RAM,
		CPU:       site.CPU,
		Env:       envs,
		Domains:   domains,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	metaJSON, _ := json.MarshalIndent(meta, "", "  ")
	w, _ := zw.Create("metadata.json")
	w.Write(metaJSON)

	zw.Close()

	// Verify backup is not empty (metadata.json alone is ~200 bytes)
	if buf.Len() < 100 {
		e.LogActivity(domain, "backup", "backup appears empty", "failed")
		return 0, fmt.Errorf("backup verification failed: backup is empty")
	}

	// Upload
	if err := store.Upload(ctx, zipKey, bytes.NewReader(buf.Bytes())); err != nil {
		return 0, fmt.Errorf("upload backup: %w", err)
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

	store, err := e.getStorage(ctx, backup.StorageName)
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

	site, _ := e.db.GetSite(domain)
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
	}

	// Restart
	for _, id := range ids {
		e.docker.StartContainer(ctx, id)
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
	store, err := e.getStorage(ctx, backup.StorageName)
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
	// Validate path stays within backup directory to prevent path traversal
	backupDir := filepath.Join(e.dataDir, "backups")
	cleanPath := filepath.Clean(filepath.Join(backupDir, backup.Path))
	if !strings.HasPrefix(cleanPath, backupDir+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid backup path")
	}
	return cleanPath, nil
}
