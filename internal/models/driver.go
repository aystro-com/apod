package models

type DriverParam struct {
	Type    string   `yaml:"type"`
	Default string   `yaml:"default"`
	Options []string `yaml:"options,omitempty"`
}

type DriverService struct {
	Image         string            `yaml:"image"`
	Volumes       []string          `yaml:"volumes,omitempty"`
	Ports         []string          `yaml:"ports,omitempty"`
	Environment   map[string]string `yaml:"environment,omitempty"`
	Command       string            `yaml:"command,omitempty"`
	BackendScheme string            `yaml:"backend_scheme,omitempty"`
	// Role is the generic, stack-agnostic process type: "web" (HTTP-routed),
	// "worker" (scalable background process), "scheduler" (background singleton),
	// or "" for a plain single-instance backing service (db, cache, …).
	Role string `yaml:"role,omitempty"`
	// Replicas is the default container count for a worker role (>= 1). Ignored
	// for non-worker roles, which are always singletons.
	Replicas int `yaml:"replicas,omitempty"`
}

type DriverHealthcheck struct {
	URL      string `yaml:"url"`
	Interval string `yaml:"interval"`
	Timeout  string `yaml:"timeout"`
	Retries  int    `yaml:"retries"`
}

type DriverBackup struct {
	Paths     []string         `yaml:"paths,omitempty"`
	Databases []DriverBackupDB `yaml:"databases,omitempty"`
}

type DriverBackupDB struct {
	Type    string `yaml:"type"`
	Service string `yaml:"service"`
}

type DriverCron struct {
	Schedule string `yaml:"schedule"`
	Command  string `yaml:"command"`
	Service  string `yaml:"service"`
}

type DriverSetupStep struct {
	Name    string `yaml:"name"`
	Command string `yaml:"command"`
	Service string `yaml:"service"`
	User    string `yaml:"user,omitempty"`
	// Optional marks a best-effort step (a wait, a permission tweak): if it
	// fails, the deploy logs a warning and continues instead of rolling the
	// whole site back. Essential steps (installs, migrations) leave this false.
	Optional bool `yaml:"optional,omitempty"`
}

type DriverDeployHooks struct {
	BeforeDeploy []string `yaml:"before_deploy,omitempty"`
	AfterDeploy  []string `yaml:"after_deploy,omitempty"`
}

// DriverFile defines a file to generate before containers start.
// Path and Content are subject to variable expansion.
type DriverFile struct {
	Path    string `yaml:"path"`
	Content string `yaml:"content"`
}

// DriverCompose defines a docker-compose based driver.
// Instead of individual services, apod manages the whole compose project.
type DriverCompose struct {
	Repo         string            `yaml:"repo,omitempty"`          // Git repo with docker-compose.yml
	File         string            `yaml:"file,omitempty"`          // Inline docker-compose.yml content (alternative to repo)
	Branch       string            `yaml:"branch,omitempty"`        // Branch (default: master)
	Path         string            `yaml:"path,omitempty"`          // Subdirectory in repo (e.g., "docker")
	ProxyService string            `yaml:"proxy_service,omitempty"` // Service Traefik routes to (auto-detected when empty)
	ProxyPort    string            `yaml:"proxy_port,omitempty"`    // Port on that service (auto-detected when empty)
	ShellService string            `yaml:"shell_service,omitempty"` // Service for apod access / terminal
	Env          map[string]string `yaml:"env,omitempty"`           // Map driver vars to compose .env
}

type Driver struct {
	Name        string                   `yaml:"name"`
	Version     string                   `yaml:"version"`
	Description string                   `yaml:"description"`
	Type        string                   `yaml:"type,omitempty"` // "services" (default) or "compose"
	Parameters  map[string]DriverParam   `yaml:"parameters,omitempty"`
	Services    map[string]DriverService `yaml:"services,omitempty"`
	Compose     *DriverCompose           `yaml:"compose,omitempty"`
	Files       []DriverFile             `yaml:"files,omitempty"`
	Healthcheck DriverHealthcheck        `yaml:"healthcheck,omitempty"`
	Backup      DriverBackup             `yaml:"backup,omitempty"`
	Cron        []DriverCron             `yaml:"cron,omitempty"`
	Setup       []DriverSetupStep        `yaml:"setup,omitempty"`
	Deploy      DriverDeployHooks        `yaml:"deploy,omitempty"`
}
