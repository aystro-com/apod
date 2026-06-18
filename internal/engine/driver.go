package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/aystro/apod/internal/models"
	"gopkg.in/yaml.v3"
)

type DriverLoader struct {
	dir string
}

func NewDriverLoader(dir string) *DriverLoader {
	return &DriverLoader{dir: dir}
}

func (dl *DriverLoader) Dir() string {
	return dl.dir
}

var driverNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

func (dl *DriverLoader) Load(name string) (*models.Driver, error) {
	// Reject driver names that could escape the driver directory (path
	// traversal) or be parsed as anything other than a plain file name. The
	// driver name reaches here from the API/CLI; without this a name like
	// "../../tmp/evil" would load an attacker-controlled driver (arbitrary
	// image, host bind mounts and root sh -c setup commands).
	if !driverNamePattern.MatchString(name) {
		return nil, fmt.Errorf("invalid driver name %q", name)
	}
	path := filepath.Join(dl.dir, name+".yaml")
	if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(dl.dir)+string(filepath.Separator)) {
		return nil, fmt.Errorf("invalid driver name %q", name)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("driver %q not found: %w", name, err)
	}

	var driver models.Driver
	if err := yaml.Unmarshal(data, &driver); err != nil {
		return nil, fmt.Errorf("parse driver %q: %w", name, err)
	}

	return &driver, nil
}

// builtinDrivers are the driver definitions that ship with apod. They may be
// overwritten (customized) but not deleted via the API, so the core system
// stays intact.
var builtinDrivers = map[string]bool{
	"php": true, "laravel": true, "node": true, "wordpress": true,
	"odoo": true, "unifi": true, "paymenter": true, "supabase": true,
	"static": true, "whmcs": true, "apod-ui": true,
}

// pathFor returns the on-disk path for a driver name after validating it
// cannot escape the driver directory.
func (dl *DriverLoader) pathFor(name string) (string, error) {
	if !driverNamePattern.MatchString(name) {
		return "", fmt.Errorf("invalid driver name %q", name)
	}
	path := filepath.Join(dl.dir, name+".yaml")
	if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(dl.dir)+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid driver name %q", name)
	}
	return path, nil
}

// IsBuiltin reports whether a driver name is a shipped built-in.
func (dl *DriverLoader) IsBuiltin(name string) bool {
	return builtinDrivers[name]
}

// GetContent returns the raw YAML for a driver.
func (dl *DriverLoader) GetContent(name string) (string, error) {
	path, err := dl.pathFor(name)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("driver %q not found: %w", name, err)
	}
	return string(data), nil
}

// Save validates and writes a driver definition. The YAML must parse into a
// Driver and its declared name must match the file name. Compose drivers are
// not cloned here (that happens at site creation, behind the compose-security
// validator), so this stores the definition only.
func (dl *DriverLoader) Save(name, content string) error {
	path, err := dl.pathFor(name)
	if err != nil {
		return err
	}
	var driver models.Driver
	if err := yaml.Unmarshal([]byte(content), &driver); err != nil {
		return fmt.Errorf("invalid driver YAML: %w", err)
	}
	if driver.Name == "" {
		return fmt.Errorf("driver YAML must set a name")
	}
	if driver.Name != name {
		return fmt.Errorf("driver name in YAML (%q) must match %q", driver.Name, name)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("write driver: %w", err)
	}
	return nil
}

// DriverPreview is a safe, human-readable summary of a parsed driver, returned
// by validation so the UI can show what a YAML document will create before it
// is saved.
type DriverPreview struct {
	Name        string               `json:"name"`
	Version     string               `json:"version"`
	Description string               `json:"description"`
	Type        string               `json:"type"`
	Parameters  []DriverParamPreview `json:"parameters"`
	Services    []string             `json:"services"`
	Compose     bool                 `json:"compose"`
	Files       int                  `json:"files"`
	CronJobs    int                  `json:"cron_jobs"`
	SetupSteps  int                  `json:"setup_steps"`
	Warnings    []string             `json:"warnings"`
}

