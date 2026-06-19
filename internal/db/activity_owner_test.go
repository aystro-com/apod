package db

import (
	"path/filepath"
	"testing"

	"github.com/aystro/apod/internal/models"
)

// A non-admin's activity feed must only contain their own sites' operations.
func TestListOperationsByOwner(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	if err := d.CreateSite(&models.Site{Domain: "a.example.com", Owner: "alice", Driver: "static"}); err != nil {
		t.Fatal(err)
	}
	if err := d.CreateSite(&models.Site{Domain: "b.example.com", Owner: "bob", Driver: "static"}); err != nil {
		t.Fatal(err)
	}
	d.LogOperation("a.example.com", "deploy", "", "success")
	d.LogOperation("b.example.com", "deploy", "", "success")

	ops, err := d.ListOperationsByOwner("alice", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].SiteDomain != "a.example.com" {
		t.Fatalf("alice should see only her site's activity, got %+v", ops)
	}

	// Admin (ListAllOperations) sees both.
	all, _ := d.ListAllOperations(50)
	if len(all) != 2 {
		t.Errorf("admin should see all activity, got %d", len(all))
	}
}
