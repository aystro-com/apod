package engine

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
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
	db            *db.DB
	docker        *Docker
	traefik       *Traefik
	drivers       *DriverLoader
	locks         *LockManager
	dataDir       string
	scheduler     *Scheduler
	uptimeChecker *UptimeChecker
	cronManager   *CronManager
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

	// Start uptime checker
	uptimeChecker := NewUptimeChecker(eng)
	uptimeChecker.Start()
	eng.uptimeChecker = uptimeChecker

	cronMgr := NewCronManager()
	cronMgr.SetEngine(eng)
	cronMgr.LoadJobs()
	cronMgr.Start()
	eng.cronManager = cronMgr

	return eng, nil
}

func (e *Engine) Close() {
	if e.scheduler != nil {
		e.scheduler.Stop()
	}
	if e.uptimeChecker != nil {
		e.uptimeChecker.Stop()
	}
	if e.cronManager != nil {
		e.cronManager.Stop()
	}
	e.db.Close()
	e.docker.Close()
}

type CreateSiteOpts struct {
	Domain  string
	Driver  string
	RAM     string
	CPU     string
	Storage string
	Repo    string
	Branch  string
	Params  map[string]string
	Owner   string
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
		Domain:  opts.Domain,
		Driver:  opts.Driver,
		RAM:     opts.RAM,
		CPU:     opts.CPU,
		Storage: opts.Storage,
		Repo:    opts.Repo,
		Branch:  opts.Branch,
		Owner:   opts.Owner,
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

	siteRoot, dataRoot := e.SiteDir(opts.Owner, opts.Domain)
	if err := os.MkdirAll(siteRoot, 0755); err != nil {
		return fmt.Errorf("create site root: %w", err)
	}
	if err := os.MkdirAll(dataRoot, 0755); err != nil {
		return fmt.Errorf("create data root: %w", err)
	}

	// Set ownership for user-owned sites
	if opts.Owner != "" {
		if user, err := e.db.GetUserByName(opts.Owner); err == nil {
			uid := user.UID
			// Own the site dir, files, and data dirs
			siteDir := filepath.Dir(siteRoot)
			os.Chown(siteDir, uid, uid)
			os.Chown(siteRoot, uid, uid)
			os.Chown(dataRoot, uid, uid)
		}
	}

	// Clone git repo if provided
	if opts.Repo != "" {
		branch := opts.Branch
		if branch == "" {
			branch = "main"
		}
		cmd := exec.CommandContext(ctx, "git", "clone", "--branch", branch, "--single-branch", opts.Repo, siteRoot)
		if output, err := cmd.CombinedOutput(); err != nil {
			e.db.UpdateSiteStatus(opts.Domain, "error")
			return fmt.Errorf("git clone: %s: %w", string(output), err)
		}
	}

	dbPass := randomHex(16)
	dbName := strings.ReplaceAll(opts.Domain, ".", "_")
	dbUser := dbName

	// Generate additional secrets for complex drivers (Supabase etc.)
	jwtSecret := randomBase64(30)
	anonKey := generateSupabaseJWT(jwtSecret, "anon")
	serviceRoleKey := generateSupabaseJWT(jwtSecret, "service_role")

	vars := map[string]string{
		"site_root":              siteRoot,
		"data_root":             dataRoot,
		"site_domain":           opts.Domain,
		"site_db_name":          dbName,
		"site_db_user":          dbUser,
		"site_db_pass":          dbPass,
		"jwt_secret":            jwtSecret,
		"anon_key":              anonKey,
		"service_role_key":      serviceRoleKey,
		"secret_key_base":       randomBase64(48),
		"vault_enc_key":         randomHex(16),
		"dashboard_password":    randomHex(16),
		"pg_meta_crypto_key":    randomBase64(24),
		"s3_access_key_id":      randomHex(16),
		"s3_access_key_secret":  randomHex(32),
		"logflare_public_token": randomBase64(24),
		"logflare_private_token": randomBase64(24),
	}
	// Add driver parameter defaults to vars
	for key, param := range driver.Parameters {
		if val, ok := opts.Params[key]; ok {
			vars[key] = val
		} else if param.Default != "" {
			vars[key] = param.Default
		}
	}
	ExpandDriverVariables(driver, vars)

	// Write driver files before container creation (e.g., kong.yml, init SQL)
	for _, f := range driver.Files {
		dir := filepath.Dir(f.Path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory for %s: %w", f.Path, err)
		}
		perm := os.FileMode(0644)
		if strings.HasSuffix(f.Path, ".sh") {
			perm = 0755
		}
		if err := os.WriteFile(f.Path, []byte(f.Content), perm); err != nil {
			return fmt.Errorf("write file %s: %w", f.Path, err)
		}
	}

	// Compose-based drivers delegate to docker compose
	if driver.Type == "compose" && driver.Compose != nil {
		if err := e.traefik.EnsureRunning(ctx); err != nil {
			return fmt.Errorf("ensure traefik: %w", err)
		}
		if err := e.CreateComposeSite(ctx, opts, driver, vars); err != nil {
			e.db.UpdateSiteStatus(opts.Domain, "error")
			return fmt.Errorf("compose site: %w", err)
		}
		e.db.UpdateSiteStatus(opts.Domain, "running")
		if createdSite, err := e.db.GetSite(opts.Domain); err == nil {
			e.db.AddDomain(createdSite.ID, opts.Domain, true)
		}
		return nil
	}

	if err := e.traefik.EnsureRunning(ctx); err != nil {
		return fmt.Errorf("ensure traefik: %w", err)
	}

	if err := e.docker.EnsureNetwork(ctx, apodNetwork); err != nil {
		return fmt.Errorf("ensure network: %w", err)
	}

	// Create per-site isolated network (only this site's containers + Traefik)
	siteNetwork := fmt.Sprintf("apod-site-%s", strings.ReplaceAll(opts.Domain, ".", "-"))
	if err := e.docker.EnsureNetwork(ctx, siteNetwork); err != nil {
		return fmt.Errorf("ensure site network: %w", err)
	}
	// Connect Traefik to this site's network so it can route traffic
	e.docker.ConnectNetwork(ctx, siteNetwork, "apod-traefik")

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
				// Create host directory for bind mounts
				if strings.HasPrefix(parts[0], "/") {
					os.MkdirAll(parts[0], 0755)
				}
			}
		}

		labels := map[string]string{
			labelPrefix + "site":    opts.Domain,
			labelPrefix + "service": svcName,
			labelPrefix + "managed": "true",
		}
		if svcName == "app" && len(svc.Ports) > 0 {
			port := svc.Ports[0]
			traefikLabels := TraefikLabels(opts.Domain, []string{opts.Domain}, port, svc.BackendScheme)
			// Tell Traefik to use the site-specific network to reach this container
			routerName := strings.ReplaceAll(opts.Domain, ".", "-")
			traefikLabels[fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port", routerName)] = port
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

		// Connect to site-specific isolated network (not the shared apod-net)
		if err := e.docker.ConnectNetwork(ctx, siteNetwork, id); err != nil {
			e.db.UpdateSiteStatus(opts.Domain, "error")
			return fmt.Errorf("connect container to network: %w", err)
		}

		if err := e.docker.StartContainer(ctx, id); err != nil {
			e.db.UpdateSiteStatus(opts.Domain, "error")
			return fmt.Errorf("start container %s: %w", containerName, err)
		}
	}

	// Generate .env file for the site with DB credentials and env vars
	if opts.Repo != "" {
		envContent := fmt.Sprintf("APP_ENV=production\nAPP_URL=https://%s\n", opts.Domain)
		envContent += fmt.Sprintf("DB_CONNECTION=mysql\nDB_HOST=apod-%s-db\nDB_PORT=3306\n", opts.Domain)
		envContent += fmt.Sprintf("DB_DATABASE=%s\nDB_USERNAME=%s\nDB_PASSWORD=%s\n", dbName, dbUser, dbPass)
		keyBytes := make([]byte, 32)
		rand.Read(keyBytes)
		appKey := "base64:" + base64Encode(keyBytes)
		envContent += fmt.Sprintf("APP_KEY=%s\n", appKey)

		envPath := filepath.Join(siteRoot, ".env")
		// Only write if .env doesn't already exist (don't overwrite user config)
		if _, err := os.Stat(envPath); os.IsNotExist(err) {
			os.WriteFile(envPath, []byte(envContent), 0644)
		}
	}

	for _, step := range driver.Setup {
		containerName := fmt.Sprintf("apod-%s-%s", opts.Domain, step.Service)
		_, err := e.docker.ExecInContainerAs(ctx, containerName, []string{"sh", "-c", step.Command}, step.User)
		if err != nil {
			e.db.UpdateSiteStatus(opts.Domain, "error")
			return fmt.Errorf("setup step %q: %w", step.Name, err)
		}
	}

	// Restart all containers after setup to pick up any DB changes (roles, schemas, etc.)
	if len(driver.Setup) > 0 {
		allIDs, _ := e.docker.ListContainersByLabel(ctx, labelPrefix+"site", opts.Domain)
		for _, id := range allIDs {
			e.docker.StopContainer(ctx, id)
		}
		for _, id := range allIDs {
			e.docker.StartContainer(ctx, id)
		}
	}

	e.db.UpdateSiteStatus(opts.Domain, "running")

	// Register primary domain
	if createdSite, err := e.db.GetSite(opts.Domain); err == nil {
		e.db.AddDomain(createdSite.ID, opts.Domain, true)
	}

	// Apply disk quota for the user
	if opts.Owner != "" && opts.Storage != "" && opts.Storage != "0" {
		e.ApplyDiskQuota(ctx, opts.Owner)
	}

	return nil
}

