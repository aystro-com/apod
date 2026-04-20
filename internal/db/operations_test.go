package db

import "testing"

func TestLogOperation(t *testing.T) {
	d := openTestDB(t)
	err := d.LogOperation("example.com", "create", "driver=static", "success")
	if err != nil {
		t.Fatalf("LogOperation: %v", err)
	}
}

func TestListOperations(t *testing.T) {
	d := openTestDB(t)
	d.LogOperation("example.com", "create", "", "success")
	d.LogOperation("example.com", "deploy", "branch=main", "success")
	d.LogOperation("other.com", "create", "", "success")

	ops, err := d.ListOperations("example.com", 10)
	if err != nil {
		t.Fatalf("ListOperations: %v", err)
	}
	if len(ops) != 2 {
		t.Errorf("got %d, want 2", len(ops))
	}
}

func TestListAllOperations(t *testing.T) {
	d := openTestDB(t)
	d.LogOperation("a.com", "create", "", "success")
	d.LogOperation("b.com", "create", "", "success")

	ops, err := d.ListAllOperations(10)
	if err != nil {
		t.Fatalf("ListAllOperations: %v", err)
	}
	if len(ops) != 2 {
		t.Errorf("got %d, want 2", len(ops))
	}
}
