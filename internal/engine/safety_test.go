package engine

import (
	"strings"
	"testing"
)

func TestLockManager(t *testing.T) {
	lm := NewLockManager()

	if err := lm.Acquire("example.com", "deploying"); err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	// The held lock reports what it's busy with.
	if info := lm.Info("example.com"); !info.Held || info.Operation != "deploying" {
		t.Fatalf("Info() = %+v, want held=true operation=deploying", info)
	}

	err := lm.Acquire("example.com", "restarting")
	if err == nil {
		t.Fatal("expected error on double acquire")
	}
	// The conflict message names the operation already in progress.
	if !strings.Contains(err.Error(), "deploying") {
		t.Errorf("conflict error %q should mention the holding operation", err)
	}

	lm.Release("example.com")

	if info := lm.Info("example.com"); info.Held {
		t.Fatalf("Info() after release = %+v, want held=false", info)
	}

	if err := lm.Acquire("example.com", "deploying"); err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
}

func TestParseMemoryMB(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"256M", 256},
		{"512M", 512},
		{"1G", 1024},
		{"2G", 2048},
		{"128", 128},
	}
	for _, tt := range tests {
		got := parseMemoryMB(tt.input)
		if got != tt.want {
			t.Errorf("parseMemoryMB(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
