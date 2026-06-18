package db

import (
	"fmt"
	"time"
)

type IPRule struct {
	ID         int64     `json:"id"`
	SiteDomain string    `json:"site_domain"`
	IP         string    `json:"ip"`
	Action     string    `json:"action"`
	CreatedAt  time.Time `json:"created_at"`
}

func (d *DB) BlockIP(siteDomain, ip string) error {
	return d.AddIPRule(siteDomain, ip, "block")
}

// AddIPRule inserts an allow or block rule for a site. A given IP has at most
// one rule per site, so an existing rule for the same IP is replaced.
func (d *DB) AddIPRule(siteDomain, ip, action string) error {
	if action != "allow" && action != "block" {
		return fmt.Errorf("invalid action %q", action)
	}
	if _, err := d.conn.Exec(`DELETE FROM ip_rules WHERE site_domain = ? AND ip = ?`, siteDomain, ip); err != nil {
		return fmt.Errorf("replace IP rule: %w", err)
	}
	if _, err := d.conn.Exec(`INSERT INTO ip_rules (site_domain, ip, action) VALUES (?, ?, ?)`, siteDomain, ip, action); err != nil {
		return fmt.Errorf("add IP rule: %w", err)
	}
	return nil
}

// UnblockIP removes any rule for the given IP on a site. Scoped to site_domain
// so it cannot affect another tenant's rules.
func (d *DB) UnblockIP(siteDomain, ip string) error {
	_, err := d.conn.Exec(`DELETE FROM ip_rules WHERE site_domain = ? AND ip = ?`, siteDomain, ip)
	return err
}

func (d *DB) ListIPRules(siteDomain string) ([]IPRule, error) {
	rows, err := d.conn.Query(`SELECT id, site_domain, ip, action, created_at FROM ip_rules WHERE site_domain = ? ORDER BY id`, siteDomain)
	if err != nil { return nil, err }
	defer rows.Close()
	var rules []IPRule
	for rows.Next() {
		var r IPRule
		if err := rows.Scan(&r.ID, &r.SiteDomain, &r.IP, &r.Action, &r.CreatedAt); err != nil { return nil, err }
		rules = append(rules, r)
	}
	return rules, nil
}
