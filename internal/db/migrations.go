package db

import (
	"fmt"
	"log"
)

// Each migration is numbered and runs exactly once.
// NEVER modify existing migrations — only append new ones.
var migrations = []struct {
	Version int
	SQL     string
}{
	{1, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`},
	{2, `CREATE TABLE IF NOT EXISTS sites (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		domain TEXT NOT NULL UNIQUE,
		driver TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'creating',
		ram TEXT NOT NULL DEFAULT '256M',
		cpu TEXT NOT NULL DEFAULT '1',
		env TEXT NOT NULL DEFAULT '{}',
		repo TEXT NOT NULL DEFAULT '',
		branch TEXT NOT NULL DEFAULT 'main',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`},
	{3, `CREATE TABLE IF NOT EXISTS domains (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		site_id INTEGER NOT NULL,
		domain TEXT NOT NULL UNIQUE,
		is_primary INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (site_id) REFERENCES sites(id) ON DELETE CASCADE
	)`},
	{4, `CREATE TABLE IF NOT EXISTS operations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		site_domain TEXT NOT NULL,
		action TEXT NOT NULL,
		details TEXT NOT NULL DEFAULT '',
		result TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`},
	{5, `CREATE TABLE IF NOT EXISTS api_keys (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		key_hash TEXT NOT NULL UNIQUE,
		scope TEXT NOT NULL DEFAULT '*',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`},
	{6, `CREATE TABLE IF NOT EXISTS backups (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		site_domain TEXT NOT NULL,
		storage_name TEXT NOT NULL DEFAULT 'local',
		path TEXT NOT NULL,
		size_bytes INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'completed',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`},
	{7, `CREATE TABLE IF NOT EXISTS storage_configs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		driver TEXT NOT NULL,
		config TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`},
	{8, `CREATE TABLE IF NOT EXISTS backup_schedules (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		site_domain TEXT NOT NULL,
		cron_expr TEXT NOT NULL,
		storage_name TEXT NOT NULL DEFAULT 'local',
		keep_count INTEGER NOT NULL DEFAULT 7,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`},
	{9, `CREATE TABLE IF NOT EXISTS deployments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		site_domain TEXT NOT NULL,
		commit_hash TEXT NOT NULL DEFAULT '',
		branch TEXT NOT NULL DEFAULT 'main',
		status TEXT NOT NULL DEFAULT 'pending',
		previous_image TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`},
	{10, `CREATE TABLE IF NOT EXISTS webhooks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		site_domain TEXT NOT NULL,
		token TEXT NOT NULL UNIQUE,
		active INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`},
	{11, `CREATE TABLE IF NOT EXISTS uptime_checks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		site_domain TEXT NOT NULL UNIQUE,
		url TEXT NOT NULL,
		interval_seconds INTEGER NOT NULL DEFAULT 60,
		alert_webhook TEXT NOT NULL DEFAULT '',
		active INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`},
	{12, `CREATE TABLE IF NOT EXISTS uptime_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		site_domain TEXT NOT NULL,
		status_code INTEGER NOT NULL,
		response_ms INTEGER NOT NULL,
		is_up INTEGER NOT NULL DEFAULT 1,
		checked_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`},
	{13, `CREATE TABLE IF NOT EXISTS cron_jobs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		site_domain TEXT NOT NULL,
		schedule TEXT NOT NULL,
		command TEXT NOT NULL,
		service TEXT NOT NULL DEFAULT 'app',
		active INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`},
	{14, `CREATE TABLE IF NOT EXISTS proxy_rules (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		site_domain TEXT NOT NULL,
		rule_type TEXT NOT NULL,
		config TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`},
	{15, `CREATE TABLE IF NOT EXISTS ip_rules (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		site_domain TEXT NOT NULL,
		ip TEXT NOT NULL,
		action TEXT NOT NULL DEFAULT 'block',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`},
	{16, `CREATE TABLE IF NOT EXISTS ftp_accounts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		site_domain TEXT NOT NULL,
		username TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`},
	{17, `CREATE TABLE IF NOT EXISTS ssh_keys (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		public_key TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`},
	{18, `CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		uid INTEGER NOT NULL UNIQUE,
		role TEXT NOT NULL DEFAULT 'user',
		api_key_hash TEXT NOT NULL UNIQUE,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`},
	{19, `ALTER TABLE sites ADD COLUMN owner TEXT NOT NULL DEFAULT ''`},
}

func (d *DB) migrate() error {
	// Ensure schema_migrations table exists (bootstrap)
	_, err := d.conn.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	// Get current version
	var currentVersion int
	d.conn.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&currentVersion)

	// Run pending migrations
	applied := 0
	for _, m := range migrations {
		if m.Version <= currentVersion {
			continue
		}
		if _, err := d.conn.Exec(m.SQL); err != nil {
			return fmt.Errorf("migration %d: %w", m.Version, err)
		}
		if _, err := d.conn.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, m.Version); err != nil {
			return fmt.Errorf("record migration %d: %w", m.Version, err)
		}
		applied++
	}

	if applied > 0 {
		log.Printf("applied %d database migration(s) (now at version %d)", applied, migrations[len(migrations)-1].Version)
	}

	return nil
}

func (d *DB) CurrentVersion() int {
	var v int
	d.conn.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&v)
	return v
}
