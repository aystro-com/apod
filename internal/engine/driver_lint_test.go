package engine

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/aystro/apod/internal/models"
	"gopkg.in/yaml.v3"
)

var driverVarRe = regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)

// Every shipped driver must be structurally valid and may only reference
// variables apod actually provides — otherwise a "${foo}" survives expansion as
// a literal and the container fails (e.g. a bad image tag).
func TestBuiltinDriversAreValid(t *testing.T) {
	dir := filepath.Join("..", "..", "drivers")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read drivers dir: %v", err)
	}

	// Variables apod always supplies, plus the on-demand generated secrets.
	base := map[string]bool{
		"site_root": true, "data_root": true, "site_domain": true,
		"site_db_name": true, "site_db_user": true, "site_db_pass": true,
	}
	for _, name := range generatedSecretNames {
		base[name] = true
	}

	count := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		count++
		t.Run(e.Name(), func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			var d models.Driver
			if err := yaml.Unmarshal(raw, &d); err != nil {
				t.Fatalf("parse: %v", err)
			}
			if d.Name == "" {
				t.Error("driver has no name")
			}

			allowed := map[string]bool{}
			for k := range base {
				allowed[k] = true
			}
			for p := range d.Parameters {
				allowed[p] = true
			}
			for _, m := range driverVarRe.FindAllStringSubmatch(string(raw), -1) {
				if !allowed[m[1]] {
					t.Errorf("references unknown variable ${%s}", m[1])
				}
			}

			if d.Type == "compose" {
				if d.Compose == nil {
					t.Error("compose driver has no compose block")
				}
				if d.Compose != nil && d.Compose.ProxyService == "" {
					t.Error("compose driver has no proxy_service to route to")
				}
				return
			}

			if len(d.Services) == 0 {
				t.Error("native driver has no services")
			}
			ports := 0
			for svcName, svc := range d.Services {
				if svc.Image == "" {
					t.Errorf("service %q has no image", svcName)
				}
				ports += len(svc.Ports)
			}
			if ports == 0 {
				t.Error("driver exposes no ports — nothing for Traefik to route")
			}
		})
	}
	if count == 0 {
		t.Fatal("no drivers found")
	}
}
