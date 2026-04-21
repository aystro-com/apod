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
func (e *Engine) ExportSite(ctx context.Context, domain, outputDir string) (string, error) {
	if err := e.locks.Acquire(domain); err != nil {
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

	f, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("create export file: %w", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)

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
			e.LogActivity(domain, "export_warning", fmt.Sprintf("db dump failed for %s: %v", dbCfg.Service, err), "warning")
			continue
		}
		if len(strings.TrimSpace(output)) == 0 {
			continue
		}
		w, _ := zw.Create(fmt.Sprintf("databases/%s_%s.sql.gz", dbCfg.Service, dbCfg.Type))
		gz := gzip.NewWriter(w)
		gz.Write([]byte(output))
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

	// Verify
	info, _ := os.Stat(outputPath)
	if info == nil || info.Size() < 100 {
		os.Remove(outputPath)
		return "", fmt.Errorf("export verification failed: file is empty")
	}

	e.LogActivity(domain, "export", fmt.Sprintf("exported to %s (%d bytes)", outputPath, info.Size()), "success")
	return outputPath, nil
}

// ImportSite creates a new site from an export zip file.
// The zip must contain metadata.json with the site config.
// Optionally override the domain with newDomain (empty = use domain from metadata).
func (e *Engine) ImportSite(ctx context.Context, zipPath, newDomain, owner string) error {
	data, err := os.ReadFile(zipPath)
	if err != nil {
		return fmt.Errorf("read export file: %w", err)
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

	domain := newDomain
	if domain == "" {
		domain = meta.Domain
	}

	// Create the site using the driver and config from metadata
	err = e.CreateSite(ctx, CreateSiteOpts{
		Domain: domain,
		Driver: meta.Driver,
		RAM:    meta.RAM,
		CPU:    meta.CPU,
		Owner:  owner,
	})
	if err != nil {
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

	// Extract files and data
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
			os.MkdirAll(filepath.Dir(destPath), 0755)
			rc, _ := f.Open()
			dest, _ := os.Create(destPath)
			io.Copy(dest, rc)
			dest.Close()
			rc.Close()
		}
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
	}

	// Import databases
	driver, err := e.drivers.Load(meta.Driver)
	if err == nil {
		dbName := strings.ReplaceAll(domain, ".", "_")
		dbUser := dbName

		for _, dbCfg := range driver.Backup.Databases {
			containerName := fmt.Sprintf("apod-%s-%s", domain, dbCfg.Service)

			// Find matching dump in zip
			dumpPrefix := fmt.Sprintf("databases/%s_%s.sql", dbCfg.Service, dbCfg.Type)
			for _, f := range zr.File {
				if !strings.HasPrefix(f.Name, dumpPrefix) {
					continue
				}

				rc, _ := f.Open()
				var dump []byte
				if strings.HasSuffix(f.Name, ".gz") {
					gz, err := gzip.NewReader(rc)
					if err == nil {
						dump, _ = io.ReadAll(gz)
						gz.Close()
					}
				} else {
					dump, _ = io.ReadAll(rc)
				}
				rc.Close()

				if len(dump) == 0 {
					continue
				}

				// Import via base64 to avoid shell injection
				b64Dump := base64Encode(dump)
				var importShell string
				switch dbCfg.Type {
				case "mysql":
					importShell = fmt.Sprintf("echo '%s' | base64 -d > /tmp/_apod_import.sql && mysql -u%s -pbackup %s < /tmp/_apod_import.sql && rm -f /tmp/_apod_import.sql", b64Dump, dbUser, dbName)
				case "postgres":
					importShell = fmt.Sprintf("echo '%s' | base64 -d > /tmp/_apod_import.sql && psql -U %s %s < /tmp/_apod_import.sql && rm -f /tmp/_apod_import.sql", b64Dump, dbUser, dbName)
				}
				if importShell != "" {
					e.docker.ExecInContainer(ctx, containerName, []string{"sh", "-c", importShell})
				}
				break
			}
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

	// Restart containers to pick up restored files
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
