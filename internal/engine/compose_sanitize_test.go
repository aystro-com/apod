package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func sanitizeString(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "docker-compose.yml")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := sanitizeComposeFile(p); err != nil {
		t.Fatalf("sanitize: %v", err)
	}
	out, _ := os.ReadFile(p)
	return string(out)
}

// A service declaring BOTH container_name and hostname must not end up with two
// hostname keys after sanitize — that produced invalid YAML and broke
// `docker compose up` (e.g. LinuxServer's syncthing).
func TestSanitizeNoDuplicateHostname(t *testing.T) {
	in := `services:
  syncthing:
    image: lscr.io/linuxserver/syncthing
    container_name: syncthing
    hostname: syncthing
    ports:
      - 8384:8384
`
	out := sanitizeString(t, in)

	if strings.Contains(out, "container_name:") {
		t.Error("container_name should be removed")
	}
	if n := strings.Count(out, "hostname:"); n != 1 {
		t.Errorf("expected exactly one hostname key, got %d:\n%s", n, out)
	}
	// Must still parse as valid YAML (duplicate keys make Unmarshal fail).
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(out), &doc); err != nil {
		t.Errorf("sanitized compose is invalid YAML: %v\n%s", err, out)
	}
}

// When there is no explicit hostname, container_name is still converted to
// hostname so cross-service references by that name keep working.
func TestSanitizeConvertsContainerNameWhenNoHostname(t *testing.T) {
	in := `services:
  db:
    image: postgres
    container_name: mydb
`
	out := sanitizeString(t, in)
	if strings.Contains(out, "container_name:") {
		t.Error("container_name should be removed")
	}
	if !strings.Contains(out, "hostname: mydb") {
		t.Errorf("container_name should be converted to hostname: mydb\n%s", out)
	}
}

// Host port bindings are still stripped (Traefik handles external routing).
func TestSanitizeStripsHostPorts(t *testing.T) {
	in := `services:
  web:
    image: nginx
    ports:
      - 8080:80
`
	out := sanitizeString(t, in)
	if strings.Contains(out, "8080:80") {
		t.Errorf("host port binding should be stripped:\n%s", out)
	}
}