func (e *Engine) DestroySite(ctx context.Context, domain string, purge bool) error {
	if err := e.locks.Acquire(domain); err != nil {
		return err
	}
	defer e.locks.Release(domain)

	// Check if this is a compose site
	site, _ := e.db.GetSite(domain)
	if site != nil {
		driver, _ := e.drivers.Load(site.Driver)
		if driver != nil && driver.Type == "compose" {
			e.DestroyComposeSite(ctx, domain, site.Owner)
			e.db.DeleteSite(domain)
			if purge {
				siteRoot, _ := e.SiteDir(site.Owner, domain)
				os.RemoveAll(filepath.Dir(siteRoot))
			}
			return nil
		}
	}

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

	// Remove site-specific network
	siteNetwork := fmt.Sprintf("apod-site-%s", strings.ReplaceAll(domain, ".", "-"))
	e.docker.RemoveNetwork(ctx, siteNetwork)

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

	site, _ := e.db.GetSite(domain)
	if site != nil {
		driver, _ := e.drivers.Load(site.Driver)
		if driver != nil && driver.Type == "compose" {
			if err := e.StartComposeSite(ctx, domain, site.Owner); err != nil {
				return err
			}
			return e.db.UpdateSiteStatus(domain, "running")
		}
	}

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

	site, _ := e.db.GetSite(domain)
	if site != nil {
		driver, _ := e.drivers.Load(site.Driver)
		if driver != nil && driver.Type == "compose" {
			if err := e.StopComposeSite(ctx, domain, site.Owner); err != nil {
				return err
			}
			return e.db.UpdateSiteStatus(domain, "stopped")
		}
	}

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

func (e *Engine) ListSitesByOwner(ctx context.Context, owner string) ([]models.Site, error) {
	return e.db.ListSitesByOwner(owner)
}

func (e *Engine) GetSite(ctx context.Context, domain string) (*models.Site, error) {
	return e.db.GetSite(domain)
}

func (e *Engine) ListDrivers() ([]models.Driver, error) {
	return e.drivers.List()
}

func (e *Engine) GetDBVersion() int {
	return e.db.CurrentVersion()
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

func base64Encode(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}
