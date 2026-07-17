package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

	// AddDomain materializes alias routing into the Traefik dynamic dir —
	// point it at a temp dir so the test never touches /etc.
	dynDir := t.TempDir()
	orig := traefikDynamicDir
	traefikDynamicDir = dynDir
	t.Cleanup(func() { traefikDynamicDir = orig })

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

	// The accepted alias must actually be materialized into a Traefik router
	// (the whole point of AddDomain — a DB row alone routes nothing).
	aliasFile := filepath.Join(dynDir, "aliases-a.example.com.toml")
	body, err := os.ReadFile(aliasFile)
	if err != nil {
		t.Fatalf("alias routing file not written: %v", err)
	}
	if !contains(string(body), "www.a.example.com") {
		t.Errorf("alias router should route the new domain:\n%s", body)
	}

	// Removing the alias must tear the router back down.
	if err := e.RemoveDomain(ctx, "a.example.com", "www.a.example.com"); err != nil {
		t.Fatalf("remove alias: %v", err)
	}
	if _, err := os.Stat(aliasFile); !os.IsNotExist(err) {
		t.Errorf("alias router file should be removed once no aliases remain, stat err=%v", err)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
