package engine

import "testing"

func TestDbExportCommand(t *testing.T) {
	// Verify dump commands are generated correctly for each type
	cmd := dbDumpCommand("mysql", "testdb", "testuser", "testpass")
	if cmd[0] != "mysqldump" {
		t.Errorf("expected mysqldump, got %s", cmd[0])
	}

	cmd = dbDumpCommand("postgres", "testdb", "testuser", "testpass")
	if cmd[0] != "pg_dumpall" {
		t.Errorf("expected pg_dumpall, got %s", cmd[0])
	}
}
