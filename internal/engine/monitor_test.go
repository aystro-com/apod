package engine

import "testing"

func TestSiteStatsStruct(t *testing.T) {
	stats := SiteStats{
		Domain:      "example.com",
		Status:      "running",
		CPUPercent:  5.2,
		MemoryMB:    128.5,
		MemoryLimit: 512,
	}
	if stats.Domain != "example.com" {
		t.Error("wrong domain")
	}
	if stats.CPUPercent != 5.2 {
		t.Error("wrong cpu")
	}
}
