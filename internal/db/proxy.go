package db

import (
	"fmt"
	"time"
)

type ProxyRule struct {
	ID         int64     `json:"id"`
	SiteDomain string    `json:"site_domain"`
	RuleType   string    `json:"rule_type"`
	Config     string    `json:"config"`
	CreatedAt  time.Time `json:"created_at"`
}

func (d *DB) CreateProxyRule(siteDomain, ruleType, config string) (int64, error) {
	result, err := d.conn.Exec(`INSERT INTO proxy_rules (site_domain, rule_type, config) VALUES (?, ?, ?)`, siteDomain, ruleType, config)
	if err != nil { return 0, fmt.Errorf("create proxy rule: %w", err) }
	return result.LastInsertId()
}

func (d *DB) ListProxyRules(siteDomain string) ([]ProxyRule, error) {
	rows, err := d.conn.Query(`SELECT id, site_domain, rule_type, config, created_at FROM proxy_rules WHERE site_domain = ? ORDER BY id`, siteDomain)
	if err != nil { return nil, err }
	defer rows.Close()
	var rules []ProxyRule
	for rows.Next() {
		var r ProxyRule
		if err := rows.Scan(&r.ID, &r.SiteDomain, &r.RuleType, &r.Config, &r.CreatedAt); err != nil { return nil, err }
		rules = append(rules, r)
	}
	return rules, nil
}

func (d *DB) DeleteProxyRule(id int64) error {
	result, err := d.conn.Exec(`DELETE FROM proxy_rules WHERE id = ?`, id)
	if err != nil { return err }
	n, _ := result.RowsAffected()
	if n == 0 { return fmt.Errorf("proxy rule %d not found", id) }
	return nil
}
