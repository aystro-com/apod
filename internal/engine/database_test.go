package engine

import "testing"

func TestDbExportCommand(t *testing.T) {
	cmd := dbDumpCommand("mysql", "testdb", "testuser")
	if cmd == nil || cmd[0] != "sh" {
		t.Errorf("expected sh -c wrapper for mysql, got %v", cmd)
	}

	cmd = dbDumpCommand("postgres", "testdb", "testuser")
	if cmd == nil || cmd[0] != "pg_dumpall" {
		t.Errorf("expected pg_dumpall, got %v", cmd)
	}
}
