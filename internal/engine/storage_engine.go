package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
	e.reloadScheduler()
	return id, nil
}

// reloadScheduler rebuilds the backup scheduler from the DB. The pointer swap is
// mutex-guarded so two concurrent schedule changes can't race on e.scheduler
// (leaking a running cron goroutine set or losing schedules).
func (e *Engine) reloadScheduler() {
	e.scheduleMu.Lock()
	defer e.scheduleMu.Unlock()
	if e.scheduler != nil {
		e.scheduler.Stop()
		e.scheduler = NewScheduler()
		e.scheduler.SetEngine(e)
		e.scheduler.LoadSchedules()
		e.scheduler.Start()
	}
}

func (e *Engine) ListBackupSchedules(ctx context.Context, domain string) (interface{}, error) {
	return e.db.ListSchedules(domain)
}

func (e *Engine) RemoveBackupSchedule(ctx context.Context, scheduleID int64, domain string) error {
	if err := e.db.DeleteScheduleForSite(scheduleID, domain); err != nil {
		return err
	}
	e.reloadScheduler()
	return nil
}

// Storage config operations

func (e *Engine) AddStorageConfig(name, driver, configJSON string) error {
	// The config blob holds cloud/SFTP credentials — encrypt it at rest so a
	// leaked DB file doesn't expose them.
	enc, err := e.encryptSecretValue(configJSON)
	if err != nil {
		return err
	}
	return e.db.CreateStorageConfig(name, driver, enc)
}

// secretConfigKeys are storage-config fields that must never be returned to a
// client (matched case-insensitively).
var secretConfigKeys = map[string]bool{
	"secret_key":        true,
	"secret_access_key": true,
	"access_key":        true,
	"access_key_id":     true,
	"password":          true,
	"passphrase":        true,
	"token":             true,
	"private_key":       true,
}

func (e *Engine) ListStorageConfigs() (interface{}, error) {
	configs, err := e.db.ListStorageConfigs()
	if err != nil {
		return nil, err
	}
	// Decrypt (if stored encrypted) then redact credentials before the configs
	// leave the engine.
	for i := range configs {
		dec, derr := e.decryptSecretValue(configs[i].Config)
		if derr != nil {
			dec = "{}"
		}
		configs[i].Config = redactStorageSecrets(dec)
	}
	return configs, nil
}

// redactStorageSecrets parses a storage config JSON blob and replaces any
// secret-bearing value with a fixed mask, returning the re-serialized JSON.
func redactStorageSecrets(configJSON string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(configJSON), &m); err != nil {
		// Unparseable — drop it entirely rather than risk leaking secrets.
		return "{}"
	}
	for k := range m {
		if secretConfigKeys[strings.ToLower(k)] {
			m[k] = "********"
		}
	}
	out, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(out)
}

func (e *Engine) RemoveStorageConfig(name string) error {
	return e.db.DeleteStorageConfig(name)
}
