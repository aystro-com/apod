package engine

import (
	"testing"
)

func TestTraefikLabels(t *testing.T) {
	labels := TraefikLabels("example.com", []string{"example.com", "www.example.com"}, "80")

	if labels["traefik.enable"] != "true" {
		t.Error("expected traefik.enable to be true")
	}

	expectedRule := "Host(`example.com`) || Host(`www.example.com`)"
	routerKey := "traefik.http.routers.example-com.rule"
	if labels[routerKey] != expectedRule {
		t.Errorf("got rule %q, want %q", labels[routerKey], expectedRule)
	}

	portKey := "traefik.http.services.example-com.loadbalancer.server.port"
	if labels[portKey] != "80" {
		t.Errorf("got port %q, want 80", labels[portKey])
	}

	if labels["apod.site"] != "example.com" {
		t.Error("expected apod.site label")
	}
}

func TestTraefikLabelsSingleDomain(t *testing.T) {
	labels := TraefikLabels("test.com", []string{"test.com"}, "8080")

	routerKey := "traefik.http.routers.test-com.rule"
	expected := "Host(`test.com`)"
	if labels[routerKey] != expected {
		t.Errorf("got rule %q, want %q", labels[routerKey], expected)
	}
}

func TestTraefikCommand(t *testing.T) {
	cmd := traefikCommand("admin@example.com")

	checks := map[string]bool{
		"--api.dashboard=false":               false,
		"--providers.docker=true":             false,
		"--providers.docker.exposedbydefault=false": false,
		"--entrypoints.web.address=:80":       false,
		"--entrypoints.websecure.address=:443": false,
		"--certificatesresolvers.letsencrypt.acme.email=admin@example.com": false,
		"--certificatesresolvers.letsencrypt.acme.storage=/letsencrypt/acme.json": false,
		"--certificatesresolvers.letsencrypt.acme.httpchallenge.entrypoint=web": false,
	}

	for _, arg := range cmd {
		if _, ok := checks[arg]; ok {
			checks[arg] = true
		}
	}

	for flag, found := range checks {
		if !found {
			t.Errorf("missing flag: %s", flag)
		}
	}
}

func TestTraefikCommandDefaultEmail(t *testing.T) {
	cmd := traefikCommand("")
	hasDefault := false
	for _, arg := range cmd {
		if arg == "--certificatesresolvers.letsencrypt.acme.email=admin@localhost" {
			hasDefault = true
		}
	}
	if !hasDefault {
		t.Error("expected default email when empty")
	}
}
