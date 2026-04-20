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

	tables := []string{"sites", "domains", "operations", "api_keys", "backups", "storage_configs", "backup_schedules", "deployments", "webhooks", "uptime_checks", "uptime_logs", "cron_jobs"}
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
