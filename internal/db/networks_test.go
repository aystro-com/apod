package db

import (
	"path/filepath"
	"testing"
)

func TestSharedNetworks(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	if err := d.CreateSharedNetwork("analytics", "alice"); err != nil {
		t.Fatal(err)
	}
	if err := d.AddNetworkMember("analytics", "erp.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := d.AddNetworkMember("analytics", "bi.example.com"); err != nil {
		t.Fatal(err)
	}
	// Idempotent add.
	if err := d.AddNetworkMember("analytics", "bi.example.com"); err != nil {
		t.Fatal(err)
	}

	sn, ok, err := d.GetSharedNetwork("analytics")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if sn.Owner != "alice" || len(sn.Members) != 2 {
		t.Errorf("got owner=%q members=%v", sn.Owner, sn.Members)
	}

	nets, err := d.ListSiteNetworks("bi.example.com")
	if err != nil || len(nets) != 1 || nets[0] != "analytics" {
		t.Errorf("ListSiteNetworks = %v, err=%v", nets, err)
	}

	// Owner filter.
	if got, _ := d.ListSharedNetworks("bob"); len(got) != 0 {
		t.Errorf("owner filter: got %d, want 0", len(got))
	}

	if err := d.RemoveNetworkMember("analytics", "bi.example.com"); err != nil {
		t.Fatal(err)
	}
	if m, _ := d.ListNetworkMembers("analytics"); len(m) != 1 || m[0] != "erp.example.com" {
		t.Errorf("after remove, members = %v", m)
	}

	// Removing a site from all networks (on destroy).
	if err := d.RemoveSiteFromAllNetworks("erp.example.com"); err != nil {
		t.Fatal(err)
	}
	if m, _ := d.ListNetworkMembers("analytics"); len(m) != 0 {
		t.Errorf("after RemoveSiteFromAllNetworks, members = %v", m)
	}

	if err := d.DeleteSharedNetwork("analytics"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := d.GetSharedNetwork("analytics"); ok {
		t.Error("network should be gone after delete")
	}
}
