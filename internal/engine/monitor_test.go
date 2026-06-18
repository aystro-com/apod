package engine

import (
	"testing"
	"time"
)

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

func TestCPUPercentFromDelta(t *testing.T) {
	// One full core busy over the window: container delta == host-per-core delta.
	// total +1e9 of 4e9 host delta on 4 CPUs => 1/4 * 4 * 100 = 100% (one core).
	prev := cpuSample{total: 0, system: 0, onCPU: 4}
	cur := cpuSample{total: 1_000_000_000, system: 4_000_000_000, onCPU: 4}
	if got := cpuPercentFromDelta(prev, cur); got != 100 {
		t.Errorf("one busy core: got %v%%, want 100%%", got)
	}

	// Idle container: no CPU delta => 0%.
	idle := cpuSample{total: 1_000_000_000, system: 8_000_000_000, onCPU: 4}
	if got := cpuPercentFromDelta(cur, idle); got != 0 {
		t.Errorf("idle: got %v%%, want 0%%", got)
	}

	// No forward window (counters reset / first sample): 0%, never negative.
	if got := cpuPercentFromDelta(cur, prev); got != 0 {
		t.Errorf("reset window: got %v%%, want 0%%", got)
	}
}

func TestPruneCPUSamples(t *testing.T) {
	cpuSamplesMu.Lock()
	cpuSamples = map[string]cpuSample{
		"fresh": {seen: time.Now()},
		"stale": {seen: time.Now().Add(-2 * cpuSampleTTL)},
	}
	cpuSamplesMu.Unlock()

	pruneCPUSamples()

	cpuSamplesMu.Lock()
	defer cpuSamplesMu.Unlock()
	if _, ok := cpuSamples["stale"]; ok {
		t.Error("stale sample should have been evicted")
	}
	if _, ok := cpuSamples["fresh"]; !ok {
		t.Error("fresh sample should have been kept")
	}
}
