package storage

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalUploadAndDownload(t *testing.T) {
	dir := t.TempDir()
	store := NewLocal(dir)

	data := []byte("hello backup world")
	err := store.Upload(nil, "test/backup.zip", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	path := filepath.Join(dir, "test", "backup.zip")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("expected file to exist on disk")
	}

	var buf bytes.Buffer
	err = store.Download(nil, "test/backup.zip", &buf)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if buf.String() != "hello backup world" {
		t.Errorf("got %q, want hello backup world", buf.String())
	}
}

func TestLocalDelete(t *testing.T) {
	dir := t.TempDir()
	store := NewLocal(dir)

	store.Upload(nil, "test.zip", bytes.NewReader([]byte("data")))

	err := store.Delete(nil, "test.zip")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	path := filepath.Join(dir, "test.zip")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected file to be deleted")
	}
}

func TestLocalList(t *testing.T) {
	dir := t.TempDir()
	store := NewLocal(dir)

	store.Upload(nil, "site/a.zip", bytes.NewReader([]byte("a")))
	store.Upload(nil, "site/b.zip", bytes.NewReader([]byte("b")))
	store.Upload(nil, "other/c.zip", bytes.NewReader([]byte("c")))

	keys, err := store.List(nil, "site/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("got %d keys, want 2", len(keys))
	}
}

func TestLocalDownloadNotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewLocal(dir)

	var buf bytes.Buffer
	err := store.Download(nil, "nope.zip", &buf)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
