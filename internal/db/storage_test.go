package db

import "testing"

func TestCreateStorageConfig(t *testing.T) {
	d := openTestDB(t)
	err := d.CreateStorageConfig("my-s3", "s3", `{"bucket":"backups","region":"us-east-1"}`)
	if err != nil {
		t.Fatalf("CreateStorageConfig: %v", err)
	}
}

func TestCreateStorageConfigDuplicate(t *testing.T) {
	d := openTestDB(t)
	d.CreateStorageConfig("my-s3", "s3", `{}`)
	err := d.CreateStorageConfig("my-s3", "s3", `{}`)
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
}

func TestGetStorageConfig(t *testing.T) {
	d := openTestDB(t)
	d.CreateStorageConfig("my-s3", "s3", `{"bucket":"backups"}`)
	sc, err := d.GetStorageConfig("my-s3")
	if err != nil {
		t.Fatalf("GetStorageConfig: %v", err)
	}
	if sc.Driver != "s3" {
		t.Errorf("got driver %q, want s3", sc.Driver)
	}
	if sc.Config != `{"bucket":"backups"}` {
		t.Errorf("got config %q", sc.Config)
	}
}

func TestListStorageConfigs(t *testing.T) {
	d := openTestDB(t)
	d.CreateStorageConfig("my-s3", "s3", `{}`)
	d.CreateStorageConfig("my-r2", "r2", `{}`)
	configs, err := d.ListStorageConfigs()
	if err != nil {
		t.Fatalf("ListStorageConfigs: %v", err)
	}
	if len(configs) != 2 {
		t.Errorf("got %d configs, want 2", len(configs))
	}
}

func TestDeleteStorageConfig(t *testing.T) {
	d := openTestDB(t)
	d.CreateStorageConfig("my-s3", "s3", `{}`)
	err := d.DeleteStorageConfig("my-s3")
	if err != nil {
		t.Fatalf("DeleteStorageConfig: %v", err)
	}
	_, err = d.GetStorageConfig("my-s3")
	if err == nil {
		t.Fatal("expected error after deletion")
	}
}
