package db

import (
	"testing"

	"github.com/aystro/apod/internal/models"
)

func TestAddDomain(t *testing.T) {
	d := openTestDB(t)
	d.CreateSite(&models.Site{Domain: "example.com", Driver: "static", RAM: "128M", CPU: "1"})
	site, _ := d.GetSite("example.com")

	err := d.AddDomain(site.ID, "www.example.com", false)
	if err != nil {
		t.Fatalf("AddDomain: %v", err)
	}
}

func TestAddDomainDuplicate(t *testing.T) {
	d := openTestDB(t)
	d.CreateSite(&models.Site{Domain: "example.com", Driver: "static", RAM: "128M", CPU: "1"})
	site, _ := d.GetSite("example.com")

	d.AddDomain(site.ID, "www.example.com", false)
	err := d.AddDomain(site.ID, "www.example.com", false)
	if err == nil {
		t.Fatal("expected error for duplicate domain")
	}
}

func TestListDomains(t *testing.T) {
	d := openTestDB(t)
	d.CreateSite(&models.Site{Domain: "example.com", Driver: "static", RAM: "128M", CPU: "1"})
	site, _ := d.GetSite("example.com")

	d.AddDomain(site.ID, "example.com", true)
	d.AddDomain(site.ID, "www.example.com", false)
	d.AddDomain(site.ID, "shop.example.com", false)

	domains, err := d.ListDomains(site.ID)
	if err != nil {
		t.Fatalf("ListDomains: %v", err)
	}
	if len(domains) != 3 {
		t.Errorf("got %d domains, want 3", len(domains))
	}
}

func TestRemoveDomain(t *testing.T) {
	d := openTestDB(t)
	d.CreateSite(&models.Site{Domain: "example.com", Driver: "static", RAM: "128M", CPU: "1"})
	site, _ := d.GetSite("example.com")

	d.AddDomain(site.ID, "www.example.com", false)
	err := d.RemoveDomain("www.example.com")
	if err != nil {
		t.Fatalf("RemoveDomain: %v", err)
	}

	domains, _ := d.ListDomains(site.ID)
	if len(domains) != 0 {
		t.Errorf("got %d domains, want 0", len(domains))
	}
}

func TestGetSiteByDomain(t *testing.T) {
	d := openTestDB(t)
	d.CreateSite(&models.Site{Domain: "example.com", Driver: "static", RAM: "128M", CPU: "1"})
	site, _ := d.GetSite("example.com")

	d.AddDomain(site.ID, "example.com", true)
	d.AddDomain(site.ID, "www.example.com", false)

	found, err := d.GetSiteByDomain("www.example.com")
	if err != nil {
		t.Fatalf("GetSiteByDomain: %v", err)
	}
	if found.Domain != "example.com" {
		t.Errorf("got domain %q, want example.com", found.Domain)
	}
}
