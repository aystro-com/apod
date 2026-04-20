package db

import (
	"fmt"
	"time"
)

type SSHKey struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	PublicKey string    `json:"public_key"`
	CreatedAt time.Time `json:"created_at"`
}

func (d *DB) AddSSHKey(name, publicKey string) error {
	_, err := d.conn.Exec(`INSERT INTO ssh_keys (name, public_key) VALUES (?, ?)`, name, publicKey)
	if err != nil { return fmt.Errorf("add SSH key: %w", err) }
	return nil
}

func (d *DB) ListSSHKeys() ([]SSHKey, error) {
	rows, err := d.conn.Query(`SELECT id, name, public_key, created_at FROM ssh_keys ORDER BY name`)
	if err != nil { return nil, err }
	defer rows.Close()
	var keys []SSHKey
	for rows.Next() {
		var k SSHKey
		if err := rows.Scan(&k.ID, &k.Name, &k.PublicKey, &k.CreatedAt); err != nil { return nil, err }
		keys = append(keys, k)
	}
	return keys, nil
}

func (d *DB) DeleteSSHKey(name string) error {
	_, err := d.conn.Exec(`DELETE FROM ssh_keys WHERE name = ?`, name)
	return err
}
