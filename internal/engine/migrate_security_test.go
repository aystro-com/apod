package engine

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeImportZip builds a minimal export archive carrying the given metadata.
func writeImportZip(t *testing.T, meta backupMetadata) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("metadata.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(w).Encode(meta); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "export.zip")
	if err := os.WriteFile(p, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// ImportSite must reject a crafted metadata domain before it can reach the
// database-name derivation that flows into `sh -c` mysql/psql commands — even
// when a clean newDomain is supplied (the bug was that meta.Domain was used
// unvalidated to build the DB name).
func TestImportSiteRejectsMaliciousMetadataDomain(t *testing.T) {
	e := &Engine{}
	zip := writeImportZip(t, backupMetadata{
		Domain:     "x$(touch /tmp/pwn)",
		Driver:     "postgres",
		DBPassword: "p", // triggers the preserve-DB path that uses srcDBName
	})
	err := e.ImportSite(t.Context(), zip, "legit.example.com", "alice", "")
	if err == nil || !strings.Contains(err.Error(), "domain") {
		t.Fatalf("expected a domain-validation error, got %v", err)
	}
}

// A crafted env key/value that would inject extra .env lines must be rejected.
func TestImportSiteRejectsEnvInjection(t *testing.T) {
	e := &Engine{}
	zip := writeImportZip(t, backupMetadata{
		Domain: "legit.example.com",
		Driver: "static",
		Env:    map[string]string{"OK\nINJECTED": "1"},
	})
	err := e.ImportSite(t.Context(), zip, "", "alice", "")
	if err == nil || !strings.Contains(err.Error(), "env") {
		t.Fatalf("expected an env-validation error, got %v", err)
	}
}
