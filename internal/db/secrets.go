package db

import "fmt"

// SetSiteSecret stores (or replaces) a generated secret for a site — the
// authoritative record for values like the database password, so backup, clone
// and rotation read them instead of reverse-engineering from container env/.env.
func (d *DB) SetSiteSecret(siteDomain, key, value string) error {
	_, err := d.conn.Exec(
		`INSERT INTO site_secrets (site_domain, key, value) VALUES (?, ?, ?)
		 ON CONFLICT(site_domain, key) DO UPDATE SET value = excluded.value`,
		siteDomain, key, value,
	)
	if err != nil {
		return fmt.Errorf("set site secret: %w", err)
	}
	return nil
}

// GetSiteSecret returns a site's secret, with ok=false when it is not set.
func (d *DB) GetSiteSecret(siteDomain, key string) (value string, ok bool, err error) {
	row := d.conn.QueryRow(
		`SELECT value FROM site_secrets WHERE site_domain = ? AND key = ?`,
		siteDomain, key,
	)
	switch scanErr := row.Scan(&value); {
	case scanErr == nil:
		return value, true, nil
	case scanErr.Error() == "sql: no rows in result set":
		return "", false, nil
	default:
		return "", false, scanErr
	}
}

// DeleteSiteSecrets removes all secrets for a site (used on destroy).
func (d *DB) DeleteSiteSecrets(siteDomain string) error {
	_, err := d.conn.Exec(`DELETE FROM site_secrets WHERE site_domain = ?`, siteDomain)
	return err
}
