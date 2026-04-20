package db

import (
	"database/sql"
	"fmt"

	"github.com/aystro/apod/internal/models"
)

func (d *DB) AddDomain(siteID int64, domain string, isPrimary bool) error {
	primary := 0
	if isPrimary {
		primary = 1
	}
	_, err := d.conn.Exec(
		`INSERT INTO domains (site_id, domain, is_primary) VALUES (?, ?, ?)`,
		siteID, domain, primary,
	)
	if err != nil {
		return fmt.Errorf("add domain %q: %w", domain, err)
	}
	return nil
}

func (d *DB) RemoveDomain(domain string) error {
	result, err := d.conn.Exec(`DELETE FROM domains WHERE domain = ?`, domain)
	if err != nil {
		return fmt.Errorf("remove domain: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("domain %q not found", domain)
	}
	return nil
}

func (d *DB) ListDomains(siteID int64) ([]string, error) {
	rows, err := d.conn.Query(
		`SELECT domain FROM domains WHERE site_id = ? ORDER BY is_primary DESC, domain`,
		siteID,
	)
	if err != nil {
		return nil, fmt.Errorf("query domains: %w", err)
	}
	defer rows.Close()

	var domains []string
	for rows.Next() {
		var domain string
		if err := rows.Scan(&domain); err != nil {
			return nil, fmt.Errorf("scan domain: %w", err)
		}
		domains = append(domains, domain)
	}
	return domains, nil
}

func (d *DB) GetSiteByDomain(domain string) (*models.Site, error) {
	var siteDomain string
	err := d.conn.QueryRow(
		`SELECT s.domain FROM sites s JOIN domains d ON d.site_id = s.id WHERE d.domain = ?`, domain,
	).Scan(&siteDomain)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no site found for domain %q", domain)
	}
	if err != nil {
		return nil, fmt.Errorf("query site by domain: %w", err)
	}
	return d.GetSite(siteDomain)
}
