package engine

import (
	"strings"
	"testing"
)

func TestParseShortPort(t *testing.T) {
	cases := map[string]string{
		"8080:80":           "80",
		"80":                "80",
		"127.0.0.1:9000:80": "80",
		"8989:8989":         "8989",
		"8080:80/tcp":       "80",
		"53:53/udp":         "", // udp skipped
		"8000-8005:80":      "80",
		"\"443:443\"":       "443",
	}
	for in, want := range cases {
		if got := parseShortPort(in); got != want {
			t.Errorf("parseShortPort(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestApodDriverVariable(t *testing.T) {
	driver := "services:\n  app:\n    volumes:\n      - \"${site_root}:/html\"\n"
	if v := apodDriverVariable(driver); v != "${site_root}" {
		t.Errorf("should detect apod driver var, got %q", v)
	}
	// A stock compose file with its own ${VAR} interpolation is NOT flagged.
	compose := "services:\n  web:\n    image: nginx\n    environment:\n      HOST: ${HOSTNAME}\n"
	if v := apodDriverVariable(compose); v != "" {
		t.Errorf("plain compose should not be flagged, got %q", v)
	}
}

func TestComposeFailureMessage(t *testing.T) {
	tail := []string{
		`The "site_root" variable is not set. Defaulting to a blank string.`,
		`The "site_root" variable is not set. Defaulting to a blank string.`,
		"invalid spec: :/usr/share/nginx/html:ro: empty section between colons",
	}
	got := composeFailureMessage(tail)
	if !strings.Contains(got, "invalid spec") {
		t.Errorf("should surface the real error line, got %q", got)
	}
	if strings.Contains(got, "variable is not set") {
		t.Errorf("interpolation warning should be skipped, got %q", got)
	}
	if composeFailureMessage(nil) == "" {
		t.Error("empty tail should still yield a message")
	}
}

func TestComposeWebTarget(t *testing.T) {
	// Single web service (LinuxServer-style) — its sole port wins even when it
	// is not a "well known" web port.
	sonarr := []byte(`
services:
  sonarr:
    image: lscr.io/linuxserver/sonarr
    ports:
      - "8989:8989"
`)
	if svc, port, err := composeWebTarget(sonarr); err != nil || svc != "sonarr" || port != "8989" {
		t.Errorf("sonarr: got (%q,%q,%v), want sonarr/8989", svc, port, err)
	}

	// App + DB: the DB publishes no port, so the app is the only candidate.
	appdb := []byte(`
services:
  app:
    image: someapp
    ports:
      - "8080:80"
  db:
    image: postgres
    environment:
      POSTGRES_PASSWORD: x
`)
	if svc, port, err := composeWebTarget(appdb); err != nil || svc != "app" || port != "80" {
		t.Errorf("appdb: got (%q,%q,%v), want app/80", svc, port, err)
	}

	// Two services both publishing ports: the one with a well-known web port
	// (80) is preferred over a database's 5432.
	twoPorts := []byte(`
services:
  web:
    ports: ["8080:80"]
  database:
    ports: ["5432:5432"]
`)
	if svc, port, err := composeWebTarget(twoPorts); err != nil || svc != "web" || port != "80" {
		t.Errorf("twoPorts: got (%q,%q,%v), want web/80", svc, port, err)
	}

	// Long-syntax ports.
	long := []byte(`
services:
  app:
    ports:
      - target: 3000
        published: 3000
        protocol: tcp
`)
	if svc, port, err := composeWebTarget(long); err != nil || svc != "app" || port != "3000" {
		t.Errorf("long: got (%q,%q,%v), want app/3000", svc, port, err)
	}

	// expose (no host publish) is still routable internally.
	exposed := []byte(`
services:
  api:
    expose: ["9000"]
`)
	if svc, port, err := composeWebTarget(exposed); err != nil || svc != "api" || port != "9000" {
		t.Errorf("exposed: got (%q,%q,%v), want api/9000", svc, port, err)
	}

	// No ports anywhere → an explicit error (caller must specify).
	if _, _, err := composeWebTarget([]byte("services:\n  worker:\n    image: busybox\n")); err == nil {
		t.Error("expected error when no service publishes a port")
	}
}
