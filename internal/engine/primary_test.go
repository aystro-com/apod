package engine

import (
	"testing"

	"github.com/aystro/apod/internal/models"
)

func TestPrimaryServiceName(t *testing.T) {
	// Explicit web role wins, regardless of name.
	d := &models.Driver{Services: map[string]models.DriverService{
		"frontend": {Role: "web"},
		"db":       {},
		"queue":    {Role: "worker"},
	}}
	if got := primaryServiceName(d); got != "frontend" {
		t.Errorf("web-role service should be primary, got %q", got)
	}

	// Backward-compat: a service named "app" with no role is web.
	d = &models.Driver{Services: map[string]models.DriverService{
		"app": {},
		"db":  {},
	}}
	if got := primaryServiceName(d); got != "app" {
		t.Errorf(`"app" should be primary, got %q`, got)
	}

	// No web service: fall back deterministically to the first sorted service.
	d = &models.Driver{Services: map[string]models.DriverService{
		"redis": {},
		"db":    {},
	}}
	if got := primaryServiceName(d); got != "db" {
		t.Errorf("expected deterministic fallback 'db', got %q", got)
	}

	// No services at all.
	if got := primaryServiceName(&models.Driver{}); got != "" {
		t.Errorf("empty driver should yield empty, got %q", got)
	}
}
