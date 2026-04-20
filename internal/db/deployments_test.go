package db

import "testing"

func TestCreateDeployment(t *testing.T) {
	d := openTestDB(t)
	id, err := d.CreateDeployment("example.com", "abc123", "main")
	if err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}
	if id == 0 {
		t.Error("expected deployment ID")
	}
}

func TestGetDeployment(t *testing.T) {
	d := openTestDB(t)
	id, _ := d.CreateDeployment("example.com", "abc123", "main")
	dep, err := d.GetDeployment(id)
	if err != nil {
		t.Fatalf("GetDeployment: %v", err)
	}
	if dep.CommitHash != "abc123" {
		t.Errorf("got hash %q", dep.CommitHash)
	}
	if dep.Status != "pending" {
		t.Errorf("got status %q", dep.Status)
	}
}

func TestUpdateDeploymentStatus(t *testing.T) {
	d := openTestDB(t)
	id, _ := d.CreateDeployment("example.com", "abc123", "main")
	d.UpdateDeploymentStatus(id, "success")
	dep, _ := d.GetDeployment(id)
	if dep.Status != "success" {
		t.Errorf("got status %q, want success", dep.Status)
	}
}

func TestListDeployments(t *testing.T) {
	d := openTestDB(t)
	d.CreateDeployment("example.com", "aaa", "main")
	d.CreateDeployment("example.com", "bbb", "main")
	deps, err := d.ListDeployments("example.com")
	if err != nil {
		t.Fatalf("ListDeployments: %v", err)
	}
	if len(deps) != 2 {
		t.Errorf("got %d, want 2", len(deps))
	}
}

func TestGetLatestDeployment(t *testing.T) {
	d := openTestDB(t)
	d.CreateDeployment("example.com", "first", "main")
	id2, _ := d.CreateDeployment("example.com", "second", "main")
	d.UpdateDeploymentStatus(id2, "success")
	dep, err := d.GetLatestDeployment("example.com")
	if err != nil {
		t.Fatalf("GetLatestDeployment: %v", err)
	}
	if dep.CommitHash != "second" {
		t.Errorf("got hash %q, want second", dep.CommitHash)
	}
}
