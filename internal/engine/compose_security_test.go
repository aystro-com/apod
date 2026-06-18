package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func writeCompose(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestValidateComposeSecurity(t *testing.T) {
	ok := []string{
		"services:\n  web:\n    image: nginx\n    ports: [\"80\"]\n",
		// Adding a default cap is a no-op and allowed.
		"services:\n  web:\n    image: nginx\n    cap_add: [\"NET_BIND_SERVICE\"]\n",
		// Named volume and in-project relative mount are fine.
		"services:\n  web:\n    image: nginx\n    volumes: [\"data:/data\", \"./sub:/sub\"]\n",
	}
	for _, c := range ok {
		if err := validateComposeSecurity(writeCompose(t, c)); err != nil {
			t.Errorf("expected OK, got %v for:\n%s", err, c)
		}
	}

	bad := map[string]string{
		"privileged":            "services:\n  x:\n    image: a\n    privileged: true\n",
		"dangerous cap":         "services:\n  x:\n    image: a\n    cap_add: [\"DAC_READ_SEARCH\"]\n",
		"sys_admin cap":         "services:\n  x:\n    image: a\n    cap_add: [\"CAP_SYS_ADMIN\"]\n",
		"host pid":              "services:\n  x:\n    image: a\n    pid: host\n",
		"join container pid":    "services:\n  x:\n    image: a\n    pid: \"container:apod-traefik\"\n",
		"join service network":  "services:\n  x:\n    image: a\n    network_mode: \"service:other\"\n",
		"docker.sock mount":     "services:\n  x:\n    image: a\n    volumes: [\"/var/run/docker.sock:/var/run/docker.sock\"]\n",
		"relative escape mount": "services:\n  x:\n    image: a\n    volumes: [\"../../../../var/run/docker.sock:/s\"]\n",
		"etc mount":             "services:\n  x:\n    image: a\n    volumes: [\"/etc:/hostetc\"]\n",
	}
	for name, c := range bad {
		if err := validateComposeSecurity(writeCompose(t, c)); err == nil {
			t.Errorf("%s: expected rejection, got nil for:\n%s", name, c)
		}
	}
}

func TestValidateNativeHostMount(t *testing.T) {
	for _, ok := range []string{
		"/run/apod", "/run/apod/apod.sock", "data", "./files", "/home/u/sites/x/files",
	} {
		if err := validateNativeHostMount(ok); err != nil {
			t.Errorf("validateNativeHostMount(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{
		"/var/run/docker.sock", "/", "/etc", "/proc", "/var/lib/docker", "/run",
	} {
		if err := validateNativeHostMount(bad); err == nil {
			t.Errorf("validateNativeHostMount(%q) = nil, want error", bad)
		}
	}
}
