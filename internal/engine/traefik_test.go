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
