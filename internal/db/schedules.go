package db

import (
	"fmt"
	"time"
)

type BackupSchedule struct {
	ID          int64     `json:"id"`
	SiteDomain  string    `json:"site_domain"`
	CronExpr    string    `json:"cron_expr"`
	StorageName string    `json:"storage_name"`
	KeepCount   int       `json:"keep_count"`
	CreatedAt   time.Time `json:"created_at"`
}

func (d *DB) CreateSchedule(siteDomain, cronExpr, storageName string, keepCount int) (int64, error) {
	result, err := d.conn.Exec(
		`INSERT INTO backup_schedules (site_domain, cron_expr, storage_name, keep_count) VALUES (?, ?, ?, ?)`,
		siteDomain, cronExpr, storageName, keepCount,
	)
	if err != nil {
		return 0, fmt.Errorf("create schedule: %w", err)
	}
	return result.LastInsertId()
}

func (d *DB) ListSchedules(siteDomain string) ([]BackupSchedule, error) {
	rows, err := d.conn.Query(
		`SELECT id, site_domain, cron_expr, storage_name, keep_count, created_at FROM backup_schedules WHERE site_domain = ? ORDER BY id`,
		siteDomain,
	)
	if err != nil {
		return nil, fmt.Errorf("query schedules: %w", err)
	}
	defer rows.Close()

	var schedules []BackupSchedule
	for rows.Next() {
		var s BackupSchedule
		if err := rows.Scan(&s.ID, &s.SiteDomain, &s.CronExpr, &s.StorageName, &s.KeepCount, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan schedule: %w", err)
		}
		schedules = append(schedules, s)
	}
	return schedules, nil
}

func (d *DB) ListAllSchedules() ([]BackupSchedule, error) {
	rows, err := d.conn.Query(
		`SELECT id, site_domain, cron_expr, storage_name, keep_count, created_at FROM backup_schedules ORDER BY id`,
	)
	if err != nil {
		return nil, fmt.Errorf("query all schedules: %w", err)
	}
	defer rows.Close()

	var schedules []BackupSchedule
	for rows.Next() {
		var s BackupSchedule
		if err := rows.Scan(&s.ID, &s.SiteDomain, &s.CronExpr, &s.StorageName, &s.KeepCount, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan schedule: %w", err)
		}
		schedules = append(schedules, s)
	}
	return schedules, nil
}

func (d *DB) DeleteSchedule(id int64) error {
	result, err := d.conn.Exec(`DELETE FROM backup_schedules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete schedule: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("schedule %d not found", id)
	}
	return nil
}
