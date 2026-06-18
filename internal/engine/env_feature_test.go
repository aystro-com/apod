package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/aystro/apod/internal/db"
	"github.com/aystro/apod/internal/models"
)

// envTestEngine returns an engine backed by a fresh DB with one site, suitable
// for exercising the DB-only env feature without Docker.
func envTestEngine(t *testing.T) *Engine {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := d.CreateSite(&models.Site{Domain: "ex.com", Driver: "php"}); err != nil {
		t.Fatal(err)
	}
	return NewWithDB(d)
}

func TestSetListUnsetEnv(t *testing.T) {
	e := envTestEngine(t)
	ctx := context.Background()

	// A new site starts with no env vars.
	got, err := e.ListEnv(ctx, "ex.com")
	if err != nil {
		t.Fatalf("ListEnv: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("new site env = %v, want empty", got)
	}

	// Set two vars, including one whose value contains '='.
	if err := e.SetEnv(ctx, "ex.com", "APP_ENV", "production"); err != nil {
		t.Fatalf("SetEnv: %v", err)
	}
	if err := e.SetEnv(ctx, "ex.com", "DSN", "a=b;c=d"); err != nil {
		t.Fatalf("SetEnv: %v", err)
	}

	got, _ = e.ListEnv(ctx, "ex.com")
	if got["APP_ENV"] != "production" || got["DSN"] != "a=b;c=d" {
		t.Errorf("after set, env = %v", got)
	}

	// Overwriting an existing key replaces its value.
	if err := e.SetEnv(ctx, "ex.com", "APP_ENV", "staging"); err != nil {
		t.Fatalf("SetEnv overwrite: %v", err)
	}
	got, _ = e.ListEnv(ctx, "ex.com")
	if got["APP_ENV"] != "staging" {
		t.Errorf("APP_ENV = %q, want staging", got["APP_ENV"])
	}

	// Unset removes only the named key.
	if err := e.UnsetEnv(ctx, "ex.com", "APP_ENV"); err != nil {
		t.Fatalf("UnsetEnv: %v", err)
	}
	got, _ = e.ListEnv(ctx, "ex.com")
	if _, ok := got["APP_ENV"]; ok {
		t.Error("APP_ENV should have been removed")
	}
	if got["DSN"] != "a=b;c=d" {
		t.Errorf("DSN should survive unset of another key, got %q", got["DSN"])
	}
}

func TestEnvUnknownSite(t *testing.T) {
	e := envTestEngine(t)
	ctx := context.Background()

	if _, err := e.ListEnv(ctx, "nope.com"); err == nil {
		t.Error("ListEnv on unknown site should error")
	}
	if err := e.SetEnv(ctx, "nope.com", "K", "V"); err == nil {
		t.Error("SetEnv on unknown site should error")
	}
}

func TestGetConfig(t *testing.T) {
	e := envTestEngine(t)
	cfg, err := e.GetConfig(context.Background(), "ex.com")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if cfg["domain"] != "ex.com" {
		t.Errorf("domain = %q, want ex.com", cfg["domain"])
	}
	if cfg["driver"] != "php" {
		t.Errorf("driver = %q, want php", cfg["driver"])
	}
}
