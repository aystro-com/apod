package db

import (
	"path/filepath"
	"testing"

	"github.com/aystro/apod/internal/models"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestCreateSite(t *testing.T) {
	d := openTestDB(t)
	site := &models.Site{Domain: "example.com", Driver: "wordpress", RAM: "512M", CPU: "1"}
	err := d.CreateSite(site)
	if err != nil {
		t.Fatalf("CreateSite: %v", err)
	}
	if site.ID == 0 {
		t.Error("expected site ID to be set")
	}
}

func TestGetSite(t *testing.T) {
	d := openTestDB(t)
	d.CreateSite(&models.Site{Domain: "example.com", Driver: "wordpress", RAM: "256M", CPU: "1"})
	site, err := d.GetSite("example.com")
	if err != nil {
		t.Fatalf("GetSite: %v", err)
	}
	if site.Domain != "example.com" {
		t.Errorf("got domain %q, want example.com", site.Domain)
	}
	if site.Driver != "wordpress" {
		t.Errorf("got driver %q, want wordpress", site.Driver)
	}
}

func TestGetSiteNotFound(t *testing.T) {
	d := openTestDB(t)
	_, err := d.GetSite("nope.com")
	if err == nil {
		t.Fatal("expected error for missing site")
	}
}

func TestListSites(t *testing.T) {
	d := openTestDB(t)
	d.CreateSite(&models.Site{Domain: "a.com", Driver: "static", RAM: "128M", CPU: "1"})
	d.CreateSite(&models.Site{Domain: "b.com", Driver: "wordpress", RAM: "512M", CPU: "2"})
	sites, err := d.ListSites()
	if err != nil {
		t.Fatalf("ListSites: %v", err)
	}
	if len(sites) != 2 {
		t.Errorf("got %d sites, want 2", len(sites))
	}
}

func TestUpdateSiteStatus(t *testing.T) {
	d := openTestDB(t)
	d.CreateSite(&models.Site{Domain: "example.com", Driver: "static", RAM: "128M", CPU: "1"})
	err := d.UpdateSiteStatus("example.com", "running")
	if err != nil {
		t.Fatalf("UpdateSiteStatus: %v", err)
	}
	site, _ := d.GetSite("example.com")
	if site.Status != "running" {
		t.Errorf("got status %q, want running", site.Status)
	}
}

func TestDeleteSite(t *testing.T) {
	d := openTestDB(t)
	d.CreateSite(&models.Site{Domain: "example.com", Driver: "static", RAM: "128M", CPU: "1"})
	err := d.DeleteSite("example.com")
	if err != nil {
		t.Fatalf("DeleteSite: %v", err)
	}
	_, err = d.GetSite("example.com")
	if err == nil {
		t.Fatal("expected error after deletion")
	}
}

func TestUpdateSiteConfig(t *testing.T) {
	d := openTestDB(t)
	d.CreateSite(&models.Site{Domain: "example.com", Driver: "static", RAM: "128M", CPU: "1"})

	err := d.UpdateSiteConfig("example.com", map[string]string{"ram": "512M", "cpu": "2"})
	if err != nil {
		t.Fatalf("UpdateSiteConfig: %v", err)
	}

	site, _ := d.GetSite("example.com")
	if site.RAM != "512M" {
		t.Errorf("got RAM %q, want 512M", site.RAM)
	}
	if site.CPU != "2" {
		t.Errorf("got CPU %q, want 2", site.CPU)
	}
}

func TestUpdateSiteConfigUnknownField(t *testing.T) {
	d := openTestDB(t)
	d.CreateSite(&models.Site{Domain: "example.com", Driver: "static", RAM: "128M", CPU: "1"})

	err := d.UpdateSiteConfig("example.com", map[string]string{"unknown": "value"})
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
}
