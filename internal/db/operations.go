package db

import (
	"fmt"
	"time"
)

type Operation struct {
	ID         int64     `json:"id"`
	SiteDomain string    `json:"site_domain"`
	Action     string    `json:"action"`
	Details    string    `json:"details"`
	Result     string    `json:"result"`
	CreatedAt  time.Time `json:"created_at"`
}

func (d *DB) LogOperation(siteDomain, action, details, result string) error {
	_, err := d.conn.Exec(
		`INSERT INTO operations (site_domain, action, details, result) VALUES (?, ?, ?, ?)`,
		siteDomain, action, details, result,
	)
	if err != nil {
		return fmt.Errorf("log operation: %w", err)
	}
	return nil
}

func (d *DB) ListOperations(siteDomain string, limit int) ([]Operation, error) {
	rows, err := d.conn.Query(
		`SELECT id, site_domain, action, details, result, created_at FROM operations WHERE site_domain = ? ORDER BY created_at DESC LIMIT ?`, siteDomain, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ops []Operation
	for rows.Next() {
		var op Operation
		if err := rows.Scan(&op.ID, &op.SiteDomain, &op.Action, &op.Details, &op.Result, &op.CreatedAt); err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
	return ops, nil
}

func (d *DB) ListAllOperations(limit int) ([]Operation, error) {
	rows, err := d.conn.Query(
		`SELECT id, site_domain, action, details, result, created_at FROM operations ORDER BY created_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ops []Operation
	for rows.Next() {
		var op Operation
		if err := rows.Scan(&op.ID, &op.SiteDomain, &op.Action, &op.Details, &op.Result, &op.CreatedAt); err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
	return ops, nil
}
