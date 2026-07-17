package db

import (
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type FTPAccount struct {
	ID         int64     `json:"id"`
	SiteDomain string    `json:"site_domain"`
	Username   string    `json:"username"`
	CreatedAt  time.Time `json:"created_at"`
}

func hashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

func (d *DB) CreateFTPAccount(siteDomain, username, password string) error {
	hash, err := hashPassword(password)
	if err != nil {
		return fmt.Errorf("hash FTP password: %w", err)
	}
	_, err = d.conn.Exec(`INSERT INTO ftp_accounts (site_domain, username, password_hash) VALUES (?, ?, ?)`, siteDomain, username, hash)
	if err != nil { return fmt.Errorf("create FTP account: %w", err) }
	return nil
}

func (d *DB) ListFTPAccounts(siteDomain string) ([]FTPAccount, error) {
	rows, err := d.conn.Query(`SELECT id, site_domain, username, created_at FROM ftp_accounts WHERE site_domain = ? ORDER BY username`, siteDomain)
	if err != nil { return nil, err }
	defer rows.Close()
	var accounts []FTPAccount
	for rows.Next() {
		var a FTPAccount
		if err := rows.Scan(&a.ID, &a.SiteDomain, &a.Username, &a.CreatedAt); err != nil { return nil, err }
		accounts = append(accounts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}
	return accounts, nil
}

// DeleteFTPAccountForSite deletes an FTP account only if it belongs to
// siteDomain (IDOR-safe).
func (d *DB) DeleteFTPAccountForSite(siteDomain, username string) error {
	result, err := d.conn.Exec(`DELETE FROM ftp_accounts WHERE site_domain = ? AND username = ?`, siteDomain, username)
	if err != nil {
		return fmt.Errorf("delete FTP account: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("FTP account %q not found", username)
	}
	return nil
}
