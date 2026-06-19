package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/aystro/apod/internal/db"
	"github.com/aystro/apod/internal/models"
)

// A shared network must not bridge two different owners' sites — AddSiteToNetwork
// enforces site.Owner == network.Owner before any docker work.
func TestAddSiteToNetworkSameOwnerEnforced(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	e := NewWithDB(d)
	ctx := context.Background()

	if err := d.CreateSharedNetwork("analytics", "alice"); err != nil {
		t.Fatal(err)
	}
	if err := d.CreateSite(&models.Site{Domain: "bob-site.com", Owner: "bob"}); err != nil {
		t.Fatal(err)
	}

	// Cross-owner join is forbidden (returns before any docker call).
	if err := e.AddSiteToNetwork(ctx, "analytics", "bob-site.com"); err == nil {
		t.Error("cross-owner join should be forbidden")
	} else if ErrorKindOf(err) != KindForbidden {
		t.Errorf("want Forbidden, got %v", err)
	}

	// Unknown network / site are NotFound, also before docker.
	if err := e.AddSiteToNetwork(ctx, "nope", "bob-site.com"); ErrorKindOf(err) != KindNotFound {
		t.Errorf("unknown network: want NotFound, got %v", err)
	}
	if err := e.AddSiteToNetwork(ctx, "analytics", "ghost.com"); ErrorKindOf(err) != KindNotFound {
		t.Errorf("unknown site: want NotFound, got %v", err)
	}
}
