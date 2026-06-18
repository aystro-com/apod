package cli

import (
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aystro/apod/internal/db"
	"github.com/aystro/apod/internal/engine"
	"github.com/aystro/apod/internal/models"
	"github.com/aystro/apod/internal/server"
)

// newE2EServer stands up the real HTTP server over a temp DB (no Docker),
// seeds an admin user and one site, and returns the base URL + admin key.
// This exercises the actual cobra commands -> HTTP -> server -> engine -> DB
// path end to end for the features that don't require containers.
func newE2EServer(t *testing.T) (url, key string) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "e2e.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	if err := d.CreateUser("root", engine.HashAPIKey("apod_rootkey"), "admin", 5000); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	if err := d.CreateSite(&models.Site{Domain: "ex.com", Driver: "php"}); err != nil {
		t.Fatalf("seed site: %v", err)
	}
	ts := httptest.NewServer(server.New(engine.NewWithDB(d)).Handler())
	t.Cleanup(ts.Close)
	return ts.URL, "apod_rootkey"
}

// runCLI executes the root command with args, capturing everything written to
// stdout (the commands print there directly).
func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs(args)
	execErr := rootCmd.Execute()

	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out), execErr
}

func TestE2EListSites(t *testing.T) {
	url, key := newE2EServer(t)
	out, err := runCLI(t, "list", "--remote", url, "--key", key)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "ex.com") || !strings.Contains(out, "php") {
		t.Errorf("list output missing seeded site:\n%s", out)
	}
}

func TestE2EEnvRoundTrip(t *testing.T) {
	url, key := newE2EServer(t)

	// set
	out, err := runCLI(t, "env", "set", "ex.com", "APP_ENV=production", "--remote", url, "--key", key)
	if err != nil {
		t.Fatalf("env set: %v", err)
	}
	if !strings.Contains(out, "Set APP_ENV=production") {
		t.Errorf("env set output = %q", out)
	}

	// list reflects the value
	out, err = runCLI(t, "env", "list", "ex.com", "--remote", url, "--key", key)
	if err != nil {
		t.Fatalf("env list: %v", err)
	}
	if !strings.Contains(out, "APP_ENV") || !strings.Contains(out, "production") {
		t.Errorf("env list missing the set var:\n%s", out)
	}

	// unset removes it
	if _, err := runCLI(t, "env", "unset", "ex.com", "APP_ENV", "--remote", url, "--key", key); err != nil {
		t.Fatalf("env unset: %v", err)
	}
	out, _ = runCLI(t, "env", "list", "ex.com", "--remote", url, "--key", key)
	if strings.Contains(out, "production") {
		t.Errorf("env still present after unset:\n%s", out)
	}
}

func TestE2EAuthRejectsBadKey(t *testing.T) {
	url, _ := newE2EServer(t)
	if _, err := runCLI(t, "list", "--remote", url, "--key", "apod_wrongkey"); err == nil {
		t.Error("expected auth failure with a bad API key")
	}
}

func TestE2EUnknownSiteErrors(t *testing.T) {
	url, key := newE2EServer(t)
	if _, err := runCLI(t, "env", "list", "nope.com", "--remote", url, "--key", key); err == nil {
		t.Error("expected error listing env of unknown site")
	}
}

func TestE2EUserList(t *testing.T) {
	url, key := newE2EServer(t)
	out, err := runCLI(t, "user", "list", "--remote", url, "--key", key)
	if err != nil {
		t.Fatalf("user list: %v", err)
	}
	if !strings.Contains(out, "root") || !strings.Contains(out, "admin") {
		t.Errorf("user list missing seeded admin:\n%s", out)
	}
}

func TestE2ETokenRoundTrip(t *testing.T) {
	url, key := newE2EServer(t)

	out, err := runCLI(t, "token", "create", "ci-token", "--abilities", "read,write", "--remote", url, "--key", key)
	if err != nil {
		t.Fatalf("token create: %v", err)
	}
	if !strings.Contains(out, "ci-token") || !strings.Contains(out, "apod_") {
		t.Errorf("token create output missing name/token:\n%s", out)
	}

	out, err = runCLI(t, "token", "list", "--remote", url, "--key", key)
	if err != nil {
		t.Fatalf("token list: %v", err)
	}
	if !strings.Contains(out, "ci-token") {
		t.Errorf("token list missing created token:\n%s", out)
	}
}

func TestE2EDomainListEmpty(t *testing.T) {
	url, key := newE2EServer(t)
	out, err := runCLI(t, "domain", "list", "ex.com", "--remote", url, "--key", key)
	if err != nil {
		t.Fatalf("domain list: %v", err)
	}
	if !strings.Contains(out, "No domains found") {
		t.Errorf("expected empty domain list, got:\n%s", out)
	}
}
