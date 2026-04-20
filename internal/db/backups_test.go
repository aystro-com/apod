package db

import "testing"

func TestCreateBackup(t *testing.T) {
	d := openTestDB(t)
	id, err := d.CreateBackup("example.com", "local", "/backups/example.com/backup_001.zip", 1024)
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	if id == 0 {
		t.Error("expected backup ID to be set")
	}
}

func TestListBackups(t *testing.T) {
	d := openTestDB(t)
	d.CreateBackup("example.com", "local", "/backups/a.zip", 1024)
	d.CreateBackup("example.com", "local", "/backups/b.zip", 2048)
	d.CreateBackup("other.com", "local", "/backups/c.zip", 512)

	backups, err := d.ListBackups("example.com")
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 2 {
		t.Errorf("got %d backups, want 2", len(backups))
	}
}

func TestGetBackup(t *testing.T) {
	d := openTestDB(t)
	id, _ := d.CreateBackup("example.com", "s3", "/backups/a.zip", 4096)

	b, err := d.GetBackup(id)
	if err != nil {
		t.Fatalf("GetBackup: %v", err)
	}
	if b.SiteDomain != "example.com" {
		t.Errorf("got domain %q, want example.com", b.SiteDomain)
	}
	if b.StorageName != "s3" {
		t.Errorf("got storage %q, want s3", b.StorageName)
	}
	if b.SizeBytes != 4096 {
		t.Errorf("got size %d, want 4096", b.SizeBytes)
	}
}

func TestDeleteBackup(t *testing.T) {
	d := openTestDB(t)
	id, _ := d.CreateBackup("example.com", "local", "/backups/a.zip", 1024)
	err := d.DeleteBackup(id)
	if err != nil {
		t.Fatalf("DeleteBackup: %v", err)
	}
	_, err = d.GetBackup(id)
	if err == nil {
		t.Fatal("expected error after deletion")
	}
}

func TestDeleteOldestBackups(t *testing.T) {
	d := openTestDB(t)
	d.CreateBackup("example.com", "local", "/backups/1.zip", 100)
	d.CreateBackup("example.com", "local", "/backups/2.zip", 200)
	d.CreateBackup("example.com", "local", "/backups/3.zip", 300)
	d.CreateBackup("example.com", "local", "/backups/4.zip", 400)
	d.CreateBackup("example.com", "local", "/backups/5.zip", 500)

	deleted, err := d.DeleteOldestBackups("example.com", "local", 3)
	if err != nil {
		t.Fatalf("DeleteOldestBackups: %v", err)
	}
	if len(deleted) != 2 {
		t.Errorf("got %d deleted, want 2", len(deleted))
	}

	remaining, _ := d.ListBackups("example.com")
	if len(remaining) != 3 {
		t.Errorf("got %d remaining, want 3", len(remaining))
	}
}
