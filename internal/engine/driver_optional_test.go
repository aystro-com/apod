package engine

import (
	"testing"

	"github.com/aystro/apod/internal/models"
	"gopkg.in/yaml.v3"
)

// Setup steps can be marked optional so a best-effort step (a wait, a
// permission tweak) doesn't roll the whole site back when it fails.
func TestDriverSetupOptionalParses(t *testing.T) {
	src := `
name: x
services:
  app:
    image: nginx
setup:
  - name: "Wait for database"
    command: "true"
    service: app
    optional: true
  - name: "Install deps"
    command: "composer install"
    service: app
`
	var d models.Driver
	if err := yaml.Unmarshal([]byte(src), &d); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(d.Setup) != 2 {
		t.Fatalf("got %d setup steps, want 2", len(d.Setup))
	}
	if !d.Setup[0].Optional {
		t.Error("first step should be optional")
	}
	if d.Setup[1].Optional {
		t.Error("second step should default to required (not optional)")
	}
}
