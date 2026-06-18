package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
		return fmt.Errorf("driver %q not found", name)
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
