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
	{20, `ALTER TABLE sites ADD COLUMN storage TEXT NOT NULL DEFAULT '0'`},
	{21, `ALTER TABLE users ADD COLUMN password_hash TEXT NOT NULL DEFAULT ''`},
	{22, `CREATE TABLE IF NOT EXISTS sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		token_hash TEXT NOT NULL UNIQUE,
		user_name TEXT NOT NULL,
		expires_at DATETIME NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`},
	{23, `CREATE TABLE IF NOT EXISTS api_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_name TEXT NOT NULL,
		name TEXT NOT NULL,
		token_hash TEXT NOT NULL UNIQUE,
		abilities TEXT NOT NULL DEFAULT 'read',
		sensitive INTEGER NOT NULL DEFAULT 0,
		expires_at DATETIME,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`},
	{24, `ALTER TABLE users ADD COLUMN totp_secret TEXT NOT NULL DEFAULT ''`},
	{25, `ALTER TABLE users ADD COLUMN totp_enabled INTEGER NOT NULL DEFAULT 0`},
	{26, `ALTER TABLE users ADD COLUMN totp_last_step INTEGER NOT NULL DEFAULT 0`},
	{27, `ALTER TABLE users ADD COLUMN recovery_codes TEXT NOT NULL DEFAULT '[]'`},
	{28, `CREATE TABLE IF NOT EXISTS process_scaling (
		site_domain TEXT NOT NULL,
		service     TEXT NOT NULL,
		replicas    INTEGER NOT NULL,
		PRIMARY KEY (site_domain, service)
	)`},
	{29, `CREATE TABLE IF NOT EXISTS site_secrets (
		site_domain TEXT NOT NULL,
		key         TEXT NOT NULL,
		value       TEXT NOT NULL,
		PRIMARY KEY (site_domain, key)
	)`},
	{30, `CREATE TABLE IF NOT EXISTS shared_networks (
		name       TEXT PRIMARY KEY,
		owner      TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`},
	{31, `CREATE TABLE IF NOT EXISTS shared_network_members (
		network     TEXT NOT NULL,
		site_domain TEXT NOT NULL,
		PRIMARY KEY (network, site_domain)
	)`},
	{32, `ALTER TABLE users ADD COLUMN can_create_sites INTEGER NOT NULL DEFAULT 0`},
	// Enforce one rule per (site, ip). Dedup any pre-existing duplicates first
	// (keep the most recent), then add the unique index that AddIPRule's upsert
	// relies on. (mattn/go-sqlite3 runs both statements in the one migration tx.)
	{33, `DELETE FROM ip_rules WHERE rowid NOT IN (SELECT MAX(rowid) FROM ip_rules GROUP BY site_domain, ip);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_ip_rules_site_ip ON ip_rules(site_domain, ip);`},
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

	// Get current version. Fail closed on a read error — assuming version 0
	// would re-run every (non-idempotent) migration and brick startup.
	var currentVersion int
	if err := d.conn.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&currentVersion); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	// Run pending migrations. Each migration's DDL and its version record are
	// committed atomically so a crash mid-migration can't leave the schema in
	// a state that re-runs a non-idempotent ALTER on next boot.
	applied := 0
	for _, m := range migrations {
		if m.Version <= currentVersion {
			continue
		}
		tx, err := d.conn.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", m.Version, err)
		}
		if _, err := tx.Exec(m.SQL); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", m.Version, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, m.Version); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", m.Version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.Version, err)
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
