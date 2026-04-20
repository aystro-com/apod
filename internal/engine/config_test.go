package engine

import "testing"

func TestParseConfigFlag(t *testing.T) {
	tests := []struct {
		input string
		key   string
		value string
	}{
		{"ram=512M", "ram", "512M"},
		{"cpu=2", "cpu", "2"},
		{"env=APP_DEBUG=false", "env", "APP_DEBUG=false"},
	}

	for _, tt := range tests {
		key, value, err := parseConfigFlag(tt.input)
		if err != nil {
			t.Errorf("parseConfigFlag(%q): %v", tt.input, err)
			continue
		}
		if key != tt.key {
			t.Errorf("key = %q, want %q", key, tt.key)
		}
		if value != tt.value {
			t.Errorf("value = %q, want %q", value, tt.value)
		}
	}
}

func TestParseConfigFlagInvalid(t *testing.T) {
	_, _, err := parseConfigFlag("invalid")
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
}
