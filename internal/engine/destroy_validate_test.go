package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/aystro/apod/internal/db"
)

// DestroySite must reject a malformed domain before it can reach any
// filesystem path (with purge=true the domain flows into os.RemoveAll).
func TestDestroySiteRejectsBadDomain(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	e := NewWithDB(d)

	for _, bad := range []string{"..", "../../etc", "a/b", "", "x;y"} {
		if err := e.DestroySite(context.Background(), bad, true); err == nil {
			t.Errorf("DestroySite(%q, purge) = nil, want validation error", bad)
		} else if ErrorKindOf(err) != KindInvalid {
			t.Errorf("DestroySite(%q): want Invalid, got %v", bad, err)
		}
	}
}
