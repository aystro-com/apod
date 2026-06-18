package engine

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/aystro/apod/internal/db"
)

func TestSecretValueRoundTrip(t *testing.T) {
	e := &Engine{dataDir: t.TempDir()}

	secret := "s3cr3t-jwt-värdé/+="
	enc, err := e.encryptSecretValue(secret)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !strings.HasPrefix(enc, secretEncPrefix) {
		t.Errorf("ciphertext missing marker prefix: %q", enc)
	}
	if strings.Contains(enc, secret) {
		t.Error("plaintext leaked into ciphertext")
	}
	got, err := e.decryptSecretValue(enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != secret {
		t.Errorf("round-trip = %q, want %q", got, secret)
	}

	// Legacy plaintext (no prefix) is returned unchanged.
	if got, _ := e.decryptSecretValue("legacy-plain"); got != "legacy-plain" {
		t.Errorf("legacy plaintext = %q, want unchanged", got)
	}
}

func TestSiteSecretStoredEncrypted(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	e := NewWithDB(d)
	e.dataDir = t.TempDir()

	if err := e.setSiteSecret("x.test", "db_password", "hunter2"); err != nil {
		t.Fatalf("set: %v", err)
	}
	// At rest the raw DB value must be ciphertext, not the plaintext.
	raw, ok, _ := d.GetSiteSecret("x.test", "db_password")
	if !ok || raw == "hunter2" || !strings.HasPrefix(raw, secretEncPrefix) {
		t.Errorf("secret not encrypted at rest: %q", raw)
	}
	// Through the wrapper it decrypts back.
	got, ok, err := e.getSiteSecret("x.test", "db_password")
	if err != nil || !ok || got != "hunter2" {
		t.Errorf("getSiteSecret = %q ok=%v err=%v, want hunter2", got, ok, err)
	}
}
