package db

import (
	"database/sql"
	"fmt"
	"time"
)

// APIToken is a scoped personal access token. Only the hash is stored.
type APIToken struct {
	ID        int64      `json:"id"`
	UserName  string     `json:"user_name"`
	Name      string     `json:"name"`
	Abilities string     `json:"abilities"` // comma-separated: read,write,deploy
	Sensitive bool       `json:"sensitive"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func (d *DB) CreateAPIToken(userName, name, tokenHash, abilities string, sensitive bool, expiresAt *time.Time) error {
	_, err := d.conn.Exec(
		`INSERT INTO api_tokens (user_name, name, token_hash, abilities, sensitive, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		userName, name, tokenHash, abilities, sensitive, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("insert api token: %w", err)
	}
	return nil
}

// GetAPITokenByHash returns a non-expired token by its hash, or nil.
func (d *DB) GetAPITokenByHash(tokenHash string) (*APIToken, error) {
	tok := &APIToken{}
	var expires sql.NullTime
	err := d.conn.QueryRow(
		`SELECT id, user_name, name, abilities, sensitive, expires_at, created_at
		 FROM api_tokens WHERE token_hash = ?`, tokenHash,
	).Scan(&tok.ID, &tok.UserName, &tok.Name, &tok.Abilities, &tok.Sensitive, &expires, &tok.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query api token: %w", err)
	}
	if expires.Valid {
		if time.Now().After(expires.Time) {
			return nil, nil
		}
		tok.ExpiresAt = &expires.Time
	}
	return tok, nil
}

func (d *DB) ListAPITokens(userName string) ([]APIToken, error) {
	rows, err := d.conn.Query(
		`SELECT id, user_name, name, abilities, sensitive, expires_at, created_at
		 FROM api_tokens WHERE user_name = ? ORDER BY created_at DESC`, userName,
	)
	if err != nil {
		return nil, fmt.Errorf("list api tokens: %w", err)
	}
	defer rows.Close()

	var tokens []APIToken
	for rows.Next() {
		var tok APIToken
		var expires sql.NullTime
		if err := rows.Scan(&tok.ID, &tok.UserName, &tok.Name, &tok.Abilities, &tok.Sensitive, &expires, &tok.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan api token: %w", err)
		}
		if expires.Valid {
			tok.ExpiresAt = &expires.Time
		}
		tokens = append(tokens, tok)
	}
	return tokens, nil
}

// DeleteAPIToken removes a token, scoped to its owner so users can't revoke
// each other's tokens.
func (d *DB) DeleteAPIToken(userName string, id int64) error {
	res, err := d.conn.Exec(
		`DELETE FROM api_tokens WHERE id = ? AND user_name = ?`, id, userName,
	)
	if err != nil {
		return fmt.Errorf("delete api token: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("token not found")
	}
	return nil
}

func (d *DB) DeleteAPITokensForUser(userName string) error {
	if _, err := d.conn.Exec(`DELETE FROM api_tokens WHERE user_name = ?`, userName); err != nil {
		return fmt.Errorf("delete user api tokens: %w", err)
	}
	return nil
}

// --- TOTP / recovery codes ---

func (d *DB) GetUserTOTP(name string) (secret string, enabled bool, err error) {
	err = d.conn.QueryRow(
		`SELECT totp_secret, totp_enabled FROM users WHERE name = ?`, name,
	).Scan(&secret, &enabled)
	if err == sql.ErrNoRows {
		return "", false, fmt.Errorf("user %q not found", name)
	}
	if err != nil {
		return "", false, fmt.Errorf("query totp: %w", err)
	}
	return secret, enabled, nil
}

func (d *DB) SetUserTOTPSecret(name, secret string) error {
	return d.updateUser(name, `UPDATE users SET totp_secret = ? WHERE name = ?`, secret, name)
}

func (d *DB) SetUserTOTPEnabled(name string, enabled bool) error {
	return d.updateUser(name, `UPDATE users SET totp_enabled = ? WHERE name = ?`, enabled, name)
}

// ClearUserTOTP disables 2FA and wipes the secret, recovery codes, and replay
// guard in a single statement.
func (d *DB) ClearUserTOTP(name string) error {
	return d.updateUser(name,
		`UPDATE users SET totp_secret = '', totp_enabled = 0, totp_last_step = 0, recovery_codes = '[]' WHERE name = ?`,
		name)
}

func (d *DB) GetUserTOTPLastStep(name string) (uint64, error) {
	var step uint64
	err := d.conn.QueryRow(`SELECT totp_last_step FROM users WHERE name = ?`, name).Scan(&step)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("user %q not found", name)
	}
	return step, err
}

func (d *DB) SetUserTOTPLastStep(name string, step uint64) error {
	return d.updateUser(name, `UPDATE users SET totp_last_step = ? WHERE name = ?`, step, name)
}

func (d *DB) GetUserRecoveryCodes(name string) (string, error) {
	var codes string
	err := d.conn.QueryRow(`SELECT recovery_codes FROM users WHERE name = ?`, name).Scan(&codes)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("user %q not found", name)
	}
	return codes, err
}

func (d *DB) SetUserRecoveryCodes(name, codesJSON string) error {
	return d.updateUser(name, `UPDATE users SET recovery_codes = ? WHERE name = ?`, codesJSON, name)
}

func (d *DB) CountUsers() (int, error) {
	var n int
	err := d.conn.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// updateUser runs an UPDATE that ends in "WHERE name = ?" and errors if no
// row matched.
func (d *DB) updateUser(name, query string, args ...interface{}) error {
	res, err := d.conn.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("user %q not found", name)
	}
	return nil
}
