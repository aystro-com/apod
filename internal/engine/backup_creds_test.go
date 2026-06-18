package engine

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestReadEnvFileValue(t *testing.T) {
	dir := t.TempDir()
	env := "APP_ENV=production\nDB_PASSWORD=\"s3cr3t pass\"\nDB_HOST=db\n# comment\nEMPTY=\n"
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(env), 0600); err != nil {
		t.Fatal(err)
	}
	if got := readEnvFileValue(path, "DB_PASSWORD"); got != "s3cr3t pass" {
		t.Errorf("DB_PASSWORD = %q, want %q (quotes trimmed)", got, "s3cr3t pass")
	}
	if got := readEnvFileValue(path, "DB_HOST"); got != "db" {
		t.Errorf("DB_HOST = %q, want db", got)
	}
	if got := readEnvFileValue(path, "EMPTY"); got != "" {
		t.Errorf("EMPTY = %q, want empty", got)
	}
	if got := readEnvFileValue(path, "MISSING"); got != "" {
		t.Errorf("MISSING = %q, want empty", got)
	}
	if got := readEnvFileValue(filepath.Join(dir, "nope"), "X"); got != "" {
		t.Errorf("missing file should yield empty, got %q", got)
	}
}

func TestDBPasswordFromZip(t *testing.T) {
	// Build an in-memory backup zip containing files/.env.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("files/.env")
	w.Write([]byte("APP_ENV=production\nDB_PASSWORD=fromenv123\n"))
	other, _ := zw.Create("files/index.php")
	other.Write([]byte("<?php"))
	zw.Close()

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if got := dbPasswordFromZip(zr); got != "fromenv123" {
		t.Errorf("dbPasswordFromZip = %q, want fromenv123", got)
	}

	// A zip without files/.env yields "".
	var buf2 bytes.Buffer
	zw2 := zip.NewWriter(&buf2)
	w2, _ := zw2.Create("files/app.js")
	w2.Write([]byte("x"))
	zw2.Close()
	zr2, _ := zip.NewReader(bytes.NewReader(buf2.Bytes()), int64(buf2.Len()))
	if got := dbPasswordFromZip(zr2); got != "" {
		t.Errorf("no .env should yield empty, got %q", got)
	}
}
