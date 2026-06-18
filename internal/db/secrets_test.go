package db

import "testing"

func TestSiteSecrets(t *testing.T) {
	d := newTestDB(t)

	// Missing => ("", false).
	if v, ok, err := d.GetSiteSecret("a.com", "db_password"); err != nil || ok || v != "" {
		t.Fatalf("expected no secret, got (%q,%v,%v)", v, ok, err)
	}

	if err := d.SetSiteSecret("a.com", "db_password", "p4ss"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := d.SetSiteSecret("a.com", "jwt_secret", "jjj"); err != nil {
		t.Fatalf("set: %v", err)
	}
	v, ok, err := d.GetSiteSecret("a.com", "db_password")
	if err != nil || !ok || v != "p4ss" {
		t.Fatalf("get = (%q,%v,%v), want (p4ss,true,nil)", v, ok, err)
	}

	// Upsert replaces.
	if err := d.SetSiteSecret("a.com", "db_password", "new"); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if v, _, _ := d.GetSiteSecret("a.com", "db_password"); v != "new" {
		t.Fatalf("upsert value = %q, want new", v)
	}

	// Scoped per site; delete-all clears one site only.
	if err := d.SetSiteSecret("b.com", "db_password", "bbb"); err != nil {
		t.Fatalf("set b: %v", err)
	}
	if err := d.DeleteSiteSecrets("a.com"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok, _ := d.GetSiteSecret("a.com", "db_password"); ok {
		t.Error("a.com secrets should be cleared")
	}
	if v, ok, _ := d.GetSiteSecret("b.com", "db_password"); !ok || v != "bbb" {
		t.Error("b.com secrets must be untouched")
	}
}
