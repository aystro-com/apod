package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn *sql.DB
}

func DefaultPath() string {
	return filepath.Join("/etc/apod", "apod.db")
}

func Open(path string) (*DB, error) {
	dir := filepath.Dir(path)
	// 0700: the DB holds password/token hashes, TOTP secrets, recovery codes
	// and (until encrypted) storage-provider credentials — keep it out of
	// reach of other local users.
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	// _busy_timeout: wait (ms) for a lock instead of failing instantly with
	// SQLITE_BUSY when another writer holds it. With WAL and the default
	// multi-connection pool, concurrent writers on separate connections would
	// otherwise return "database is locked" the moment they race.
	conn, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// SQLite allows only one writer at a time; serialize on a single connection
	// so concurrent API requests queue instead of colliding on SQLITE_BUSY. WAL
	// still lets reads proceed, and single-node throughput is not the bottleneck
	// for a control plane.
	conn.SetMaxOpenConns(1)

	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	// Restrict the DB file (and WAL/SHM side files) to the owner.
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		if _, statErr := os.Stat(p); statErr == nil {
			os.Chmod(p, 0600)
		}
	}

	d := &DB{conn: conn}
	if err := d.migrate(); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return d, nil
}

func (d *DB) Close() error {
	return d.conn.Close()
}

func (d *DB) Conn() *sql.DB {
	return d.conn
}
