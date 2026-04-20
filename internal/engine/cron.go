package engine

import (
	"context"
	"fmt"
	"log"

	"github.com/robfig/cron/v3"
)

type CronManager struct {
	cron   *cron.Cron
	engine *Engine
}

func NewCronManager() *CronManager {
	return &CronManager{
		cron: cron.New(),
	}
}

func (cm *CronManager) SetEngine(e *Engine) {
	cm.engine = e
}

func (cm *CronManager) Start() {
	cm.cron.Start()
}

func (cm *CronManager) Stop() {
	cm.cron.Stop()
}

func (cm *CronManager) LoadJobs() error {
	if cm.engine == nil {
		return nil
	}

	jobs, err := cm.engine.db.ListAllCronJobs()
	if err != nil {
		return err
	}

	for _, job := range jobs {
		domain := job.SiteDomain
		command := job.Command
		service := job.Service

		cm.cron.AddFunc(job.Schedule, func() {
			ctx := context.Background()
			containerName := fmt.Sprintf("apod-%s-%s", domain, service)
			_, err := cm.engine.docker.ExecInContainer(ctx, containerName, []string{"sh", "-c", command})
			if err != nil {
				log.Printf("cron job failed [%s] %s: %v", domain, command, err)
			}
		})
	}
	return nil
}

func (cm *CronManager) Reload() {
	cm.cron.Stop()
	cm.cron = cron.New()
	cm.LoadJobs()
	cm.cron.Start()
}

// Engine methods
func (e *Engine) AddCronJob(ctx context.Context, domain, schedule, command, service string) (int64, error) {
	id, err := e.db.CreateCronJob(domain, schedule, command, service)
	if err != nil {
		return 0, err
	}
	if e.cronManager != nil {
		e.cronManager.Reload()
	}
	e.LogActivity(domain, "cron_add", fmt.Sprintf("%s: %s", schedule, command), "success")
	return id, nil
}

func (e *Engine) RemoveCronJob(ctx context.Context, id int64) error {
	if err := e.db.DeleteCronJob(id); err != nil {
		return err
	}
	if e.cronManager != nil {
		e.cronManager.Reload()
	}
	return nil
}

func (e *Engine) ListCronJobs(ctx context.Context, domain string) (interface{}, error) {
	return e.db.ListCronJobs(domain)
}
