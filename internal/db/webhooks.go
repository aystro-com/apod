package db

import (
	"database/sql"
	"fmt"
	"time"
)

type Webhook struct {
	ID         int64     `json:"id"`
	SiteDomain string    `json:"site_domain"`
	Token      string    `json:"token"`
	Active     bool      `json:"active"`
	CreatedAt  time.Time `json:"created_at"`
}

func (d *DB) CreateWebhook(siteDomain, token string) error {
	_, err := d.conn.Exec(`INSERT INTO webhooks (site_domain, token) VALUES (?, ?)`, siteDomain, token)
	if err != nil {
		return fmt.Errorf("create webhook: %w", err)
	}
	return nil
}

func (d *DB) GetWebhookByToken(token string) (*Webhook, error) {
	wh := &Webhook{}
	var active int
	err := d.conn.QueryRow(
		`SELECT id, site_domain, token, active, created_at FROM webhooks WHERE token = ?`, token,
	).Scan(&wh.ID, &wh.SiteDomain, &wh.Token, &active, &wh.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("webhook not found")
	}
	if err != nil {
		return nil, err
	}
	wh.Active = active == 1
	return wh, nil
}

func (d *DB) ListWebhooks(siteDomain string) ([]Webhook, error) {
	rows, err := d.conn.Query(`SELECT id, site_domain, token, active, created_at FROM webhooks WHERE site_domain = ?`, siteDomain)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var whs []Webhook
	for rows.Next() {
		var wh Webhook
		var active int
		if err := rows.Scan(&wh.ID, &wh.SiteDomain, &wh.Token, &active, &wh.CreatedAt); err != nil {
			return nil, err
		}
		wh.Active = active == 1
		whs = append(whs, wh)
	}
	return whs, nil
}

func (d *DB) DeleteWebhook(siteDomain string) error {
	_, err := d.conn.Exec(`DELETE FROM webhooks WHERE site_domain = ?`, siteDomain)
	return err
}
