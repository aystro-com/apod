package db

import (
	"database/sql"
	"fmt"
	"time"
)

type StorageConfig struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Driver    string    `json:"driver"`
	Config    string    `json:"config"`
	CreatedAt time.Time `json:"created_at"`
}

func (d *DB) CreateStorageConfig(name, driver, config string) error {
	_, err := d.conn.Exec(
		`INSERT INTO storage_configs (name, driver, config) VALUES (?, ?, ?)`,
		name, driver, config,
	)
	if err != nil {
		return fmt.Errorf("create storage config %q: %w", name, err)
	}
	return nil
}

func (d *DB) GetStorageConfig(name string) (*StorageConfig, error) {
	sc := &StorageConfig{}
	err := d.conn.QueryRow(
		`SELECT id, name, driver, config, created_at FROM storage_configs WHERE name = ?`, name,
	).Scan(&sc.ID, &sc.Name, &sc.Driver, &sc.Config, &sc.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("storage config %q not found", name)
	}
	if err != nil {
		return nil, fmt.Errorf("query storage config: %w", err)
	}
	return sc, nil
}

func (d *DB) ListStorageConfigs() ([]StorageConfig, error) {
	rows, err := d.conn.Query(`SELECT id, name, driver, config, created_at FROM storage_configs ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("query storage configs: %w", err)
	}
	defer rows.Close()

	var configs []StorageConfig
	for rows.Next() {
		var sc StorageConfig
		if err := rows.Scan(&sc.ID, &sc.Name, &sc.Driver, &sc.Config, &sc.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan storage config: %w", err)
		}
		configs = append(configs, sc)
	}
	return configs, nil
}

func (d *DB) DeleteStorageConfig(name string) error {
	result, err := d.conn.Exec(`DELETE FROM storage_configs WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("delete storage config: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("storage config %q not found", name)
	}
	return nil
}
