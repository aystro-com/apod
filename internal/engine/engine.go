package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aystro/apod/internal/db"
	"github.com/aystro/apod/internal/models"
)

const (
	defaultDataDir   = "/var/lib/apod"
	defaultDriverDir = "/etc/apod/drivers"
)

type Engine struct {
	db        *db.DB
	docker    *Docker
	traefik   *Traefik
	drivers   *DriverLoader
	locks     *LockManager
	dataDir   string
	scheduler *Scheduler
}

type Config struct {
	DBPath    string
	DataDir   string
	DriverDir string
	AcmeEmail string
}

func New(cfg Config) (*Engine, error) {
	if cfg.DBPath == "" {
		cfg.DBPath = db.DefaultPath()
	}
	if cfg.DataDir == "" {
		cfg.DataDir = defaultDataDir
	}
	if cfg.DriverDir == "" {
		cfg.DriverDir = defaultDriverDir
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	docker, err := NewDocker()
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}

	eng := &Engine{
		db:      database,
		docker:  docker,
		traefik: NewTraefik(docker, cfg.AcmeEmail),
		drivers: NewDriverLoader(cfg.DriverDir),
		locks:   NewLockManager(),
		dataDir: cfg.DataDir,
	}

	sched := NewScheduler()
	sched.SetEngine(eng)
	sched.LoadSchedules()
	sched.Start()
	eng.scheduler = sched

	return eng, nil
}

func (e *Engine) Close() {
	if e.scheduler != nil {
		e.scheduler.Stop()
	}
	e.db.Close()
	e.docker.Close()
}

type CreateSiteOpts struct {
	Domain string
	Driver string
	RAM    string
	CPU    string
	Repo   string
	Branch string
	Params map[string]string
}

func (e *Engine) CreateSite(ctx context.Context, opts CreateSiteOpts) error {
	if err := e.locks.Acquire(opts.Domain); err != nil {
		return err
	}
	defer e.locks.Release(opts.Domain)

	driver, err := e.drivers.Load(opts.Driver)
	if err != nil {
		return fmt.Errorf("load driver: %w", err)
	}

	site := &models.Site{
		Domain: opts.Domain,
		Driver: opts.Driver,
		RAM:    opts.RAM,
		CPU:    opts.CPU,
		Repo:   opts.Repo,
		Branch: opts.Branch,
	}
	if site.RAM == "" {
		site.RAM = "256M"
	}
	if site.CPU == "" {
		site.CPU = "1"
	}

	if err := e.db.CreateSite(site); err != nil {
		return fmt.Errorf("create site record: %w", err)
	}

	siteRoot := filepath.Join(e.dataDir, "sites", opts.Domain, "files")
	dataRoot := filepath.Join(e.dataDir, "sites", opts.Domain, "data")
	if err := os.MkdirAll(siteRoot, 0755); err != nil {
		return fmt.Errorf("create site root: %w", err)
	}
	if err := os.MkdirAll(dataRoot, 0755); err != nil {
		return fmt.Errorf("create data root: %w", err)
	}

	dbPass := randomHex(16)
	dbName := strings.ReplaceAll(opts.Domain, ".", "_")
	dbUser := dbName

	vars := map[string]string{
		"site_root":    siteRoot,
		"data_root":    dataRoot,
		"site_domain":  opts.Domain,
		"site_db_name": dbName,
		"site_db_user": dbUser,
		"site_db_pass": dbPass,
	}
	ExpandDriverVariables(driver, vars)

	if err := e.traefik.EnsureRunning(ctx); err != nil {
		return fmt.Errorf("ensure traefik: %w", err)
	}

	if err := e.docker.EnsureNetwork(ctx, apodNetwork); err != nil {
		return fmt.Errorf("ensure network: %w", err)
	}

	memoryMB := parseMemoryMB(site.RAM)
	cpus, _ := strconv.ParseFloat(site.CPU, 64)

	for svcName, svc := range driver.Services {
		containerName := fmt.Sprintf("apod-%s-%s", opts.Domain, svcName)

		if err := e.docker.PullImage(ctx, svc.Image); err != nil {
			e.db.UpdateSiteStatus(opts.Domain, "error")
			return fmt.Errorf("pull image %s: %w", svc.Image, err)
		}

		var env []string
		for k, v := range svc.Environment {
			env = append(env, k+"="+v)
		}

		volumes := make(map[string]string)
		for _, v := range svc.Volumes {
			parts := strings.SplitN(v, ":", 2)
			if len(parts) == 2 {
				volumes[parts[0]] = parts[1]
			}
		}

		labels := map[string]string{
			labelPrefix + "site":    opts.Domain,
			labelPrefix + "service": svcName,
			labelPrefix + "managed": "true",
		}
		if svcName == "app" && len(svc.Ports) > 0 {
			port := svc.Ports[0]
			traefikLabels := TraefikLabels(opts.Domain, []string{opts.Domain}, port)
			for k, v := range traefikLabels {
				labels[k] = v
			}
		}

		id, err := e.docker.CreateContainer(ctx, ContainerConfig{
			Name:     containerName,
			Image:    svc.Image,
			Env:      env,
			Volumes:  volumes,
			Labels:   labels,
			MemoryMB: memoryMB,
			CPUs:     cpus,
			Command:  svc.Command,
		})
		if err != nil {
			e.db.UpdateSiteStatus(opts.Domain, "error")
			return fmt.Errorf("create container %s: %w", containerName, err)
		}

		if err := e.docker.ConnectNetwork(ctx, apodNetwork, id); err != nil {
			e.db.UpdateSiteStatus(opts.Domain, "error")
			return fmt.Errorf("connect container to network: %w", err)
		}

		if err := e.docker.StartContainer(ctx, id); err != nil {
			e.db.UpdateSiteStatus(opts.Domain, "error")
			return fmt.Errorf("start container %s: %w", containerName, err)
		}
	}

	for _, step := range driver.Setup {
		containerName := fmt.Sprintf("apod-%s-%s", opts.Domain, step.Service)
		_, err := e.docker.ExecInContainer(ctx, containerName, []string{"sh", "-c", step.Command})
		if err != nil {
			e.db.UpdateSiteStatus(opts.Domain, "error")
			return fmt.Errorf("setup step %q: %w", step.Name, err)
		}
	}

	e.db.UpdateSiteStatus(opts.Domain, "running")

	// Register primary domain
	if createdSite, err := e.db.GetSite(opts.Domain); err == nil {
		e.db.AddDomain(createdSite.ID, opts.Domain, true)
	}

	return nil
}

