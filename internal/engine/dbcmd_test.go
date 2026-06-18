package engine

import (
	"strings"
	"testing"
)

func joinArgv(a []string) string { return strings.Join(a, " ") }

func TestDBDumpCmd(t *testing.T) {
	// Per-site mysql dump uses the site user + password env, single database.
	got := joinArgv(dbDumpCmd("mysql", "site_db", "site_user", siteCreds))
	if !strings.Contains(got, "mysqldump") || !strings.Contains(got, "-usite_user") || !strings.Contains(got, "site_db") {
		t.Errorf("mysql site dump = %q", got)
	}
	// Super mysql dump uses root + all-databases.
	got = joinArgv(dbDumpCmd("mysql", "", "", superCreds))
	if !strings.Contains(got, "--all-databases") || !strings.Contains(got, "-u root") {
		t.Errorf("mysql super dump = %q", got)
	}
	// Postgres per-site is pg_dumpall with the site user, as a bare argv.
	pg := dbDumpCmd("postgres", "site_db", "site_user", siteCreds)
	if pg[0] != "pg_dumpall" || pg[len(pg)-1] != "site_user" {
		t.Errorf("postgres site dump = %v", pg)
	}
	if dbDumpCmd("unknown", "", "", siteCreds) != nil {
		t.Error("unknown engine should return nil")
	}
}

func TestDBRestoreCmd(t *testing.T) {
	// mysql restore must always use --binary-mode (this drift caused real bugs)
	// and read the dump from STDIN — never embed it (argv length limit).
	for _, mode := range []dbCredMode{siteCreds, superCreds} {
		got := joinArgv(dbRestoreCmd("mysql", "db", "user", mode))
		if !strings.Contains(got, "--binary-mode=1") {
			t.Errorf("mysql restore (mode %d) missing --binary-mode: %q", mode, got)
		}
		if strings.Contains(got, "base64") || strings.Contains(got, "<") {
			t.Errorf("restore must read stdin, not embed/redirect a file: %q", got)
		}
	}
	// Postgres per-site targets the named database.
	got := joinArgv(dbRestoreCmd("postgres", "site_db", "site_user", siteCreds))
	if !strings.Contains(got, "-U site_user") || !strings.Contains(got, "-d site_db") {
		t.Errorf("postgres site restore = %q", got)
	}
}

func TestDBProbeCmd(t *testing.T) {
	if !strings.Contains(joinArgv(dbProbeCmd("mysql", "db", "user")), "SELECT 1") {
		t.Error("mysql probe should SELECT 1")
	}
	if !strings.Contains(joinArgv(dbProbeCmd("postgres", "db", "user")), "SELECT 1") {
		t.Error("postgres probe should SELECT 1")
	}
	if dbProbeCmd("mongo", "", "") != nil {
		t.Error("mongo has no probe")
	}
}
