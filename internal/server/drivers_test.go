package server

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/aystro/apod/internal/models"
)

// The drivers list must serialize with lowercase JSON keys the UI reads, and
// must NOT include a driver's internals (services, deploy hooks, images).
func TestSummarizeDriversShape(t *testing.T) {
	in := []models.Driver{
		{
			Name: "php", Version: "1.0", Description: "PHP stack",
			Services: map[string]models.DriverService{"app": {Image: "php:8"}},
			Deploy:   models.DriverDeployHooks{AfterDeploy: []string{"secret-ish hook"}},
		},
		{Name: "supa", Type: "compose"},
	}

	out := summarizeDrivers(in)
	if len(out) != 2 {
		t.Fatalf("got %d summaries, want 2", len(out))
	}
	if out[0].Name != "php" || out[0].Version != "1.0" {
		t.Errorf("unexpected summary: %+v", out[0])
	}
	// Empty type defaults to "services"; explicit type is preserved.
	if out[0].Type != "services" || out[1].Type != "compose" {
		t.Errorf("type defaulting wrong: %q / %q", out[0].Type, out[1].Type)
	}

	data, _ := json.Marshal(out)
	js := string(data)
	if !strings.Contains(js, `"name":"php"`) {
		t.Errorf("expected lowercase name key, got: %s", js)
	}
	if strings.Contains(js, `"Name"`) {
		t.Errorf("capitalised key leaked: %s", js)
	}
	// Internals must not be exposed in the list (the image, deploy hooks, or the
	// nested struct keys).
	for _, leak := range []string{`"Services"`, `"Deploy"`, "AfterDeploy", "php:8", "secret-ish"} {
		if strings.Contains(js, leak) {
			t.Errorf("driver internals leaked (%q) in: %s", leak, js)
		}
	}
}
