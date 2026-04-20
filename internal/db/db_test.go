package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenAndMigrate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	d, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	tables := []string{"schema_migrations", "sites", "domains", "operations", "api_keys", "backups", "storage_configs", "backup_schedules", "deployments", "webhooks", "uptime_checks", "uptime_logs", "cron_jobs", "proxy_rules", "ip_rules", "ftp_accounts", "ssh_keys"}
	for _, table := range tables {
		var name string
		err := d.Conn().QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %s not found: %v", table, err)
		}
	}
}

func TestMigrationVersioning(t *testing.T) {
	d := openTestDB(t)

	// Check version is set
	version := d.CurrentVersion()
	if version == 0 {
		t.Error("expected version > 0 after migrations")
	}

	// Running migrate again should be idempotent (no errors)
	if err := d.migrate(); err != nil {
		t.Fatalf("re-run migrate: %v", err)
	}

	// Version should be the same
	v2 := d.CurrentVersion()
	if v2 != version {
		t.Errorf("version changed after re-migrate: %d -> %d", version, v2)
	}
}

func TestOpenCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "test.db")

	d, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	if _, err := os.Stat(filepath.Dir(path)); os.IsNotExist(err) {
		t.Error("expected directory to be created")
	}
}
