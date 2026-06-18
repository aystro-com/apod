package storage

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeJoin(t *testing.T) {
	base := "/var/lib/apod/backups"

	ok := []struct{ key, want string }{
		{"site/a.zip", "/var/lib/apod/backups/site/a.zip"},
		{"a.zip", "/var/lib/apod/backups/a.zip"},
		{"/leading/slash.zip", "/var/lib/apod/backups/leading/slash.zip"},
		{"nested/../a.zip", "/var/lib/apod/backups/a.zip"},
	}
	for _, c := range ok {
		got, err := safeJoin(base, c.key)
		if err != nil {
			t.Errorf("safeJoin(%q) unexpected error: %v", c.key, err)
			continue
		}
		if got != c.want {
			t.Errorf("safeJoin(%q) = %q, want %q", c.key, got, c.want)
		}
	}

	// Traversal keys are neutralised, not escaped: the key is forced relative
	// to root first, so "../" sequences resolve harmlessly inside the base.
	// The security guarantee is containment — the result is always within base.
	traversal := []string{
		"../../etc/passwd",
		"../outside.zip",
		"sub/../../escape.zip",
		"/../../../../root/.ssh/authorized_keys",
	}
	cleanBase := filepath.Clean(base)
	for _, key := range traversal {
		got, err := safeJoin(base, key)
		if err != nil {
			continue // rejecting is also acceptable
		}
		if got != cleanBase && !strings.HasPrefix(got, cleanBase+string(filepath.Separator)) {
			t.Errorf("safeJoin(%q) = %q escaped base %q", key, got, cleanBase)
		}
	}
}

func TestLocalContainsTraversalKey(t *testing.T) {
	dir := t.TempDir()
	store := NewLocal(dir)

	// A traversal key must never write outside the base directory; it is
	// neutralised to a path inside it.
	if err := store.Upload(nil, "../escape.zip", bytes.NewReader([]byte("x"))); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(dir), "escape.zip")); !os.IsNotExist(statErr) {
		t.Fatal("traversal write escaped the base directory")
	}
	// It lands inside the base instead.
	if _, statErr := os.Stat(filepath.Join(dir, "escape.zip")); statErr != nil {
		t.Errorf("neutralised key should land inside base: %v", statErr)
	}
}

func TestNewFactory(t *testing.T) {
	// local with default path.
	s, err := New("local", map[string]string{})
	if err != nil || s == nil {
		t.Fatalf("local default: %v", err)
	}

	// local with explicit path.
	if _, err := New("local", map[string]string{"path": t.TempDir()}); err != nil {
		t.Fatalf("local custom path: %v", err)
	}

	// Unknown driver is an error.
	if _, err := New("floppydisk", map[string]string{}); err == nil {
		t.Error("unknown driver should error")
	}

	// s3 requires a bucket.
	if _, err := New("s3", map[string]string{}); err == nil {
		t.Error("s3 without bucket should error")
	}
	if _, err := New("s3", map[string]string{"bucket": "b"}); err != nil {
		t.Errorf("s3 with bucket should succeed: %v", err)
	}

	// r2 requires account_id.
	if _, err := New("r2", map[string]string{"bucket": "b"}); err == nil {
		t.Error("r2 without account_id should error")
	}
}

func TestNewR2SetsEndpoint(t *testing.T) {
	// R2 must derive the Cloudflare endpoint from the account ID.
	cfg := map[string]string{"bucket": "b", "account_id": "abc123"}
	if _, err := NewR2(cfg); err != nil {
		t.Fatalf("NewR2: %v", err)
	}
	if want := "https://abc123.r2.cloudflarestorage.com"; cfg["endpoint"] != want {
		t.Errorf("endpoint = %q, want %q", cfg["endpoint"], want)
	}
	if cfg["region"] != "auto" {
		t.Errorf("region = %q, want auto", cfg["region"])
	}
}

func TestNewSFTPValidation(t *testing.T) {
	if _, err := NewSFTP(map[string]string{"user": "u"}); err == nil {
		t.Error("sftp without host should error")
	}
	if _, err := NewSFTP(map[string]string{"host": "h"}); err == nil {
		t.Error("sftp without user should error")
	}

	s, err := NewSFTP(map[string]string{"host": "h", "user": "u"})
	if err != nil {
		t.Fatalf("valid sftp: %v", err)
	}
	// Defaults are applied.
	if s.port != "22" {
		t.Errorf("port = %q, want 22", s.port)
	}
	if s.basePath != "/backups" {
		t.Errorf("basePath = %q, want /backups", s.basePath)
	}
}

func TestNewSFTPHostKeyPinned(t *testing.T) {
	s, err := NewSFTP(map[string]string{
		"host": "h", "user": "u", "host_key": "ssh-ed25519 AAAAfake",
	})
	if err != nil {
		t.Fatalf("NewSFTP: %v", err)
	}
	if !strings.Contains(s.hostKey, "ssh-ed25519") {
		t.Errorf("pinned host key not stored: %q", s.hostKey)
	}
}
