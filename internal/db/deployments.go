package db

import (
	"database/sql"
	"fmt"
	"time"
)

type Deployment struct {
	ID            int64     `json:"id"`
	SiteDomain    string    `json:"site_domain"`
	CommitHash    string    `json:"commit_hash"`
	Branch        string    `json:"branch"`
	Status        string    `json:"status"`
	PreviousImage string    `json:"previous_image"`
	CreatedAt     time.Time `json:"created_at"`
}

func (d *DB) CreateDeployment(siteDomain, commitHash, branch string) (int64, error) {
	result, err := d.conn.Exec(
		`INSERT INTO deployments (site_domain, commit_hash, branch) VALUES (?, ?, ?)`,
		siteDomain, commitHash, branch,
	)
	if err != nil {
		return 0, fmt.Errorf("create deployment: %w", err)
	}
	return result.LastInsertId()
}

func (d *DB) GetDeployment(id int64) (*Deployment, error) {
	dep := &Deployment{}
	err := d.conn.QueryRow(
		`SELECT id, site_domain, commit_hash, branch, status, previous_image, created_at FROM deployments WHERE id = ?`, id,
	).Scan(&dep.ID, &dep.SiteDomain, &dep.CommitHash, &dep.Branch, &dep.Status, &dep.PreviousImage, &dep.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("deployment %d not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("query deployment: %w", err)
	}
	return dep, nil
}

func (d *DB) UpdateDeploymentStatus(id int64, status string) error {
	_, err := d.conn.Exec(`UPDATE deployments SET status = ? WHERE id = ?`, status, id)
	return err
}

func (d *DB) SetDeploymentPreviousImage(id int64, image string) error {
	_, err := d.conn.Exec(`UPDATE deployments SET previous_image = ? WHERE id = ?`, image, id)
	return err
}

func (d *DB) ListDeployments(siteDomain string) ([]Deployment, error) {
	rows, err := d.conn.Query(
		`SELECT id, site_domain, commit_hash, branch, status, previous_image, created_at FROM deployments WHERE site_domain = ? ORDER BY created_at DESC`, siteDomain,
	)
	if err != nil {
		return nil, fmt.Errorf("query deployments: %w", err)
	}
	defer rows.Close()
	var deps []Deployment
	for rows.Next() {
		var dep Deployment
		if err := rows.Scan(&dep.ID, &dep.SiteDomain, &dep.CommitHash, &dep.Branch, &dep.Status, &dep.PreviousImage, &dep.CreatedAt); err != nil {
			return nil, err
		}
		deps = append(deps, dep)
	}
	return deps, nil
}

func (d *DB) GetLatestDeployment(siteDomain string) (*Deployment, error) {
	dep := &Deployment{}
	err := d.conn.QueryRow(
		`SELECT id, site_domain, commit_hash, branch, status, previous_image, created_at FROM deployments WHERE site_domain = ? ORDER BY id DESC LIMIT 1`, siteDomain,
	).Scan(&dep.ID, &dep.SiteDomain, &dep.CommitHash, &dep.Branch, &dep.Status, &dep.PreviousImage, &dep.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no deployments for %q", siteDomain)
	}
	if err != nil {
		return nil, err
	}
	return dep, nil
}
