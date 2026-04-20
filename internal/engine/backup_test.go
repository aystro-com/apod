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
		{"7d", "0 0 * * 0"},
		{"30d", "0 0 1 * *"},
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
		dbType  string
		wantCmd string
	}{
		{"mysql", "mysqldump"},
		{"postgres", "pg_dump"},
		{"mongo", "mongodump"},
	}
	for _, tt := range tests {
		cmd := dbDumpCommand(tt.dbType, "mydb", "myuser", "mypass")
		if len(cmd) == 0 {
			t.Errorf("dbDumpCommand(%q) returned empty", tt.dbType)
			continue
		}
		if cmd[0] != tt.wantCmd {
			t.Errorf("dbDumpCommand(%q)[0] = %q, want %q", tt.dbType, cmd[0], tt.wantCmd)
		}
	}
}

func TestDbRestoreCommand(t *testing.T) {
	tests := []struct {
		dbType  string
		wantCmd string
	}{
		{"mysql", "mysql"},
		{"postgres", "psql"},
		{"mongo", "mongorestore"},
	}
	for _, tt := range tests {
		cmd := dbRestoreCommand(tt.dbType, "mydb", "myuser", "mypass", "/tmp/dump.sql")
		if len(cmd) == 0 {
			t.Errorf("dbRestoreCommand(%q) returned empty", tt.dbType)
			continue
		}
		if cmd[0] != tt.wantCmd {
			t.Errorf("dbRestoreCommand(%q)[0] = %q, want %q", tt.dbType, cmd[0], tt.wantCmd)
		}
	}
}
