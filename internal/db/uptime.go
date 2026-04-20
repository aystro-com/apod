package db

import (
	"database/sql"
	"fmt"
	"time"
)

type UptimeCheck struct {
	ID              int64     `json:"id"`
	SiteDomain      string    `json:"site_domain"`
	URL             string    `json:"url"`
	IntervalSeconds int       `json:"interval_seconds"`
	AlertWebhook    string    `json:"alert_webhook"`
	Active          bool      `json:"active"`
	CreatedAt       time.Time `json:"created_at"`
}

type UptimeLog struct {
	ID         int64     `json:"id"`
	SiteDomain string    `json:"site_domain"`
	StatusCode int       `json:"status_code"`
	ResponseMs int       `json:"response_ms"`
	IsUp       bool      `json:"is_up"`
	CheckedAt  time.Time `json:"checked_at"`
}

type UptimeStats struct {
	UptimePercent float64 `json:"uptime_percent"`
	AvgResponseMs int     `json:"avg_response_ms"`
	TotalChecks   int     `json:"total_checks"`
	TotalDowntime int     `json:"total_downtime"`
}

func (d *DB) CreateUptimeCheck(domain, url string, intervalSec int, alertWebhook string) error {
	_, err := d.conn.Exec(
		`INSERT INTO uptime_checks (site_domain, url, interval_seconds, alert_webhook) VALUES (?, ?, ?, ?)`,
		domain, url, intervalSec, alertWebhook,
	)
	if err != nil {
		return fmt.Errorf("create uptime check: %w", err)
	}
	return nil
}

func (d *DB) GetUptimeCheck(domain string) (*UptimeCheck, error) {
	uc := &UptimeCheck{}
	var active int
	err := d.conn.QueryRow(
		`SELECT id, site_domain, url, interval_seconds, alert_webhook, active, created_at FROM uptime_checks WHERE site_domain = ?`, domain,
	).Scan(&uc.ID, &uc.SiteDomain, &uc.URL, &uc.IntervalSeconds, &uc.AlertWebhook, &active, &uc.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("uptime check for %q not found", domain)
	}
	if err != nil {
		return nil, err
	}
	uc.Active = active == 1
	return uc, nil
}

func (d *DB) ListUptimeChecks() ([]UptimeCheck, error) {
	rows, err := d.conn.Query(`SELECT id, site_domain, url, interval_seconds, alert_webhook, active, created_at FROM uptime_checks WHERE active = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var checks []UptimeCheck
	for rows.Next() {
		var uc UptimeCheck
		var active int
		if err := rows.Scan(&uc.ID, &uc.SiteDomain, &uc.URL, &uc.IntervalSeconds, &uc.AlertWebhook, &active, &uc.CreatedAt); err != nil {
			return nil, err
		}
		uc.Active = active == 1
		checks = append(checks, uc)
	}
	return checks, nil
}

func (d *DB) DeleteUptimeCheck(domain string) error {
	_, err := d.conn.Exec(`DELETE FROM uptime_checks WHERE site_domain = ?`, domain)
	return err
}

func (d *DB) LogUptimeResult(domain string, statusCode, responseMs int, isUp bool) error {
	up := 0
	if isUp {
		up = 1
	}
	_, err := d.conn.Exec(
		`INSERT INTO uptime_logs (site_domain, status_code, response_ms, is_up) VALUES (?, ?, ?, ?)`,
		domain, statusCode, responseMs, up,
	)
	return err
}

func (d *DB) GetUptimeStats(domain string, hours int) (*UptimeStats, error) {
	stats := &UptimeStats{}
	err := d.conn.QueryRow(
		`SELECT COUNT(*), COALESCE(AVG(response_ms), 0), COALESCE(SUM(CASE WHEN is_up = 0 THEN 1 ELSE 0 END), 0)
		 FROM uptime_logs WHERE site_domain = ? AND checked_at > datetime('now', ?)`,
		domain, fmt.Sprintf("-%d hours", hours),
	).Scan(&stats.TotalChecks, &stats.AvgResponseMs, &stats.TotalDowntime)
	if err != nil {
		return nil, err
	}
	if stats.TotalChecks > 0 {
		stats.UptimePercent = float64(stats.TotalChecks-stats.TotalDowntime) / float64(stats.TotalChecks) * 100
	}
	return stats, nil
}

func (d *DB) GetUptimeLogs(domain string, limit int) ([]UptimeLog, error) {
	rows, err := d.conn.Query(
		`SELECT id, site_domain, status_code, response_ms, is_up, checked_at FROM uptime_logs WHERE site_domain = ? ORDER BY checked_at DESC LIMIT ?`,
		domain, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []UptimeLog
	for rows.Next() {
		var l UptimeLog
		var isUp int
		if err := rows.Scan(&l.ID, &l.SiteDomain, &l.StatusCode, &l.ResponseMs, &isUp, &l.CheckedAt); err != nil {
			return nil, err
		}
		l.IsUp = isUp == 1
		logs = append(logs, l)
	}
	return logs, nil
}
