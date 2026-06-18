package engine

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
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
	tls           TLSConfig
	drivers       *DriverLoader
	locks         *LockManager
	dataDir       string
	scheduler     *Scheduler
	uptimeChecker *UptimeChecker
	cronManager   *CronManager
	loginThrottle *loginThrottle
}

type Config struct {
	DBPath          string
	DataDir         string
	DriverDir       string
	AcmeEmail       string
	TLSMode         string // auto (default) | dns | external
	ACMEDNSProvider string // lego provider code when TLSMode == dns
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

	// Ensure all required directories exist on startup
	for _, dir := range []string{
		cfg.DataDir,
		cfg.DriverDir,
		filepath.Dir(cfg.DBPath),
		"/etc/apod/traefik/dynamic",
	} {
		os.MkdirAll(dir, 0755)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	docker, err := NewDocker()
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}

	tls := TLSConfig{
		Mode:        cfg.TLSMode,
		Email:       cfg.AcmeEmail,
		DNSProvider: cfg.ACMEDNSProvider,
	}

	eng := &Engine{
		db:            database,
		docker:        docker,
		traefik:       NewTraefik(docker, tls),
		tls:           tls,
		drivers:       NewDriverLoader(cfg.DriverDir),
		locks:         NewLockManager(),
		dataDir:       cfg.DataDir,
		loginThrottle: newLoginThrottle(),
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

// NewWithDB builds a minimal Engine around an existing database handle.
// Subsystems that need Docker stay nil — intended for tests and tooling that
// only exercise database-backed features (users, auth, activity log).
func NewWithDB(database *db.DB) *Engine {
	return &Engine{
		db:            database,
		locks:         NewLockManager(),
		loginThrottle: newLoginThrottle(),
	}
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
	// DBName and DBPassword, when set, override the database name/user (which
	// otherwise derive from the domain) and the generated password. Used when
	// cloning a site from a backup so the new site keeps the source's
	// credentials and database name, leaving its restored data directory valid.
	DBName     string
	DBPassword string
	// SkipClone keeps Repo on the site record but does not git-clone into
	// siteRoot (the caller has populated it some other way).
	SkipClone bool
}

func (e *Engine) CreateSite(ctx context.Context, opts CreateSiteOpts) (err error) {
	if err := ValidateDomain(opts.Domain); err != nil {
		return err
	}
	if err := ValidateOwner(opts.Owner); err != nil {
		return err
	}
	if err := ValidateRepo(opts.Repo); err != nil {
		return err
	}
	if err := ValidateBranch(opts.Branch); err != nil {
		return err
	}

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
		site.RAM = "512M"
	}
	if site.CPU == "" {
		site.CPU = "1"
	}

	if err := e.db.CreateSite(site); err != nil {
		// If domain exists from a failed previous create, clean it up and retry
		if existing, _ := e.db.GetSite(opts.Domain); existing != nil {
			// Only reclaim a stuck record if it belongs to the same owner —
			// otherwise a user could hijack another tenant's failed domain.
			if (existing.Status == "creating" || existing.Status == "error") && existing.Owner == opts.Owner {
				e.db.DeleteSite(opts.Domain)
				if err := e.db.CreateSite(site); err != nil {
					return fmt.Errorf("create site record: %w", err)
				}
			} else {
				return Conflict("site %q already exists (status: %s)", opts.Domain, existing.Status)
			}
		} else {
			return fmt.Errorf("create site record: %w", err)
		}
	}

	// From here on the site record exists and we start creating real resources.
	// If anything below fails, tear it all back down so a failed create never
	// leaves orphan containers, networks, files or a half-built record.
	provisioned := false
	defer func() {
		if !provisioned {
			e.rollbackPartialCreate(opts.Domain, opts.Owner)
			if err != nil {
				e.LogActivity(opts.Domain, "create", err.Error(), "rolled-back")
			}
		}
	}()

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

	// Clone git repo if provided. SkipClone keeps the repo on the site record
	// (so future deploys work) but leaves siteRoot untouched — used when the
	// files are supplied another way, e.g. a physical site clone.
	if opts.Repo != "" && !opts.SkipClone {
		branch := opts.Branch
		if branch == "" {
			branch = "main"
		}
		args := append(gitHardeningArgs(), "clone", "--branch", branch, "--single-branch", "--", opts.Repo, siteRoot)
		cmd := exec.CommandContext(ctx, "git", args...)
		if output, err := cmd.CombinedOutput(); err != nil {
			e.db.UpdateSiteStatus(opts.Domain, "error")
			return fmt.Errorf("git clone: %s: %w", string(output), err)
		}
	}

	dbPass := randomHex(16)
	if opts.DBPassword != "" {
		dbPass = opts.DBPassword
	}
	dbName := strings.ReplaceAll(opts.Domain, ".", "_")
	if opts.DBName != "" {
		dbName = opts.DBName
	}
	dbUser := dbName

	vars := map[string]string{
		"site_root":    siteRoot,
		"data_root":    dataRoot,
		"site_domain":  opts.Domain,
		"site_db_name": dbName,
		"site_db_user": dbUser,
		"site_db_pass": dbPass,
	}

	// Generate extra secrets only if the driver references them.
	// This keeps the core engine generic — drivers declare what they need.
	// Order matters: jwt_secret must exist before anon_key/service_role_key.
	driverText := driverRawText(driver)
	genOrder := generatedSecretNames
	generators := secretGenerators()
	for _, varName := range genOrder {
		if strings.Contains(driverText, "${"+varName+"}") {
			vars[varName] = generators[varName](vars)
		}
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

	// Persist generated secrets as the authoritative record (backup/clone read
	// these instead of reverse-engineering them from container env or .env).
	e.db.SetSiteSecret(opts.Domain, "db_password", dbPass)
	for _, varName := range genOrder {
		if v, ok := vars[varName]; ok {
			e.db.SetSiteSecret(opts.Domain, varName, v)
		}
	}

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
				// Create the host directory for bind mounts — but don't clobber a
				// path that already exists as a non-directory, e.g. a bind-mounted
				// unix socket like /run/apod/apod.sock.
				if strings.HasPrefix(parts[0], "/") {
					if fi, statErr := os.Stat(parts[0]); statErr != nil || fi.IsDir() {
						os.MkdirAll(parts[0], 0755)
					}
				}
			}
		}

		role := effectiveRole(svcName, svc.Role)
		isWeb := role == roleWeb && len(svc.Ports) > 0
		replicas := e.desiredReplicasFor(opts.Domain, svcName, svc)

		// A service runs as one or more replica containers. Web/scheduler/plain
		// services are singletons; workers may run several (or zero).
		for idx := 0; idx < replicas; idx++ {
			containerName := replicaContainerName(opts.Domain, svcName, idx)

			labels := map[string]string{
				labelPrefix + "site":    opts.Domain,
				labelPrefix + "service": svcName,
				labelPrefix + "role":    role,
				labelPrefix + "replica": strconv.Itoa(idx),
				labelPrefix + "managed": "true",
			}
			// Only the (singleton) web process is published through Traefik.
			if isWeb {
				port := svc.Ports[0]
				traefikLabels := TraefikLabels(opts.Domain, []string{opts.Domain}, port, svc.BackendScheme, e.tls.CertResolver())
				// Tell Traefik to use the site-specific network to reach this container
				routerName := strings.ReplaceAll(opts.Domain, ".", "-")
				traefikLabels[fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port", routerName)] = port
				for k, v := range traefikLabels {
					labels[k] = v
				}
			}

			// Create the container on the site-specific isolated network only, so
			// it never joins Docker's default bridge (which would let other sites
			// reach it by raw IP).
			id, err := e.docker.CreateContainer(ctx, ContainerConfig{
				Name:        containerName,
				Image:       svc.Image,
				Env:         env,
				Volumes:     volumes,
				Labels:      labels,
				MemoryMB:    memoryMB,
				CPUs:        cpus,
				Command:     svc.Command,
				NetworkName: siteNetwork,
			})
			if err != nil {
				e.db.UpdateSiteStatus(opts.Domain, "error")
				return fmt.Errorf("create container %s: %w", containerName, err)
			}

			if err := e.docker.StartContainer(ctx, id); err != nil {
				e.db.UpdateSiteStatus(opts.Domain, "error")
				return fmt.Errorf("start container %s: %w", containerName, err)
			}
		}
	}

	// Generate .env file for the site with DB credentials and env vars
	if opts.Repo != "" {
		envContent := fmt.Sprintf("APP_ENV=production\nAPP_URL=https://%s\n", opts.Domain)
		envContent += fmt.Sprintf("DB_CONNECTION=mysql\nDB_HOST=apod-%s-db\nDB_PORT=3306\n", opts.Domain)
		envContent += fmt.Sprintf("DB_DATABASE=%s\nDB_USERNAME=%s\nDB_PASSWORD=%s\n", dbName, dbUser, dbPass)
		appKey := "base64:" + randomBase64(32)
		envContent += fmt.Sprintf("APP_KEY=%s\n", appKey)

		envPath := filepath.Join(siteRoot, ".env")
		// Only write if .env doesn't already exist (don't overwrite user config).
		// 0600: the file holds the DB password and APP_KEY.
		if _, err := os.Stat(envPath); os.IsNotExist(err) {
			os.WriteFile(envPath, []byte(envContent), 0600)
			if opts.Owner != "" {
				if user, err := e.db.GetUserByName(opts.Owner); err == nil {
					os.Chown(envPath, user.UID, user.UID)
				}
			}
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

	provisioned = true
	return nil
}

// rollbackPartialCreate tears down the resources of a site whose creation failed
// partway through, leaving no orphan containers, network, files or DB records.
func (e *Engine) rollbackPartialCreate(domain, owner string) {
	ctx := context.Background()
	ids, _ := e.docker.ListContainersByLabel(ctx, labelPrefix+"site", domain)
	for _, id := range ids {
		e.docker.StopContainer(ctx, id)
		e.docker.RemoveContainer(ctx, id)
	}
	e.docker.RemoveNetwork(ctx, fmt.Sprintf("apod-site-%s", strings.ReplaceAll(domain, ".", "-")))
	siteRoot, dataRoot := e.SiteDir(owner, domain)
	os.RemoveAll(siteRoot)
	os.RemoveAll(dataRoot)
	e.db.DeleteProcessScaling(domain)
	e.db.DeleteSiteSecrets(domain)
	e.db.DeleteSite(domain)
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

	// Remove the site's IP allowlist middleware file.
	os.Remove(filepath.Join(traefikDynamicDir, ipAllowMiddlewareName(domain)+".toml"))

	// Drop any per-service scaling overrides and stored secrets.
	e.db.DeleteProcessScaling(domain)
	e.db.DeleteSiteSecrets(domain)

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

	// Materialize the site's IP allowlist middleware before the router comes up.
	if err := e.ApplyIPRules(domain); err != nil {
		log.Printf("apply ip rules for %s: %v", domain, err)
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

// GetDriverContent returns the raw YAML for a driver.
func (e *Engine) GetDriverContent(name string) (string, error) {
	return e.drivers.GetContent(name)
}

// SaveDriver validates and stores a (custom) driver definition.
// ValidateDriver parses driver YAML and returns a preview without saving it.
func (e *Engine) ValidateDriver(content string) (*DriverPreview, error) {
	return e.drivers.Validate(content)
}

func (e *Engine) SaveDriver(name, content string) error {
	if err := e.drivers.Save(name, content); err != nil {
		return err
	}
	e.LogActivity("server", "driver_save", name, "success")
	return nil
}

// DeleteDriver removes a custom driver (built-ins are protected).
func (e *Engine) DeleteDriver(name string) error {
	if err := e.drivers.Delete(name); err != nil {
		return err
	}
	e.LogActivity("server", "driver_delete", name, "success")
	return nil
}

// DriverIsBuiltin reports whether a driver name is a shipped built-in.
func (e *Engine) DriverIsBuiltin(name string) bool {
	return e.drivers.IsBuiltin(name)
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

// generatedSecretNames lists the optional secrets the engine generates on
// demand (only when a driver references them). Order matters: jwt_secret must
// exist before anon_key/service_role_key.
var generatedSecretNames = []string{
	"jwt_secret", "anon_key", "service_role_key",
	"secret_key_base", "vault_enc_key", "dashboard_password",
	"pg_meta_crypto_key", "s3_access_key_id", "s3_access_key_secret",
	"logflare_public_token", "logflare_private_token",
}

// siteVars reconstructs the driver variable map for an existing site so driver
// strings (deploy hooks, healthcheck, cron, …) can be expanded with the same
// values used at creation. Generated secrets and the DB password come from the
// secrets store; name/user/paths are deterministic from the domain.
func (e *Engine) siteVars(site *models.Site) map[string]string {
	siteRoot, dataRoot := e.SiteDir(site.Owner, site.Domain)
	dbName := strings.ReplaceAll(site.Domain, ".", "_")
	vars := map[string]string{
		"site_root":    siteRoot,
		"data_root":    dataRoot,
		"site_domain":  site.Domain,
		"site_db_name": dbName,
		"site_db_user": dbName,
	}
	if pass, ok, _ := e.db.GetSiteSecret(site.Domain, "db_password"); ok {
		vars["site_db_pass"] = pass
	}
	for _, name := range generatedSecretNames {
		if v, ok, _ := e.db.GetSiteSecret(site.Domain, name); ok {
			vars[name] = v
		}
	}
	return vars
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failure is catastrophic — never emit a weak secret.
		panic("apod: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func base64Encode(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

// secretGenerators returns a map of variable names to generator functions.
// A generator receives the current vars map and returns the generated value.
// These are only called if the driver YAML actually references ${var_name}.
func secretGenerators() map[string]func(vars map[string]string) string {
	return map[string]func(vars map[string]string) string{
		"jwt_secret":             func(v map[string]string) string { return randomBase64(30) },
		"anon_key":               func(v map[string]string) string { return generateJWT(v["jwt_secret"], "anon") },
		"service_role_key":       func(v map[string]string) string { return generateJWT(v["jwt_secret"], "service_role") },
		"secret_key_base":        func(v map[string]string) string { return randomBase64(48) },
		"vault_enc_key":          func(v map[string]string) string { return randomHex(16) },
		"dashboard_password":     func(v map[string]string) string { return randomHex(16) },
		"pg_meta_crypto_key":     func(v map[string]string) string { return randomBase64(24) },
		"s3_access_key_id":       func(v map[string]string) string { return randomHex(16) },
		"s3_access_key_secret":   func(v map[string]string) string { return randomHex(32) },
		"logflare_public_token":  func(v map[string]string) string { return randomBase64(24) },
		"logflare_private_token": func(v map[string]string) string { return randomBase64(24) },
	}
}

// driverRawText returns a string representation of the driver for variable detection.
func driverRawText(driver *models.Driver) string {
	var b strings.Builder
	for _, svc := range driver.Services {
		b.WriteString(svc.Image)
		b.WriteString(svc.Command)
		for _, v := range svc.Volumes {
			b.WriteString(v)
		}
		for k, v := range svc.Environment {
			b.WriteString(k)
			b.WriteString(v)
		}
	}
	for _, f := range driver.Files {
		b.WriteString(f.Path)
		b.WriteString(f.Content)
	}
	for _, s := range driver.Setup {
		b.WriteString(s.Command)
	}
	if driver.Compose != nil {
		for k, v := range driver.Compose.Env {
			b.WriteString(k)
			b.WriteString(v)
		}
	}
	return b.String()
}
