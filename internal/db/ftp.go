package db

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

type FTPAccount struct {
	ID         int64     `json:"id"`
	SiteDomain string    `json:"site_domain"`
	Username   string    `json:"username"`
	CreatedAt  time.Time `json:"created_at"`
}

func hashPassword(password string) string {
	h := sha256.Sum256([]byte(password))
	return hex.EncodeToString(h[:])
}

func (d *DB) CreateFTPAccount(siteDomain, username, password string) error {
	hash := hashPassword(password)
	_, err := d.conn.Exec(`INSERT INTO ftp_accounts (site_domain, username, password_hash) VALUES (?, ?, ?)`, siteDomain, username, hash)
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
	return accounts, nil
}

func (d *DB) DeleteFTPAccount(username string) error {
	_, err := d.conn.Exec(`DELETE FROM ftp_accounts WHERE username = ?`, username)
	return err
}
