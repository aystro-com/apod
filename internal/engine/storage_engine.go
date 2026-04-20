package engine

import (
	"context"
	"fmt"
)

// Backup schedule operations

func (e *Engine) AddBackupSchedule(ctx context.Context, domain, duration, storageName string, keepCount int) (int64, error) {
	cronExpr, err := durationToCron(duration)
	if err != nil {
		return 0, err
	}
	if storageName == "" {
		storageName = "local"
	}
	id, err := e.db.CreateSchedule(domain, cronExpr, storageName, keepCount)
	if err != nil {
		return 0, fmt.Errorf("create schedule: %w", err)
	}
	if e.scheduler != nil {
		e.scheduler.Stop()
		e.scheduler = NewScheduler()
		e.scheduler.SetEngine(e)
		e.scheduler.LoadSchedules()
		e.scheduler.Start()
	}
	return id, nil
}

func (e *Engine) ListBackupSchedules(ctx context.Context, domain string) (interface{}, error) {
	return e.db.ListSchedules(domain)
}

func (e *Engine) RemoveBackupSchedule(ctx context.Context, scheduleID int64) error {
	if err := e.db.DeleteSchedule(scheduleID); err != nil {
		return err
	}
	if e.scheduler != nil {
		e.scheduler.Stop()
		e.scheduler = NewScheduler()
		e.scheduler.SetEngine(e)
		e.scheduler.LoadSchedules()
		e.scheduler.Start()
	}
	return nil
}

// Storage config operations

func (e *Engine) AddStorageConfig(name, driver, configJSON string) error {
	return e.db.CreateStorageConfig(name, driver, configJSON)
}

func (e *Engine) ListStorageConfigs() (interface{}, error) {
	return e.db.ListStorageConfigs()
}

func (e *Engine) RemoveStorageConfig(name string) error {
	return e.db.DeleteStorageConfig(name)
}
