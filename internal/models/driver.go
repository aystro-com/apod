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

type Driver struct {
	Name        string                   `yaml:"name"`
	Version     string                   `yaml:"version"`
	Description string                   `yaml:"description"`
	Parameters  map[string]DriverParam   `yaml:"parameters,omitempty"`
	Services    map[string]DriverService `yaml:"services"`
	Files       []DriverFile             `yaml:"files,omitempty"`
	Healthcheck DriverHealthcheck        `yaml:"healthcheck,omitempty"`
	Backup      DriverBackup             `yaml:"backup,omitempty"`
	Cron        []DriverCron             `yaml:"cron,omitempty"`
	Setup       []DriverSetupStep        `yaml:"setup,omitempty"`
	Deploy      DriverDeployHooks        `yaml:"deploy,omitempty"`
}
