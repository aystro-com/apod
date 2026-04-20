package db

import "testing"

func TestCreateUptimeCheck(t *testing.T) {
	d := openTestDB(t)
	err := d.CreateUptimeCheck("example.com", "https://example.com", 60, "")
	if err != nil {
		t.Fatalf("CreateUptimeCheck: %v", err)
	}
}

func TestGetUptimeCheck(t *testing.T) {
	d := openTestDB(t)
	d.CreateUptimeCheck("example.com", "https://example.com", 30, "https://hooks.slack.com/xxx")
	uc, err := d.GetUptimeCheck("example.com")
	if err != nil {
		t.Fatalf("GetUptimeCheck: %v", err)
	}
	if uc.URL != "https://example.com" {
		t.Errorf("got URL %q", uc.URL)
	}
	if uc.IntervalSeconds != 30 {
		t.Errorf("got interval %d", uc.IntervalSeconds)
	}
}

func TestLogUptimeResult(t *testing.T) {
	d := openTestDB(t)
	err := d.LogUptimeResult("example.com", 200, 150, true)
	if err != nil {
		t.Fatalf("LogUptimeResult: %v", err)
	}
	err = d.LogUptimeResult("example.com", 500, 2000, false)
	if err != nil {
		t.Fatalf("LogUptimeResult: %v", err)
	}
}

func TestGetUptimeStats(t *testing.T) {
	d := openTestDB(t)
	d.LogUptimeResult("example.com", 200, 100, true)
	d.LogUptimeResult("example.com", 200, 150, true)
	d.LogUptimeResult("example.com", 500, 2000, false)

	stats, err := d.GetUptimeStats("example.com", 24)
	if err != nil {
		t.Fatalf("GetUptimeStats: %v", err)
	}
	if stats.TotalChecks != 3 {
		t.Errorf("got %d checks, want 3", stats.TotalChecks)
	}
	if stats.TotalDowntime != 1 {
		t.Errorf("got %d downtime, want 1", stats.TotalDowntime)
	}
}

func TestGetUptimeLogs(t *testing.T) {
	d := openTestDB(t)
	d.LogUptimeResult("example.com", 200, 100, true)
	d.LogUptimeResult("example.com", 200, 120, true)

	logs, err := d.GetUptimeLogs("example.com", 10)
	if err != nil {
		t.Fatalf("GetUptimeLogs: %v", err)
	}
	if len(logs) != 2 {
		t.Errorf("got %d logs, want 2", len(logs))
	}
}

func TestDeleteUptimeCheck(t *testing.T) {
	d := openTestDB(t)
	d.CreateUptimeCheck("example.com", "https://example.com", 60, "")
	d.DeleteUptimeCheck("example.com")
	_, err := d.GetUptimeCheck("example.com")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}
