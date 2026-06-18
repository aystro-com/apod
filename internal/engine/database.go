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

	dumpCmd := dbDumpCmd(dbCfg.Type, dbName, dbUser, siteCreds)
	if dumpCmd == nil {
		return "", Invalid("unsupported database type: %s", dbCfg.Type)
	}

	// Capture stdout only — stderr warnings and exec frame headers would
	// corrupt the SQL.
	output, err := e.docker.ExecCaptureStdout(ctx, containerName, dumpCmd)
	if err != nil {
		return "", fmt.Errorf("database dump: %w", err)
	}

	e.LogActivity(domain, "db_export", "", "success")
	return string(output), nil
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
		return Invalid("site %q has no database configured", domain)
	}

	dbCfg := driver.Backup.Databases[0]
	containerName := fmt.Sprintf("apod-%s-%s", domain, dbCfg.Service)
	dbName := strings.ReplaceAll(domain, ".", "_")
	dbUser := dbName

	// Decode and replay the dump from a temp file (avoids shell-quoting the SQL).
	importCmd := dbRestoreCmd(dbCfg.Type, dbName, dbUser, base64Encode([]byte(dump)), siteCreds)
	if importCmd == nil {
		return Invalid("unsupported database type for import: %s", dbCfg.Type)
	}

	_, err = e.docker.ExecInContainer(ctx, containerName, importCmd)
	if err != nil {
		return fmt.Errorf("database import: %w", err)
	}

	e.LogActivity(domain, "db_import", "", "success")
	return nil
}
