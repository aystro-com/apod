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

func TestDeleteCronJob(t *testing.T) {
	d := openTestDB(t)
	id, _ := d.CreateCronJob("example.com", "* * * * *", "cmd1", "app")
	err := d.DeleteCronJob(id)
	if err != nil {
		t.Fatalf("DeleteCronJob: %v", err)
	}
	jobs, _ := d.ListCronJobs("example.com")
	if len(jobs) != 0 {
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
