package db

import (
	"database/sql"
	"fmt"

	"github.com/aystro/apod/internal/models"
)

func (d *DB) CreateUser(name, apiKeyHash, role string, uid int) error {
	_, err := d.conn.Exec(
		`INSERT INTO users (name, uid, role, api_key_hash) VALUES (?, ?, ?, ?)`,
		name, uid, role, apiKeyHash,
	)
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

func (d *DB) GetUserByName(name string) (*models.User, error) {
	u := &models.User{}
	err := d.conn.QueryRow(
		`SELECT id, name, uid, role, created_at FROM users WHERE name = ?`, name,
	).Scan(&u.ID, &u.Name, &u.UID, &u.Role, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user %q not found", name)
	}
	if err != nil {
		return nil, fmt.Errorf("query user: %w", err)
	}
	return u, nil
}

func (d *DB) GetUserByAPIKeyHash(hash string) (*models.User, error) {
	u := &models.User{}
	err := d.conn.QueryRow(
		`SELECT id, name, uid, role, created_at FROM users WHERE api_key_hash = ?`, hash,
	).Scan(&u.ID, &u.Name, &u.UID, &u.Role, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query user by key: %w", err)
	}
	return u, nil
}

func (d *DB) ListUsers() ([]models.User, error) {
	rows, err := d.conn.Query(
		`SELECT id, name, uid, role, created_at FROM users ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("query users: %w", err)
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Name, &u.UID, &u.Role, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	return users, nil
}

func (d *DB) DeleteUser(name string) error {
	result, err := d.conn.Exec(`DELETE FROM users WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("user %q not found", name)
	}
	return nil
}

func (d *DB) UpdateUserAPIKeyHash(name, newHash string) error {
	result, err := d.conn.Exec(
		`UPDATE users SET api_key_hash = ? WHERE name = ?`, newHash, name,
	)
	if err != nil {
		return fmt.Errorf("update api key: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("user %q not found", name)
	}
	return nil
}

func (d *DB) CountSitesByOwner(owner string) (int, error) {
	var count int
	err := d.conn.QueryRow(`SELECT COUNT(*) FROM sites WHERE owner = ?`, owner).Scan(&count)
	return count, err
}
