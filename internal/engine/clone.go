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

	driver, err := e.drivers.Load(source.Driver)
	if err != nil {
		return fmt.Errorf("load driver: %w", err)
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

	isCompose := driver.Type == "compose"

	sourceRoot, sourceData := e.SiteDir(source.Owner, sourceDomain)
	target, _ := e.db.GetSite(targetDomain)
	targetRoot, targetData := e.SiteDir(target.Owner, targetDomain)

	if isCompose {
		// For compose sites: only copy storage/upload files, NOT raw DB data or compose config.
		// The target already has its own compose setup with fresh secrets.
		// We copy driver-defined backup paths (e.g., storage files).
		for _, p := range driver.Backup.Paths {
			srcPath := strings.ReplaceAll(p, "${site_root}", sourceRoot)
			srcPath = strings.ReplaceAll(srcPath, "${data_root}", sourceData)
			dstPath := strings.ReplaceAll(p, "${site_root}", targetRoot)
			dstPath = strings.ReplaceAll(dstPath, "${data_root}", targetData)
			copyDir(srcPath, dstPath)
		}
	} else {
		// Normal sites: copy all files and data
		copyDir(sourceRoot, targetRoot)
		copyDir(sourceData, targetData)
	}

	// Copy env vars
	envs, _ := parseEnvJSON(source.Env)
	if len(envs) > 0 {
		envJSON, _ := envToJSON(envs)
		e.db.UpdateSiteConfig(targetDomain, map[string]string{"env": envJSON})
	}

	// Dump and import database
	dbName := strings.ReplaceAll(sourceDomain, ".", "_")
	dbUser := dbName
	targetDbName := strings.ReplaceAll(targetDomain, ".", "_")

	for _, dbCfg := range driver.Backup.Databases {
		// Dump from source
		var dumpCmd []string
		if isCompose {
			dumpCmd = composeDumpCommand(dbCfg.Type)
		} else {
			dumpCmd = dbDumpCommand(dbCfg.Type, dbName, dbUser, "backup")
		}
		if dumpCmd == nil {
			continue
		}

		var output string
		if isCompose {
			output, err = e.ExecInComposeSite(ctx, sourceDomain, source.Owner, dbCfg.Service, dumpCmd)
		} else {
			sourceContainer := fmt.Sprintf("apod-%s-%s", sourceDomain, dbCfg.Service)
			output, err = e.docker.ExecInContainer(ctx, sourceContainer, dumpCmd)
		}
		if err != nil {
			continue
		}

		// Import to target
		b64Dump := base64Encode([]byte(output))
		var restoreShell string
		if isCompose {
			// Compose: use default user
			switch dbCfg.Type {
			case "mysql":
				restoreShell = fmt.Sprintf("echo '%s' | base64 -d > /tmp/_apod_clone.sql && mysql -u root -p\"$MYSQL_ROOT_PASSWORD\" < /tmp/_apod_clone.sql && rm -f /tmp/_apod_clone.sql", b64Dump)
			case "postgres":
				restoreShell = fmt.Sprintf("echo '%s' | base64 -d > /tmp/_apod_clone.sql && psql -U \"${POSTGRES_USER:-postgres}\" -f /tmp/_apod_clone.sql && rm -f /tmp/_apod_clone.sql", b64Dump)
			}
			if restoreShell != "" {
				e.ExecInComposeSite(ctx, targetDomain, target.Owner, dbCfg.Service, []string{"sh", "-c", restoreShell})
			}
		} else {
			switch dbCfg.Type {
			case "mysql":
				restoreShell = fmt.Sprintf("echo '%s' | base64 -d > /tmp/_apod_clone.sql && mysql -u%s -pbackup %s < /tmp/_apod_clone.sql && rm -f /tmp/_apod_clone.sql", b64Dump, targetDbName, targetDbName)
			case "postgres":
				restoreShell = fmt.Sprintf("echo '%s' | base64 -d > /tmp/_apod_clone.sql && psql -U %s -d %s -f /tmp/_apod_clone.sql && rm -f /tmp/_apod_clone.sql", b64Dump, targetDbName, targetDbName)
			}
			if restoreShell != "" {
				targetContainer := fmt.Sprintf("apod-%s-%s", targetDomain, dbCfg.Service)
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