func (e *Engine) DestroySite(ctx context.Context, domain string, purge bool) error {
	if err := e.locks.Acquire(domain); err != nil {
		return err
	}
	defer e.locks.Release(domain)

	ids, err := e.docker.ListContainersByLabel(ctx, labelPrefix+"site", domain)
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}

	for _, id := range ids {
		e.docker.StopContainer(ctx, id)
		if err := e.docker.RemoveContainer(ctx, id); err != nil {
			return fmt.Errorf("remove container: %w", err)
		}
	}

	if err := e.db.DeleteSite(domain); err != nil {
		return fmt.Errorf("delete site record: %w", err)
	}

	if purge {
		siteDir := filepath.Join(e.dataDir, "sites", domain)
		if err := os.RemoveAll(siteDir); err != nil {
			return fmt.Errorf("remove site data: %w", err)
		}
	}

	return nil
}

func (e *Engine) StartSite(ctx context.Context, domain string) error {
	if err := e.locks.Acquire(domain); err != nil {
		return err
	}
	defer e.locks.Release(domain)

	ids, err := e.docker.ListContainersByLabel(ctx, labelPrefix+"site", domain)
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}

	for _, id := range ids {
		if err := e.docker.StartContainer(ctx, id); err != nil {
			return fmt.Errorf("start container: %w", err)
		}
	}

	return e.db.UpdateSiteStatus(domain, "running")
}

func (e *Engine) StopSite(ctx context.Context, domain string) error {
	if err := e.locks.Acquire(domain); err != nil {
		return err
	}
	defer e.locks.Release(domain)

	ids, err := e.docker.ListContainersByLabel(ctx, labelPrefix+"site", domain)
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}

	for _, id := range ids {
		if err := e.docker.StopContainer(ctx, id); err != nil {
			return fmt.Errorf("stop container: %w", err)
		}
	}

	return e.db.UpdateSiteStatus(domain, "stopped")
}

func (e *Engine) RestartSite(ctx context.Context, domain string) error {
	if err := e.StopSite(ctx, domain); err != nil {
		return err
	}
	return e.StartSite(ctx, domain)
}

func (e *Engine) ListSites(ctx context.Context) ([]models.Site, error) {
	return e.db.ListSites()
}

func (e *Engine) GetSite(ctx context.Context, domain string) (*models.Site, error) {
	return e.db.GetSite(domain)
}

func (e *Engine) ListDrivers() ([]models.Driver, error) {
	return e.drivers.List()
}

func parseMemoryMB(s string) int64 {
	s = strings.TrimSpace(strings.ToUpper(s))
	if strings.HasSuffix(s, "G") {
		n, _ := strconv.ParseInt(strings.TrimSuffix(s, "G"), 10, 64)
		return n * 1024
	}
	if strings.HasSuffix(s, "M") {
		n, _ := strconv.ParseInt(strings.TrimSuffix(s, "M"), 10, 64)
		return n
	}
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
