package engine

import (
	"context"
	"fmt"
	"log"

	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	cron   *cron.Cron
	engine *Engine
}

func NewScheduler() *Scheduler {
	return &Scheduler{
		cron: cron.New(),
	}
}

func (s *Scheduler) SetEngine(e *Engine) {
	s.engine = e
}

func (s *Scheduler) Start() {
	s.cron.Start()
}

func (s *Scheduler) Stop() {
	s.cron.Stop()
}

func (s *Scheduler) LoadSchedules() error {
	if s.engine == nil {
		return nil
	}

	schedules, err := s.engine.db.ListAllSchedules()
	if err != nil {
		return err
	}

	for _, sched := range schedules {
		domain := sched.SiteDomain
		storageName := sched.StorageName
		keepCount := sched.KeepCount

		s.cron.AddFunc(sched.CronExpr, func() {
			ctx := context.Background()
			log.Printf("scheduled backup: %s -> %s", domain, storageName)

			_, err := s.engine.CreateBackup(ctx, domain, storageName)
			if err != nil {
				log.Printf("scheduled backup failed for %s: %v", domain, err)
				return
			}

			deleted, err := s.engine.db.DeleteOldestBackups(domain, storageName, keepCount)
			if err != nil {
				log.Printf("retention cleanup failed for %s: %v", domain, err)
				return
			}

			if len(deleted) > 0 {
				log.Printf("retention: %d old backup(s) pruned for %s", len(deleted), domain)
			}

			log.Printf("scheduled backup complete: %s (%d old backups cleaned)", domain, len(deleted))
		})
	}

	return nil
}

func durationToCron(d string) (string, error) {
	switch d {
	case "1h":
		return "0 * * * *", nil
	case "6h":
		return "0 */6 * * *", nil
	case "12h":
		return "0 */12 * * *", nil
	case "24h":
		return "0 0 * * *", nil
	case "7d":
		return "0 0 * * 0", nil
	case "30d":
		return "0 0 1 * *", nil
	default:
		return "", fmt.Errorf("unsupported duration %q", d)
	}
}
