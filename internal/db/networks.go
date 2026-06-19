package db

import "fmt"

// SharedNetwork is a named bridge that several sites are deliberately attached
// to, so they can reach each other privately while staying isolated from
// everyone else.
type SharedNetwork struct {
	Name    string   `json:"name"`
	Owner   string   `json:"owner"`
	Members []string `json:"members"`
}

// CreateSharedNetwork records a new shared network owned by owner.
func (d *DB) CreateSharedNetwork(name, owner string) error {
	_, err := d.conn.Exec(
		`INSERT INTO shared_networks (name, owner) VALUES (?, ?)`, name, owner)
	if err != nil {
		return fmt.Errorf("create shared network: %w", err)
	}
	return nil
}

// GetSharedNetwork returns one network with its members, or ok=false.
func (d *DB) GetSharedNetwork(name string) (sn SharedNetwork, ok bool, err error) {
	row := d.conn.QueryRow(`SELECT name, owner FROM shared_networks WHERE name = ?`, name)
	if scanErr := row.Scan(&sn.Name, &sn.Owner); scanErr != nil {
		if scanErr.Error() == "sql: no rows in result set" {
			return SharedNetwork{}, false, nil
		}
		return SharedNetwork{}, false, scanErr
	}
	sn.Members, err = d.ListNetworkMembers(name)
	return sn, true, err
}

// ListSharedNetworks returns all shared networks (optionally filtered to an
// owner) with their members. An empty owner returns all.
func (d *DB) ListSharedNetworks(owner string) ([]SharedNetwork, error) {
	query := `SELECT name, owner FROM shared_networks ORDER BY name`
	args := []any{}
	if owner != "" {
		query = `SELECT name, owner FROM shared_networks WHERE owner = ? ORDER BY name`
		args = append(args, owner)
	}
	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SharedNetwork
	for rows.Next() {
		var sn SharedNetwork
		if err := rows.Scan(&sn.Name, &sn.Owner); err != nil {
			return nil, err
		}
		out = append(out, sn)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Members, err = d.ListNetworkMembers(out[i].Name); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// DeleteSharedNetwork removes a network and all its memberships.
func (d *DB) DeleteSharedNetwork(name string) error {
	if _, err := d.conn.Exec(`DELETE FROM shared_network_members WHERE network = ?`, name); err != nil {
		return err
	}
	_, err := d.conn.Exec(`DELETE FROM shared_networks WHERE name = ?`, name)
	return err
}

// AddNetworkMember attaches a site to a shared network (idempotent).
func (d *DB) AddNetworkMember(network, siteDomain string) error {
	_, err := d.conn.Exec(
		`INSERT INTO shared_network_members (network, site_domain) VALUES (?, ?)
		 ON CONFLICT(network, site_domain) DO NOTHING`, network, siteDomain)
	return err
}

// RemoveNetworkMember detaches a site from a shared network.
func (d *DB) RemoveNetworkMember(network, siteDomain string) error {
	_, err := d.conn.Exec(
		`DELETE FROM shared_network_members WHERE network = ? AND site_domain = ?`,
		network, siteDomain)
	return err
}

// ListNetworkMembers returns the site domains attached to a network.
func (d *DB) ListNetworkMembers(network string) ([]string, error) {
	rows, err := d.conn.Query(
		`SELECT site_domain FROM shared_network_members WHERE network = ? ORDER BY site_domain`, network)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ListSiteNetworks returns the names of shared networks a site belongs to.
func (d *DB) ListSiteNetworks(siteDomain string) ([]string, error) {
	rows, err := d.conn.Query(
		`SELECT network FROM shared_network_members WHERE site_domain = ? ORDER BY network`, siteDomain)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// RemoveSiteFromAllNetworks drops a site from every shared network (on destroy).
func (d *DB) RemoveSiteFromAllNetworks(siteDomain string) error {
	_, err := d.conn.Exec(`DELETE FROM shared_network_members WHERE site_domain = ?`, siteDomain)
	return err
}
