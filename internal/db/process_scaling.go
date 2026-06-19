package db

import (
	"database/sql"
	"errors"
	"fmt"
)

// SetProcessReplicas records a per-service replica override for a site,
// replacing any existing value (including 0, to pause a worker).
func (d *DB) SetProcessReplicas(siteDomain, service string, replicas int) error {
	_, err := d.conn.Exec(
		`INSERT INTO process_scaling (site_domain, service, replicas) VALUES (?, ?, ?)
		 ON CONFLICT(site_domain, service) DO UPDATE SET replicas = excluded.replicas`,
		siteDomain, service, replicas,
	)
	if err != nil {
		return fmt.Errorf("set process replicas: %w", err)
	}
	return nil
}

// GetProcessReplicas returns the override for a service, with ok=false when no
// override is set (the driver default then applies).
func (d *DB) GetProcessReplicas(siteDomain, service string) (n int, ok bool, err error) {
	row := d.conn.QueryRow(
		`SELECT replicas FROM process_scaling WHERE site_domain = ? AND service = ?`,
		siteDomain, service,
	)
	switch scanErr := row.Scan(&n); scanErr {
	case nil:
		return n, true, nil
	default:
		// sql.ErrNoRows => treat as "no override".
		if errors.Is(scanErr, sql.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, scanErr
	}
}

// ListProcessScaling returns all service->replicas overrides for a site.
func (d *DB) ListProcessScaling(siteDomain string) (map[string]int, error) {
	rows, err := d.conn.Query(
		`SELECT service, replicas FROM process_scaling WHERE site_domain = ?`,
		siteDomain,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]int)
	for rows.Next() {
		var svc string
		var n int
		if err := rows.Scan(&svc, &n); err != nil {
			return nil, err
		}
		out[svc] = n
	}
	return out, rows.Err()
}

// DeleteProcessScaling removes all overrides for a site (used on destroy).
func (d *DB) DeleteProcessScaling(siteDomain string) error {
	_, err := d.conn.Exec(`DELETE FROM process_scaling WHERE site_domain = ?`, siteDomain)
	return err
}
