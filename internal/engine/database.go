package engine

import (
	"context"
	"fmt"
	"strings"
)

func (e *Engine) DBExport(ctx context.Context, domain string) (string, error) {
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

	if len(driver.Backup.Databases) == 0 {
		return "", fmt.Errorf("site %q has no database configured", domain)
	}

	dbCfg := driver.Backup.Databases[0]
	containerName := fmt.Sprintf("apod-%s-%s", domain, dbCfg.Service)
	dbName := strings.ReplaceAll(domain, ".", "_")
	dbUser := dbName

	dumpCmd := dbDumpCommand(dbCfg.Type, dbName, dbUser, "backup")
	if dumpCmd == nil {
		return "", fmt.Errorf("unsupported database type: %s", dbCfg.Type)
	}

	output, err := e.docker.ExecInContainer(ctx, containerName, dumpCmd)
	if err != nil {
		return "", fmt.Errorf("database dump: %w", err)
	}

	e.LogActivity(domain, "db_export", "", "success")
	return output, nil
}

func (e *Engine) DBImport(ctx context.Context, domain, dump string) error {
	if err := e.locks.Acquire(domain); err != nil {
		return err
	}
	defer e.locks.Release(domain)

	site, err := e.db.GetSite(domain)
	if err != nil {
		return fmt.Errorf("get site: %w", err)
	}

	driver, err := e.drivers.Load(site.Driver)
	if err != nil {
		return fmt.Errorf("load driver: %w", err)
	}

	if len(driver.Backup.Databases) == 0 {
		return fmt.Errorf("site %q has no database configured", domain)
	}

	dbCfg := driver.Backup.Databases[0]
	containerName := fmt.Sprintf("apod-%s-%s", domain, dbCfg.Service)
	dbName := strings.ReplaceAll(domain, ".", "_")
	dbUser := dbName

	// Write dump to temp file in container, then restore
	var importCmd []string
	switch dbCfg.Type {
	case "mysql":
		importCmd = []string{"sh", "-c", fmt.Sprintf("echo '%s' | mysql -u%s -pbackup %s", dump, dbUser, dbName)}
	case "postgres":
		importCmd = []string{"sh", "-c", fmt.Sprintf("echo '%s' | psql -U %s %s", dump, dbUser, dbName)}
	default:
		return fmt.Errorf("unsupported database type for import: %s", dbCfg.Type)
	}

	_, err = e.docker.ExecInContainer(ctx, containerName, importCmd)
	if err != nil {
		return fmt.Errorf("database import: %w", err)
	}

	e.LogActivity(domain, "db_import", "", "success")
	return nil
}
