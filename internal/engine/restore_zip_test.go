package engine

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// zipWithEntry builds an in-memory zip archive containing a single entry.
func zipWithEntry(t *testing.T, name string, content []byte) *zip.Reader {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatalf("create entry: %v", err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	return zr
}

// restoreZipEntry must never write more than `limit` bytes, so the running
// total in RestoreBackup/ImportSite can detect and abort a decompression bomb.
func TestRestoreZipEntryHonorsLimit(t *testing.T) {
	zr := zipWithEntry(t, "files/big.bin", bytes.Repeat([]byte("A"), 10_000))
	dest := filepath.Join(t.TempDir(), "out.bin")

	n, err := restoreZipEntry(zr.File[0], filepath.Dir(dest), dest, 100)
	if err != nil {
		t.Fatalf("restoreZipEntry: %v", err)
	}
	if n > 100 {
		t.Errorf("wrote %d bytes, limit was 100 — decompression bomb would not be capped", n)
	}
	fi, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	if fi.Size() > 100 {
		t.Errorf("file is %d bytes, expected <= 100", fi.Size())
	}
}

// A full-sized entry under the limit must be written intact.
func TestRestoreZipEntryWritesWholeEntryUnderLimit(t *testing.T) {
	content := []byte("hello world")
	zr := zipWithEntry(t, "files/small.txt", content)
	dest := filepath.Join(t.TempDir(), "small.txt")

	n, err := restoreZipEntry(zr.File[0], filepath.Dir(dest), dest, 1<<20)
	if err != nil {
		t.Fatalf("restoreZipEntry: %v", err)
	}
	if int(n) != len(content) {
		t.Errorf("wrote %d bytes, want %d", n, len(content))
	}
	got, _ := os.ReadFile(dest)
	if !bytes.Equal(got, content) {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}
}
