package engine

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func (e *Engine) Clone(ctx context.Context, sourceDomain, targetDomain string) error {
	if sourceDomain == targetDomain {
		return fmt.Errorf("source and target domain must be different")
	}

	if err := e.locks.Acquire(sourceDomain); err != nil {
		return err
	}
	defer e.locks.Release(sourceDomain)

	source, err := e.db.GetSite(sourceDomain)
	if err != nil {
		return fmt.Errorf("get source site: %w", err)
	}

	// Create target site with same config
	err = e.CreateSite(ctx, CreateSiteOpts{
		Domain: targetDomain,
		Driver: source.Driver,
		RAM:    source.RAM,
		CPU:    source.CPU,
		Repo:   source.Repo,
		Branch: source.Branch,
	})
	if err != nil {
		return fmt.Errorf("create target site: %w", err)
	}

	// Copy files
	sourceRoot := filepath.Join(e.dataDir, "sites", sourceDomain, "files")
	targetRoot := filepath.Join(e.dataDir, "sites", targetDomain, "files")

	err = copyDir(sourceRoot, targetRoot)
	if err != nil {
		return fmt.Errorf("copy files: %w", err)
	}

	// Copy env vars
	envs, _ := parseEnvJSON(source.Env)
	if len(envs) > 0 {
		envJSON, _ := envToJSON(envs)
		e.db.UpdateSiteConfig(targetDomain, map[string]string{"env": envJSON})
	}

	// Dump and import database
	driver, err := e.drivers.Load(source.Driver)
	if err == nil {
		dbName := strings.ReplaceAll(sourceDomain, ".", "_")
		dbUser := dbName
		targetDbName := strings.ReplaceAll(targetDomain, ".", "_")

		for _, dbCfg := range driver.Backup.Databases {
			sourceContainer := fmt.Sprintf("apod-%s-%s", sourceDomain, dbCfg.Service)
			targetContainer := fmt.Sprintf("apod-%s-%s", targetDomain, dbCfg.Service)

			// Dump from source
			dumpCmd := dbDumpCommand(dbCfg.Type, dbName, dbUser, "backup")
			if dumpCmd == nil {
				continue
			}
			output, err := e.docker.ExecInContainer(ctx, sourceContainer, dumpCmd)
			if err != nil {
				continue // non-fatal, site might not have a populated DB
			}

			// Import to target via base64 to avoid shell injection
			b64Dump := base64Encode([]byte(output))
			var restoreShell string
			switch dbCfg.Type {
			case "mysql":
				restoreShell = fmt.Sprintf("echo '%s' | base64 -d > /tmp/_apod_clone.sql && mysql -u%s -pbackup %s < /tmp/_apod_clone.sql && rm -f /tmp/_apod_clone.sql", b64Dump, targetDbName, targetDbName)
			case "postgres":
				restoreShell = fmt.Sprintf("echo '%s' | base64 -d > /tmp/_apod_clone.sql && psql -U %s -d %s -f /tmp/_apod_clone.sql && rm -f /tmp/_apod_clone.sql", b64Dump, targetDbName, targetDbName)
			}
			if restoreShell != "" {
				e.docker.ExecInContainer(ctx, targetContainer, []string{"sh", "-c", restoreShell})
			}
		}
	}

	e.LogActivity(sourceDomain, "clone", fmt.Sprintf("cloned to %s", targetDomain), "success")
	e.LogActivity(targetDomain, "clone", fmt.Sprintf("cloned from %s", sourceDomain), "success")
	return nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		relPath, _ := filepath.Rel(src, path)
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, 0755)
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer srcFile.Close()

		os.MkdirAll(filepath.Dir(dstPath), 0755)
		dstFile, err := os.Create(dstPath)
		if err != nil {
			return nil
		}
		defer dstFile.Close()

		_, err = io.Copy(dstFile, srcFile)
		return err
	})
}
