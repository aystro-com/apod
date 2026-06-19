package db

import (
	"database/sql"
	"fmt"

	"github.com/aystro/apod/internal/models"
)

func (d *DB) CreateSite(site *models.Site) error {
	result, err := d.conn.Exec(
		`INSERT INTO sites (domain, driver, status, ram, cpu, storage, env, repo, branch, owner)
		 VALUES (?, ?, 'creating', ?, ?, ?, '{}', ?, ?, ?)`,
		site.Domain, site.Driver, site.RAM, site.CPU, site.Storage, site.Repo, site.Branch, site.Owner,
	)
	if err != nil {
		return fmt.Errorf("insert site: %w", err)
	}
	id, _ := result.LastInsertId()
	site.ID = id
	return nil
}

func (d *DB) GetSite(domain string) (*models.Site, error) {
	site := &models.Site{}
	err := d.conn.QueryRow(
		`SELECT id, domain, driver, status, ram, cpu, storage, env, repo, branch, owner, created_at, updated_at
		 FROM sites WHERE domain = ?`, domain,
	).Scan(&site.ID, &site.Domain, &site.Driver, &site.Status, &site.RAM, &site.CPU, &site.Storage,
		&site.Env, &site.Repo, &site.Branch, &site.Owner, &site.CreatedAt, &site.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("site %q not found", domain)
	}
	if err != nil {
		return nil, fmt.Errorf("query site: %w", err)
	}
	return site, nil
}

func (d *DB) ListSites() ([]models.Site, error) {
	rows, err := d.conn.Query(
		`SELECT id, domain, driver, status, ram, cpu, storage, env, repo, branch, owner, created_at, updated_at
		 FROM sites ORDER BY domain`,
	)
	if err != nil {
		return nil, fmt.Errorf("query sites: %w", err)
	}
	defer rows.Close()

	var sites []models.Site
	for rows.Next() {
		var s models.Site
		if err := rows.Scan(&s.ID, &s.Domain, &s.Driver, &s.Status, &s.RAM, &s.CPU, &s.Storage,
			&s.Env, &s.Repo, &s.Branch, &s.Owner, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan site: %w", err)
		}
		sites = append(sites, s)
	}
	return sites, nil
}

func (d *DB) ListSitesByOwner(owner string) ([]models.Site, error) {
	rows, err := d.conn.Query(
		`SELECT id, domain, driver, status, ram, cpu, storage, env, repo, branch, owner, created_at, updated_at
		 FROM sites WHERE owner = ? ORDER BY domain`, owner,
	)
	if err != nil {
		return nil, fmt.Errorf("query sites: %w", err)
	}
	defer rows.Close()

	var sites []models.Site
	for rows.Next() {
		var s models.Site
		if err := rows.Scan(&s.ID, &s.Domain, &s.Driver, &s.Status, &s.RAM, &s.CPU, &s.Storage,
			&s.Env, &s.Repo, &s.Branch, &s.Owner, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan site: %w", err)
		}
		sites = append(sites, s)
	}
	return sites, nil
}

func (d *DB) UpdateSiteStatus(domain, status string) error {
	result, err := d.conn.Exec(
		`UPDATE sites SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE domain = ?`,
		status, domain,
	)
	if err != nil {
		return fmt.Errorf("update site status: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("site %q not found", domain)
	}
	return nil
}

func (d *DB) UpdateSiteConfig(domain string, fields map[string]string) error {
	for key, value := range fields {
		var query string
		switch key {
		case "ram":
			query = `UPDATE sites SET ram = ?, updated_at = CURRENT_TIMESTAMP WHERE domain = ?`
		case "cpu":
			query = `UPDATE sites SET cpu = ?, updated_at = CURRENT_TIMESTAMP WHERE domain = ?`
		case "storage":
			query = `UPDATE sites SET storage = ?, updated_at = CURRENT_TIMESTAMP WHERE domain = ?`
		case "env":
			query = `UPDATE sites SET env = ?, updated_at = CURRENT_TIMESTAMP WHERE domain = ?`
		case "repo":
			query = `UPDATE sites SET repo = ?, updated_at = CURRENT_TIMESTAMP WHERE domain = ?`
		case "branch":
			query = `UPDATE sites SET branch = ?, updated_at = CURRENT_TIMESTAMP WHERE domain = ?`
		default:
			return fmt.Errorf("unknown config field: %s", key)
		}
		result, err := d.conn.Exec(query, value, domain)
		if err != nil {
			return fmt.Errorf("update %s: %w", key, err)
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			return fmt.Errorf("site %q not found", domain)
		}
	}
	return nil
}

func (d *DB) UpdateSiteOwner(domain, owner string) error {
	result, err := d.conn.Exec(
		`UPDATE sites SET owner = ?, updated_at = CURRENT_TIMESTAMP WHERE domain = ?`,
		owner, domain,
	)
	if err != nil {
		return fmt.Errorf("update site owner: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("site %q not found", domain)
	}
	return nil
}

func (d *DB) DeleteSite(domain string) error {
	result, err := d.conn.Exec(`DELETE FROM sites WHERE domain = ?`, domain)
	if err != nil {
		return fmt.Errorf("delete site: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("site %q not found", domain)
	}
	return nil
}

// childTablesByDomain lists the per-site tables keyed on a plain `site_domain`
// TEXT column (no FK/cascade), which therefore must be cleaned explicitly when
// a site is destroyed. Otherwise the rows are orphaned and silently inherited
// by a future site that reuses the same domain — a cross-tenant leak (stale
// cron commands re-run, old webhook tokens still deploy, etc.).
var childTablesByDomain = []string{
	"backups", "deployments", "cron_jobs", "proxy_rules", "ip_rules",
	"ftp_accounts", "backup_schedules", "webhooks", "uptime_logs",
}

// DeleteSiteChildData removes every per-domain child row for a site in one
// transaction. Call it on every destroy path so a reused domain never inherits
// the previous tenant's data. (uptime_checks, operations, process_scaling,
// site_secrets and network membership are cleaned separately by the engine.)
func (d *DB) DeleteSiteChildData(domain string) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return fmt.Errorf("begin child-data cleanup: %w", err)
	}
	defer tx.Rollback()
	for _, table := range childTablesByDomain {
		// table names are from a fixed internal allowlist, never user input.
		if _, err := tx.Exec(`DELETE FROM `+table+` WHERE site_domain = ?`, domain); err != nil {
			return fmt.Errorf("delete %s rows for %q: %w", table, domain, err)
		}
	}
	return tx.Commit()
}
