package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/aystro/apod/internal/db"
)

func newProcEngine(t *testing.T) *Engine {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return NewWithDB(d)
}

func TestScaleProcessValidation(t *testing.T) {
	e := newProcEngine(t)

	// Out-of-range replica counts are rejected before any DB/Docker work.
	if err := e.ScaleProcess(context.Background(), "x.com", "queue", -1); err == nil {
		t.Error("negative replicas should be rejected")
	}
	if err := e.ScaleProcess(context.Background(), "x.com", "queue", maxReplicas+1); err == nil {
		t.Error("over-max replicas should be rejected")
	}

	// A valid count for a non-existent site fails at lookup (no panic, clear error).
	if err := e.ScaleProcess(context.Background(), "missing.com", "queue", 2); err == nil {
		t.Error("scaling a missing site should error")
	}
}
