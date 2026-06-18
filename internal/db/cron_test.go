package db

import "testing"

func TestCreateCronJob(t *testing.T) {
	d := openTestDB(t)
	id, err := d.CreateCronJob("example.com", "* * * * *", "php artisan schedule:run", "app")
	if err != nil {
		t.Fatalf("CreateCronJob: %v", err)
	}
	if id == 0 {
		t.Error("expected ID")
	}
}

func TestListCronJobs(t *testing.T) {
	d := openTestDB(t)
	d.CreateCronJob("example.com", "* * * * *", "cmd1", "app")
	d.CreateCronJob("example.com", "0 * * * *", "cmd2", "app")
	d.CreateCronJob("other.com", "0 0 * * *", "cmd3", "app")

	jobs, err := d.ListCronJobs("example.com")
	if err != nil {
		t.Fatalf("ListCronJobs: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("got %d, want 2", len(jobs))
	}
}

func TestDeleteCronJobForSite(t *testing.T) {
	d := openTestDB(t)
	id, _ := d.CreateCronJob("example.com", "* * * * *", "cmd1", "app")

	// A different site must not be able to delete this job by ID (IDOR).
	if err := d.DeleteCronJobForSite(id, "attacker.com"); err == nil {
		t.Error("cross-site delete should fail")
	}
	if jobs, _ := d.ListCronJobs("example.com"); len(jobs) != 1 {
		t.Fatalf("job removed by cross-site delete: got %d, want 1", len(jobs))
	}

	// The owning site can delete it.
	if err := d.DeleteCronJobForSite(id, "example.com"); err != nil {
		t.Fatalf("DeleteCronJobForSite: %v", err)
	}
	if jobs, _ := d.ListCronJobs("example.com"); len(jobs) != 0 {
		t.Errorf("got %d, want 0", len(jobs))
	}
}

func TestListAllCronJobs(t *testing.T) {
	d := openTestDB(t)
	d.CreateCronJob("a.com", "* * * * *", "cmd1", "app")
	d.CreateCronJob("b.com", "0 * * * *", "cmd2", "app")
	jobs, err := d.ListAllCronJobs()
	if err != nil {
		t.Fatalf("ListAllCronJobs: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("got %d, want 2", len(jobs))
	}
}
