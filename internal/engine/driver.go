package engine

import (
	"fmt"
	"os"
	"path/filepath"
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

func (dl *DriverLoader) Load(name string) (*models.Driver, error) {
	path := filepath.Join(dl.dir, name+".yaml")
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
}
