package db

import "fmt"

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS sites (
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
	)`,
	`CREATE TABLE IF NOT EXISTS domains (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		site_id INTEGER NOT NULL,
		domain TEXT NOT NULL UNIQUE,
		is_primary INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (site_id) REFERENCES sites(id) ON DELETE CASCADE
	)`,
	`CREATE TABLE IF NOT EXISTS operations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		site_domain TEXT NOT NULL,
		action TEXT NOT NULL,
		details TEXT NOT NULL DEFAULT '',
		result TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS api_keys (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		key_hash TEXT NOT NULL UNIQUE,
		scope TEXT NOT NULL DEFAULT '*',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
}

func (d *DB) migrate() error {
	for i, m := range migrations {
		if _, err := d.conn.Exec(m); err != nil {
			return fmt.Errorf("migration %d: %w", i, err)
		}
	}
	return nil
}
