package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Local struct {
	baseDir string
}

func NewLocal(baseDir string) *Local {
	return &Local{baseDir: baseDir}
}

func (l *Local) Upload(_ context.Context, key string, reader io.Reader) error {
	path, err := safeJoin(l.baseDir, key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, reader); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

func (l *Local) Download(_ context.Context, key string, writer io.Writer) error {
	path, err := safeJoin(l.baseDir, key)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(writer, f); err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	return nil
}

func (l *Local) Delete(_ context.Context, key string) error {
	path, err := safeJoin(l.baseDir, key)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func (l *Local) List(_ context.Context, prefix string) ([]string, error) {
	// Same traversal guard as Upload/Download/Delete — a raw join would let a
	// "../" prefix enumerate (and via the returned keys, read) outside baseDir.
	dir, err := safeJoin(l.baseDir, prefix)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read directory: %w", err)
	}
	var keys []string
	for _, entry := range entries {
		if !entry.IsDir() {
			keys = append(keys, strings.TrimPrefix(filepath.Join(prefix, entry.Name()), "/"))
		}
	}
	return keys, nil
}
