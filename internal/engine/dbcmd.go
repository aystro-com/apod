package engine

import "fmt"

// dbCredMode selects which credentials a database command runs with.
type dbCredMode int

const (
	// siteCreds uses the per-site database user and its password env var. Used
	// for native (non-compose) sites where each service has dedicated creds.
	siteCreds dbCredMode = iota
	// superCreds uses the engine's superuser (root / POSTGRES_USER) — used for
	// compose sites, whose service credentials are managed by compose itself.
	superCreds
)

// This file is the single source of truth for how apod dumps, restores and
// probes databases. Every backup, restore, clone, export and import path goes
// through here, so a fix (e.g. mysql --binary-mode, capturing stdout only)
// applies everywhere at once instead of drifting across copies.

// dbDumpCmd returns the command that writes a logical dump of the database to
// stdout. Callers must capture stdout only (ExecCaptureStdout).
func dbDumpCmd(dbType, dbName, dbUser string, mode dbCredMode) []string {
	switch dbType {
	case "mysql", "mariadb":
		if mode == superCreds {
			return shc(`mysqldump --no-tablespaces --all-databases -u root -p"$MYSQL_ROOT_PASSWORD"`)
		}
		return shc(fmt.Sprintf(`mysqldump --no-tablespaces -u%s -p"$MYSQL_PASSWORD" %s`, dbUser, dbName))
	case "postgres":
		if mode == superCreds {
			return shc(`pg_dumpall -U "${POSTGRES_USER:-postgres}"`)
		}
		return []string{"pg_dumpall", "-U", dbUser}
	case "mongo":
		if mode == superCreds || dbName == "" {
			return []string{"mongodump", "--archive"}
		}
		return []string{"mongodump", "--archive", "--db", dbName}
	}
	return nil
}

// dbRestoreCmd returns the command that restores a logical dump read from STDIN.
// The caller streams the dump to the command's stdin (see restoreDatabase) so
// there is no shell argv length limit. mysql always runs with --binary-mode so
// dumps containing NUL bytes load cleanly.
func dbRestoreCmd(dbType, dbName, dbUser string, mode dbCredMode) []string {
	switch dbType {
	case "mysql", "mariadb":
		if mode == superCreds {
			return shc(`mysql --binary-mode=1 -u root -p"$MYSQL_ROOT_PASSWORD"`)
		}
		return shc(fmt.Sprintf(`mysql --binary-mode=1 -u%s -p"$MYSQL_PASSWORD" %s`, dbUser, dbName))
	case "postgres":
		if mode == superCreds {
			return shc(`psql -U "${POSTGRES_USER:-postgres}"`)
		}
		return shc(fmt.Sprintf(`psql -U %s -d %s`, dbUser, dbName))
	case "mongo":
		return []string{"mongorestore", "--archive", "--drop"}
	}
	return nil
}

// dbProbeCmd returns a cheap readiness check used to wait for a freshly-created
// database container to accept connections before a restore. Returns nil for
// engines without a probe.
func dbProbeCmd(dbType, dbName, dbUser string) []string {
	switch dbType {
	case "mysql", "mariadb":
		return shc(fmt.Sprintf(`mysql -u%s -p"$MYSQL_PASSWORD" -e 'SELECT 1' %s`, dbUser, dbName))
	case "postgres":
		return shc(fmt.Sprintf(`psql -U %s -d %s -c 'SELECT 1'`, dbUser, dbName))
	}
	return nil
}

// shc wraps a shell command line as an argv for `sh -c`.
func shc(cmd string) []string {
	return []string{"sh", "-c", cmd}
}
