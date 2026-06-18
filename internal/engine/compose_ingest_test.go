package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/aystro/apod/internal/db"
)

// CreateSiteFromCompose must reject inputs it cannot route BEFORE provisioning
// anything — these checks run ahead of any Docker work, so they're unit-testable
// with a DB-only engine.
func TestCreateSiteFromComposeRejectsBadInput(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	e := NewWithDB(d)
	ctx := context.Background()
	opts := CreateSiteOpts{Domain: "x.test", Owner: ""}

	if err := e.CreateSiteFromCompose(ctx, opts, ""); err == nil {
		t.Error("empty compose should be rejected")
	} else if ErrorKindOf(err) != KindInvalid {
		t.Errorf("empty compose: want Invalid, got %v", err)
	}

	// A compose with no published port cannot be routed → Invalid, not a panic
	// or a half-created site.
	noPorts := "services:\n  worker:\n    image: busybox\n    command: sleep 100\n"
	if err := e.CreateSiteFromCompose(ctx, opts, noPorts); err == nil {
		t.Error("portless compose should be rejected")
	} else if ErrorKindOf(err) != KindInvalid {
		t.Errorf("portless compose: want Invalid, got %v", err)
	}
}
