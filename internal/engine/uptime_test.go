package engine

import "testing"

func TestPing(t *testing.T) {
	uc := &UptimeChecker{}
	// Test with a known-good URL (httpbin or similar may not be available in CI)
	// Just verify the function returns without panic
	isUp, code, ms := uc.ping("http://localhost:1")
	// Should fail since nothing listens on :1
	if isUp {
		t.Error("expected down for unreachable host")
	}
	_ = code
	_ = ms
}
