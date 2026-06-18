package engine

import "testing"

func TestDbExportCommand(t *testing.T) {
	cmd := dbDumpCmd("mysql", "testdb", "testuser", siteCreds)
	if cmd == nil || cmd[0] != "sh" {
		t.Errorf("expected sh -c wrapper for mysql, got %v", cmd)
	}

	cmd = dbDumpCmd("postgres", "testdb", "testuser", siteCreds)
	if cmd == nil || cmd[0] != "pg_dumpall" {
		t.Errorf("expected pg_dumpall, got %v", cmd)
	}
}
