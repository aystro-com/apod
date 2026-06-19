package engine

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/aystro/apod/internal/db"
)

func newStorageTestEngine(t *testing.T) *Engine {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return NewWithDB(d)
}

// AddStorageConfig must reject a misconfigured backend at add time with a typed
// Invalid error (→ 400), not store it and fail silently at backup time.
func TestAddStorageConfigValidatesDriver(t *testing.T) {
	e := newStorageTestEngine(t)

	cases := []struct {
		name    string
		driver  string
		config  string
		wantSub string
	}{
		{"unknown driver", "bogus", `{}`, "unknown storage driver"},
		{"sftp without host key", "sftp", `{"host":"h","user":"u"}`, "host_key is required"},
		{"s3 http endpoint", "s3", `{"bucket":"b","endpoint":"http://minio.local"}`, "must use https"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := e.AddStorageConfig("cfg-"+tc.name, tc.driver, tc.config)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if ErrorKindOf(err) != KindInvalid {
				t.Errorf("kind = %v, want KindInvalid", ErrorKindOf(err))
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantSub)
			}
		})
	}
}

// A valid config is accepted; a second add under the same name is a Conflict
// (→ 409), not an opaque 500.
func TestAddStorageConfigDuplicateIsConflict(t *testing.T) {
	e := newStorageTestEngine(t)

	cfg := `{"path":"/var/lib/apod/backups"}`
	if err := e.AddStorageConfig("dup", "local", cfg); err != nil {
		t.Fatalf("first add: %v", err)
	}
	err := e.AddStorageConfig("dup", "local", cfg)
	if err == nil {
		t.Fatal("expected conflict on duplicate name")
	}
	if ErrorKindOf(err) != KindConflict {
		t.Errorf("kind = %v, want KindConflict", ErrorKindOf(err))
	}
}
