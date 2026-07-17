package db

import (
	"database/sql"
	"fmt"
	"time"
)

type Backup struct {
	ID          int64     `json:"id"`
	SiteDomain  string    `json:"site_domain"`
	StorageName string    `json:"storage_name"`
	Path        string    `json:"path"`
	SizeBytes   int64     `json:"size_bytes"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

func (d *DB) CreateBackup(siteDomain, storageName, path string, sizeBytes int64) (int64, error) {
	result, err := d.conn.Exec(
		`INSERT INTO backups (site_domain, storage_name, path, size_bytes) VALUES (?, ?, ?, ?)`,
		siteDomain, storageName, path, sizeBytes,
	)
	if err != nil {
		return 0, fmt.Errorf("create backup record: %w", err)
	}
	return result.LastInsertId()
}

func (d *DB) GetBackup(id int64) (*Backup, error) {
	b := &Backup{}
	err := d.conn.QueryRow(
		`SELECT id, site_domain, storage_name, path, size_bytes, status, created_at FROM backups WHERE id = ?`, id,
	).Scan(&b.ID, &b.SiteDomain, &b.StorageName, &b.Path, &b.SizeBytes, &b.Status, &b.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("backup %d not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("query backup: %w", err)
	}
	return b, nil
}

func (d *DB) ListBackups(siteDomain string) ([]Backup, error) {
	rows, err := d.conn.Query(
		`SELECT id, site_domain, storage_name, path, size_bytes, status, created_at
		 FROM backups WHERE site_domain = ? ORDER BY created_at DESC`, siteDomain,
	)
	if err != nil {
		return nil, fmt.Errorf("query backups: %w", err)
	}
	defer rows.Close()

	var backups []Backup
	for rows.Next() {
		var b Backup
		if err := rows.Scan(&b.ID, &b.SiteDomain, &b.StorageName, &b.Path, &b.SizeBytes, &b.Status, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan backup: %w", err)
		}
		backups = append(backups, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}
	return backups, nil
}

func (d *DB) DeleteBackup(id int64) error {
	result, err := d.conn.Exec(`DELETE FROM backups WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete backup: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("backup %d not found", id)
	}
	return nil
}

func (d *DB) DeleteOldestBackups(siteDomain, storageName string, keep int) ([]string, error) {
	rows, err := d.conn.Query(
		// id breaks created_at ties (1s resolution): two backups in the same
		// second would otherwise order arbitrarily, letting retention keep the
		// stale one and delete the newest.
		`SELECT id, path FROM backups WHERE site_domain = ? AND storage_name = ? ORDER BY created_at DESC, id DESC LIMIT -1 OFFSET ?`,
		siteDomain, storageName, keep,
	)
	if err != nil {
		return nil, fmt.Errorf("query old backups: %w", err)
	}
	defer rows.Close()

	type oldBackup struct {
		id   int64
		path string
	}
	var old []oldBackup
	for rows.Next() {
		var b oldBackup
		if err := rows.Scan(&b.id, &b.path); err != nil {
			return nil, fmt.Errorf("scan old backup: %w", err)
		}
		old = append(old, b)
	}
	// A cursor error means the candidate list may be truncated — bail before
	// deleting anything rather than pruning from an incomplete view.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate old backups: %w", err)
	}

	// Only return paths whose DB row was actually deleted, so the caller never
	// removes a storage object while its record still exists (dangling backup).
	var paths []string
	for _, b := range old {
		if _, err := d.conn.Exec(`DELETE FROM backups WHERE id = ?`, b.id); err != nil {
			return paths, fmt.Errorf("delete old backup %d: %w", b.id, err)
		}
		paths = append(paths, b.path)
	}

	return paths, nil
}
