package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/aystro/apod/internal/db"
	"github.com/aystro/apod/internal/models"
)

// A domain may belong to only one site — AddDomain must reject one already in
// use (as a primary domain or an alias) with a clear conflict.
func TestAddDomainRejectsTakenDomain(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	e := NewWithDB(d)
	ctx := context.Background()

	// Two sites, each with its primary domain registered in the domains table.
	for _, dom := range []string{"a.example.com", "b.example.com"} {
		if err := d.CreateSite(&models.Site{Domain: dom, Owner: "alice", Driver: "static"}); err != nil {
			t.Fatal(err)
		}
		s, _ := d.GetSite(dom)
		if err := d.AddDomain(s.ID, dom, true); err != nil {
			t.Fatal(err)
		}
	}

	// Adding b's primary domain as an alias of a must conflict.
	if err := e.AddDomain(ctx, "a.example.com", "b.example.com"); err == nil {
		t.Error("adding an in-use domain should conflict")
	} else if ErrorKindOf(err) != KindConflict {
		t.Errorf("want Conflict, got %v", err)
	}

	// A fresh alias is accepted, then a second site can't claim it.
	if err := e.AddDomain(ctx, "a.example.com", "www.a.example.com"); err != nil {
		t.Fatalf("fresh alias should be accepted: %v", err)
	}
	if err := e.AddDomain(ctx, "b.example.com", "www.a.example.com"); ErrorKindOf(err) != KindConflict {
		t.Errorf("claiming another site's alias should conflict, got %v", err)
	}
}