type DriverParamPreview struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Default string   `json:"default"`
	Options []string `json:"options,omitempty"`
}

// Validate parses driver YAML without writing it to disk and returns a preview.
// It applies the same hard checks as Save (valid YAML, a name, a known type)
// and collects non-fatal warnings so the user can review before saving.
func (dl *DriverLoader) Validate(content string) (*DriverPreview, error) {
	var driver models.Driver
	if err := yaml.Unmarshal([]byte(content), &driver); err != nil {
		return nil, fmt.Errorf("invalid driver YAML: %w", err)
	}
	if driver.Name == "" {
		return nil, fmt.Errorf("driver YAML must set a name")
	}
	if !driverNamePattern.MatchString(driver.Name) {
		return nil, fmt.Errorf("invalid driver name %q", driver.Name)
	}
	dtype := driver.Type
	if dtype == "" {
		dtype = "services"
	}
	if dtype != "services" && dtype != "compose" {
		return nil, fmt.Errorf("driver type must be \"services\" or \"compose\", got %q", driver.Type)
	}

	preview := &DriverPreview{
		Name:        driver.Name,
		Version:     driver.Version,
		Description: driver.Description,
		Type:        dtype,
		Compose:     driver.Compose != nil,
		Files:       len(driver.Files),
		CronJobs:    len(driver.Cron),
		SetupSteps:  len(driver.Setup),
	}
	for name, p := range driver.Parameters {
		preview.Parameters = append(preview.Parameters, DriverParamPreview{
			Name: name, Type: p.Type, Default: p.Default, Options: p.Options,
		})
	}
	sort.Slice(preview.Parameters, func(i, j int) bool {
		return preview.Parameters[i].Name < preview.Parameters[j].Name
	})
	for name := range driver.Services {
		preview.Services = append(preview.Services, name)
	}
	sort.Strings(preview.Services)

	// Non-fatal warnings.
	if dtype == "compose" && driver.Compose == nil {
		preview.Warnings = append(preview.Warnings, "type is \"compose\" but no compose block is defined")
	}
	if dtype == "services" && len(driver.Services) == 0 {
		preview.Warnings = append(preview.Warnings, "no services defined — the driver will not start any containers")
	}
	if driver.Version == "" {
		preview.Warnings = append(preview.Warnings, "no version set")
	}
	return preview, nil
}

// Delete removes a custom driver. Built-in drivers cannot be deleted.
func (dl *DriverLoader) Delete(name string) error {
	if dl.IsBuiltin(name) {
		return fmt.Errorf("driver %q is a built-in and cannot be deleted", name)
	}
	path, err := dl.pathFor(name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		return NotFound("driver %q not found", name)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete driver: %w", err)
	}
	return nil
}

func (dl *DriverLoader) List() ([]models.Driver, error) {
	entries, err := os.ReadDir(dl.dir)
	if err != nil {
		return nil, fmt.Errorf("read drivers directory: %w", err)
	}

	var drivers []models.Driver
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".yaml")
		driver, err := dl.Load(name)
		if err != nil {
			continue
		}
		drivers = append(drivers, *driver)
	}

	return drivers, nil
}

func expandVariables(s string, vars map[string]string) string {
	result := s
	for key, val := range vars {
		result = strings.ReplaceAll(result, "${"+key+"}", val)
	}
	return result
}

func ExpandDriverVariables(driver *models.Driver, vars map[string]string) {
	for name, svc := range driver.Services {
		svc.Image = expandVariables(svc.Image, vars)
		for i, v := range svc.Volumes {
			svc.Volumes[i] = expandVariables(v, vars)
		}
		for k, v := range svc.Environment {
			svc.Environment[k] = expandVariables(v, vars)
		}
		svc.Command = expandVariables(svc.Command, vars)
		driver.Services[name] = svc
	}
	for i, step := range driver.Setup {
		driver.Setup[i].Command = expandVariables(step.Command, vars)
	}
	for i, f := range driver.Files {
		driver.Files[i].Path = expandVariables(f.Path, vars)
		driver.Files[i].Content = expandVariables(f.Content, vars)
	}
}
