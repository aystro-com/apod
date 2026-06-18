package db

import "testing"

func TestProcessScaling(t *testing.T) {
	d := newTestDB(t)

	// Unset => not found.
	if _, ok, err := d.GetProcessReplicas("a.com", "queue"); err != nil || ok {
		t.Fatalf("expected no override, got ok=%v err=%v", ok, err)
	}

	// Set and read back.
	if err := d.SetProcessReplicas("a.com", "queue", 3); err != nil {
		t.Fatalf("SetProcessReplicas: %v", err)
	}
	n, ok, err := d.GetProcessReplicas("a.com", "queue")
	if err != nil || !ok || n != 3 {
		t.Fatalf("GetProcessReplicas = (%d,%v,%v), want (3,true,nil)", n, ok, err)
	}

	// Upsert replaces, including scale-to-zero.
	if err := d.SetProcessReplicas("a.com", "queue", 0); err != nil {
		t.Fatalf("SetProcessReplicas 0: %v", err)
	}
	n, ok, _ = d.GetProcessReplicas("a.com", "queue")
	if !ok || n != 0 {
		t.Fatalf("expected override 0, got (%d,%v)", n, ok)
	}

	// Scoped per site.
	if err := d.SetProcessReplicas("b.com", "queue", 5); err != nil {
		t.Fatalf("SetProcessReplicas b: %v", err)
	}
	list, err := d.ListProcessScaling("a.com")
	if err != nil {
		t.Fatalf("ListProcessScaling: %v", err)
	}
	if len(list) != 1 || list["queue"] != 0 {
		t.Fatalf("site a scaling map wrong: %+v", list)
	}

	// Delete-all (used on site destroy).
	if err := d.DeleteProcessScaling("a.com"); err != nil {
		t.Fatalf("DeleteProcessScaling: %v", err)
	}
	list, _ = d.ListProcessScaling("a.com")
	if len(list) != 0 {
		t.Fatalf("expected cleared scaling for a.com, got %+v", list)
	}
	// b.com untouched.
	if bl, _ := d.ListProcessScaling("b.com"); len(bl) != 1 {
		t.Fatalf("deleting a.com must not affect b.com")
	}
}
