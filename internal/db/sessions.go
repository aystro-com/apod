package db

import (
	"database/sql"
	"fmt"
	"time"
)

// CreateSession stores a login session. Only the SHA-256 hash of the token is
// persisted — the raw token exists client-side only.
func (d *DB) CreateSession(tokenHash, userName string, expiresAt time.Time) error {
	_, err := d.conn.Exec(
		`INSERT INTO sessions (token_hash, user_name, expires_at) VALUES (?, ?, ?)`,
		tokenHash, userName, expiresAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

// GetSessionUser returns the user name for a non-expired session token hash,
// or "" if the session does not exist or has expired.
func (d *DB) GetSessionUser(tokenHash string) (string, error) {
	var name string
	err := d.conn.QueryRow(
		`SELECT user_name FROM sessions WHERE token_hash = ? AND expires_at > ?`,
		tokenHash, time.Now().UTC(),
	).Scan(&name)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("query session: %w", err)
	}
	return name, nil
}

func (d *DB) DeleteSession(tokenHash string) error {
	if _, err := d.conn.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// DeleteSessionsForUser revokes every session of a user (password change,
// API key reset, user deletion).
func (d *DB) DeleteSessionsForUser(userName string) error {
	if _, err := d.conn.Exec(`DELETE FROM sessions WHERE user_name = ?`, userName); err != nil {
		return fmt.Errorf("delete user sessions: %w", err)
	}
	return nil
}

func (d *DB) DeleteExpiredSessions() error {
	if _, err := d.conn.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, time.Now().UTC()); err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}
	return nil
}
