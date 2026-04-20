package db

import (
	"database/sql"
	"fmt"

	"github.com/aystro/apod/internal/models"
)

func (d *DB) CreateSite(site *models.Site) error {
	result, err := d.conn.Exec(
		`INSERT INTO sites (domain, driver, status, ram, cpu, env, repo, branch)
		 VALUES (?, ?, 'creating', ?, ?, '{}', ?, ?)`,
		site.Domain, site.Driver, site.RAM, site.CPU, site.Repo, site.Branch,
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
		`SELECT id, domain, driver, status, ram, cpu, env, repo, branch, created_at, updated_at
		 FROM sites WHERE domain = ?`, domain,
	).Scan(&site.ID, &site.Domain, &site.Driver, &site.Status, &site.RAM, &site.CPU,
		&site.Env, &site.Repo, &site.Branch, &site.CreatedAt, &site.UpdatedAt)
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
		`SELECT id, domain, driver, status, ram, cpu, env, repo, branch, created_at, updated_at
		 FROM sites ORDER BY domain`,
	)
	if err != nil {
		return nil, fmt.Errorf("query sites: %w", err)
	}
	defer rows.Close()

	var sites []models.Site
	for rows.Next() {
		var s models.Site
		if err := rows.Scan(&s.ID, &s.Domain, &s.Driver, &s.Status, &s.RAM, &s.CPU,
			&s.Env, &s.Repo, &s.Branch, &s.CreatedAt, &s.UpdatedAt); err != nil {
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
