package db

import (
	"path/filepath"
	"testing"

	"github.com/aystro/apod/internal/models"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	d, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// TestDeleteFTPAccountForSiteScoped verifies an FTP account can't be deleted
// while specifying the wrong site (the IDOR that the unscoped delete allowed).
func TestDeleteFTPAccountForSiteScoped(t *testing.T) {
	d := newTestDB(t)
	if err := d.CreateFTPAccount("alice.com", "deploy", "pw-123456"); err != nil {
		t.Fatalf("CreateFTPAccount: %v", err)
	}

	// Wrong site must NOT delete it.
	if err := d.DeleteFTPAccountForSite("bob.com", "deploy"); err == nil {
		t.Fatal("DeleteFTPAccountForSite(wrong site) succeeded, want error")
	}
	accts, _ := d.ListFTPAccounts("alice.com")
	if len(accts) != 1 {
		t.Fatalf("account was deleted via wrong site; have %d accounts", len(accts))
	}

	// Correct site deletes it.
	if err := d.DeleteFTPAccountForSite("alice.com", "deploy"); err != nil {
		t.Fatalf("DeleteFTPAccountForSite(correct site): %v", err)
	}
	accts, _ = d.ListFTPAccounts("alice.com")
	if len(accts) != 0 {
		t.Fatalf("account not deleted; have %d", len(accts))
	}
}

// TestRemoveDomainForSiteScoped verifies an alias can't be removed by another
// site id.
func TestRemoveDomainForSiteScoped(t *testing.T) {
	d := newTestDB(t)
	idA := mustCreateSite(t, d, "a.com")
	idB := mustCreateSite(t, d, "b.com")

	if err := d.AddDomain(idA, "alias.a.com", false); err != nil {
		t.Fatalf("AddDomain: %v", err)
	}

	// site B must not be able to remove site A's alias.
	if err := d.RemoveDomainForSite(idB, "alias.a.com"); err == nil {
		t.Fatal("RemoveDomainForSite(wrong site) succeeded, want error")
	}
	domains, _ := d.ListDomains(idA)
	found := false
	for _, dm := range domains {
		if dm == "alias.a.com" {
			found = true
		}
	}
	if !found {
		t.Fatal("alias was removed via wrong site id")
	}

	// Correct site removes it.
	if err := d.RemoveDomainForSite(idA, "alias.a.com"); err != nil {
		t.Fatalf("RemoveDomainForSite(correct site): %v", err)
	}
}

func mustCreateSite(t *testing.T, d *DB, domain string) int64 {
	t.Helper()
	if err := d.CreateSite(&models.Site{Domain: domain, Driver: "static", RAM: "128M", CPU: "1"}); err != nil {
		t.Fatalf("CreateSite(%s): %v", domain, err)
	}
	s, err := d.GetSite(domain)
	if err != nil {
		t.Fatalf("GetSite(%s): %v", domain, err)
	}
	return s.ID
}
