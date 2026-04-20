package engine

import "testing"

func TestLockManager(t *testing.T) {
	lm := NewLockManager()

	if err := lm.Acquire("example.com"); err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	if err := lm.Acquire("example.com"); err == nil {
		t.Fatal("expected error on double acquire")
	}

	lm.Release("example.com")

	if err := lm.Acquire("example.com"); err != nil {
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
