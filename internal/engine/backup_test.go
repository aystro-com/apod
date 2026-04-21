package engine

import "testing"

func TestDurationToCron(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1h", "0 * * * *"},
		{"6h", "0 */6 * * *"},
		{"12h", "0 */12 * * *"},
		{"24h", "0 0 * * *"},
		{"daily", "0 0 * * *"},
		{"7d", "0 0 * * 0"},
		{"weekly", "0 0 * * 0"},
		{"30d", "0 0 1 * *"},
		{"monthly", "0 0 1 * *"},
	}
	for _, tt := range tests {
		got, err := durationToCron(tt.input)
		if err != nil {
			t.Errorf("durationToCron(%q): %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("durationToCron(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDurationToCronInvalid(t *testing.T) {
	_, err := durationToCron("5m")
	if err == nil {
		t.Fatal("expected error for unsupported duration")
	}
}

func TestDbDumpCommand(t *testing.T) {
	tests := []struct {
		dbType string
		notNil bool
	}{
		{"mysql", true},
		{"postgres", true},
		{"mongo", true},
		{"unknown", false},
	}
	for _, tt := range tests {
		cmd := dbDumpCommand(tt.dbType, "mydb", "myuser")
		if tt.notNil && len(cmd) == 0 {
			t.Errorf("dbDumpCommand(%q) returned empty", tt.dbType)
		}
		if !tt.notNil && cmd != nil {
			t.Errorf("dbDumpCommand(%q) should return nil", tt.dbType)
		}
	}
}

func TestDbRestoreCommand(t *testing.T) {
	tests := []struct {
		dbType string
		notNil bool
	}{
		{"mysql", true},
		{"postgres", true},
		{"mongo", true},
		{"unknown", false},
	}
	for _, tt := range tests {
		cmd := dbRestoreCommand(tt.dbType, "mydb", "myuser", "/tmp/dump.sql")
		if tt.notNil && len(cmd) == 0 {
			t.Errorf("dbRestoreCommand(%q) returned empty", tt.dbType)
		}
		if !tt.notNil && cmd != nil {
			t.Errorf("dbRestoreCommand(%q) should return nil", tt.dbType)
		}
	}
}
