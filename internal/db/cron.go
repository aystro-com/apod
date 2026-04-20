package db

import (
	"fmt"
	"time"
)

type CronJob struct {
	ID         int64     `json:"id"`
	SiteDomain string    `json:"site_domain"`
	Schedule   string    `json:"schedule"`
	Command    string    `json:"command"`
	Service    string    `json:"service"`
	Active     bool      `json:"active"`
	CreatedAt  time.Time `json:"created_at"`
}

func (d *DB) CreateCronJob(siteDomain, schedule, command, service string) (int64, error) {
	if service == "" {
		service = "app"
	}
	result, err := d.conn.Exec(
		`INSERT INTO cron_jobs (site_domain, schedule, command, service) VALUES (?, ?, ?, ?)`,
		siteDomain, schedule, command, service,
	)
	if err != nil {
		return 0, fmt.Errorf("create cron job: %w", err)
	}
	return result.LastInsertId()
}

func (d *DB) ListCronJobs(siteDomain string) ([]CronJob, error) {
	rows, err := d.conn.Query(
		`SELECT id, site_domain, schedule, command, service, active, created_at FROM cron_jobs WHERE site_domain = ? ORDER BY id`, siteDomain,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []CronJob
	for rows.Next() {
		var j CronJob
		var active int
		if err := rows.Scan(&j.ID, &j.SiteDomain, &j.Schedule, &j.Command, &j.Service, &active, &j.CreatedAt); err != nil {
			return nil, err
		}
		j.Active = active == 1
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func (d *DB) ListAllCronJobs() ([]CronJob, error) {
	rows, err := d.conn.Query(
		`SELECT id, site_domain, schedule, command, service, active, created_at FROM cron_jobs WHERE active = 1 ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []CronJob
	for rows.Next() {
		var j CronJob
		var active int
		if err := rows.Scan(&j.ID, &j.SiteDomain, &j.Schedule, &j.Command, &j.Service, &active, &j.CreatedAt); err != nil {
			return nil, err
		}
		j.Active = active == 1
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func (d *DB) DeleteCronJob(id int64) error {
	result, err := d.conn.Exec(`DELETE FROM cron_jobs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete cron job: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("cron job %d not found", id)
	}
	return nil
}
