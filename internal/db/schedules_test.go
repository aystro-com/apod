package db

import "testing"

func TestCreateSchedule(t *testing.T) {
	d := openTestDB(t)
	id, err := d.CreateSchedule("example.com", "0 0 * * *", "local", 7)
	if err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}
	if id == 0 {
		t.Error("expected schedule ID")
	}
}

func TestListSchedules(t *testing.T) {
	d := openTestDB(t)
	d.CreateSchedule("example.com", "0 0 * * *", "local", 7)
	d.CreateSchedule("example.com", "0 0 * * 0", "s3", 4)

	schedules, err := d.ListSchedules("example.com")
	if err != nil {
		t.Fatalf("ListSchedules: %v", err)
	}
	if len(schedules) != 2 {
		t.Errorf("got %d schedules, want 2", len(schedules))
	}
}

func TestDeleteSchedule(t *testing.T) {
	d := openTestDB(t)
	id, _ := d.CreateSchedule("example.com", "0 0 * * *", "local", 7)
	err := d.DeleteSchedule(id)
	if err != nil {
		t.Fatalf("DeleteSchedule: %v", err)
	}
	schedules, _ := d.ListSchedules("example.com")
	if len(schedules) != 0 {
		t.Errorf("got %d schedules, want 0", len(schedules))
	}
}

func TestListAllSchedules(t *testing.T) {
	d := openTestDB(t)
	d.CreateSchedule("a.com", "0 0 * * *", "local", 7)
	d.CreateSchedule("b.com", "0 0 * * 0", "s3", 4)

	schedules, err := d.ListAllSchedules()
	if err != nil {
		t.Fatalf("ListAllSchedules: %v", err)
	}
	if len(schedules) != 2 {
		t.Errorf("got %d schedules, want 2", len(schedules))
	}
}
