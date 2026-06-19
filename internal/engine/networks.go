package engine

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/aystro/apod/internal/db"
)

// sharedNetworkPattern restricts shared-network names to safe, DNS-ish tokens
// (they become a docker network name).
var sharedNetworkPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// sharedNetworkName is the docker bridge name backing a shared network.
func sharedNetworkName(name string) string {
	return "apod-net-" + name
}

// NetworkNeighbor is one container a site can reach over a shared network —
// rendered as a neighbor in the architecture view.
type NetworkNeighbor struct {
	Network string `json:"network"`
	Site    string `json:"site"`
	Service string `json:"service"`
	Name    string `json:"name"`
	IP      string `json:"ip"` // address on the shared network
	Running bool   `json:"running"`
}

// CreateSharedNetwork creates a new shared network owned by owner and its
// backing docker bridge.
func (e *Engine) CreateSharedNetwork(ctx context.Context, name, owner string) error {
	if !sharedNetworkPattern.MatchString(name) {
		return Invalid("invalid network name %q: use letters, digits, _ or -", name)
	}
	if _, ok, _ := e.db.GetSharedNetwork(name); ok {
		return Conflict("network %q already exists", name)
	}
	if err := e.db.CreateSharedNetwork(name, owner); err != nil {
		return err
	}
	if err := e.docker.EnsureNetwork(ctx, sharedNetworkName(name)); err != nil {
		// Roll back the record so a failed bridge doesn't leave a row that
		// blocks re-creation.
		e.db.DeleteSharedNetwork(name)
		return fmt.Errorf("create network bridge: %w", err)
	}
	return nil
}

// ListSharedNetworks returns shared networks (all, or filtered to an owner).
func (e *Engine) ListSharedNetworks(ctx context.Context, owner string) ([]db.SharedNetwork, error) {
	return e.db.ListSharedNetworks(owner)
}

// GetSharedNetwork returns one shared network (with members), or ok=false.
func (e *Engine) GetSharedNetwork(ctx context.Context, name string) (db.SharedNetwork, bool, error) {
	return e.db.GetSharedNetwork(name)
}

// DeleteSharedNetwork disconnects every member and removes the network.
func (e *Engine) DeleteSharedNetwork(ctx context.Context, name string) error {
	sn, ok, err := e.db.GetSharedNetwork(name)
	if err != nil {
		return err
	}
	if !ok {
		return NotFound("network %q not found", name)
	}
	netName := sharedNetworkName(name)
	for _, domain := range sn.Members {
		e.disconnectSiteFromNetwork(ctx, netName, domain)
	}
	if err := e.db.DeleteSharedNetwork(name); err != nil {
		return err
	}
	e.docker.RemoveNetwork(ctx, netName) // best effort
	return nil
}

// AddSiteToNetwork attaches a site's containers to a shared network. The site
// and the network must share an owner — this is the invariant that keeps a
// shared network from bridging two different tenants (even by admin mistake).
func (e *Engine) AddSiteToNetwork(ctx context.Context, name, domain string) error {
	sn, ok, err := e.db.GetSharedNetwork(name)
	if err != nil {
		return err
	}
	if !ok {
		return NotFound("network %q not found", name)
	}
	site, err := e.db.GetSite(domain)
	if err != nil || site == nil {
		return NotFound("site %q not found", domain)
	}
	if site.Owner != sn.Owner {
		return Forbidden("site %q and network %q have different owners; a shared network cannot bridge tenants", domain, name)
	}
	if err := e.db.AddNetworkMember(name, domain); err != nil {
		return err
	}
	e.connectSiteToNetwork(ctx, sharedNetworkName(name), domain)
	e.LogActivity(domain, "network_join", name, "success")
	return nil
}

// RemoveSiteFromNetwork detaches a site from a shared network.
func (e *Engine) RemoveSiteFromNetwork(ctx context.Context, name, domain string) error {
	if err := e.db.RemoveNetworkMember(name, domain); err != nil {
		return err
	}
	e.disconnectSiteFromNetwork(ctx, sharedNetworkName(name), domain)
	e.LogActivity(domain, "network_leave", name, "success")
	return nil
}

// leaveAllSharedNetworks removes a site from every shared network it belongs to,
// both in the DB and by disconnecting its live containers. Used when a site
// changes owner: shared-network membership is only authorized at join time
// (same-owner), so a transferred site must be detached or it stays bridged into
// the previous owner's private network — a cross-tenant isolation breach.
func (e *Engine) leaveAllSharedNetworks(ctx context.Context, domain string) {
	nets, err := e.db.ListSiteNetworks(domain)
	if err != nil {
		return
	}
	for _, n := range nets {
		e.disconnectSiteFromNetwork(ctx, sharedNetworkName(n), domain)
	}
	if err := e.db.RemoveSiteFromAllNetworks(domain); err != nil {
		log.Printf("shared net: clear membership for %s: %v", domain, err)
	}
}

// reconnectSharedNetworks re-attaches a site's containers to every shared
// network it belongs to. Called after any operation that (re)creates the site's
// containers, so a deploy/restart/scale never silently drops a link.
func (e *Engine) reconnectSharedNetworks(ctx context.Context, domain string) {
	nets, err := e.db.ListSiteNetworks(domain)
	if err != nil {
		return
	}
	for _, n := range nets {
		e.connectSiteToNetwork(ctx, sharedNetworkName(n), domain)
	}
}

func (e *Engine) connectSiteToNetwork(ctx context.Context, netName, domain string) {
	if err := e.docker.EnsureNetwork(ctx, netName); err != nil {
		log.Printf("shared net: ensure %s: %v", netName, err)
		return
	}
	// Only running containers can be attached — a stopped container has no
	// network sandbox yet. Stopped ones join automatically when they start, via
	// the reconnect hooks on StartSite/scale/update.
	containers, _ := e.docker.ListSiteContainers(ctx, domain)
	for _, c := range containers {
		if c.Name == "" || !c.Running {
			continue
		}
		if err := e.docker.ConnectNetwork(ctx, netName, c.Name); err != nil && !isAlreadyConnected(err) {
			log.Printf("shared net: connect %s -> %s: %v", c.Name, netName, err)
		}
	}
}

func (e *Engine) disconnectSiteFromNetwork(ctx context.Context, netName, domain string) {
	ids, _ := e.docker.ListContainersByLabel(ctx, labelPrefix+"site", domain)
	for _, id := range ids {
		if err := e.docker.DisconnectNetwork(ctx, netName, id); err != nil && !isNotConnected(err) {
			// A failed disconnect leaves a removed site bridged — surface it.
			log.Printf("shared net: disconnect %s from %s: %v", id, netName, err)
		}
	}
}

// isAlreadyConnected reports whether a connect error just means the container is
// already on the network (harmless, idempotent).
func isAlreadyConnected(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "already exists in network") ||
		strings.Contains(s, "already connected") ||
		strings.Contains(s, "endpoint with name") ||
		// Container stopped between listing and connecting — it has no network
		// sandbox; it'll be attached when it starts (reconnect hooks). Benign.
		strings.Contains(s, "network sandbox") ||
		strings.Contains(s, "is not running")
}

// isNotConnected reports whether a disconnect error just means the container was
// not on the network (already detached / gone — harmless).
func isNotConnected(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "is not connected") ||
		strings.Contains(s, "not connected to network") ||
		strings.Contains(s, "no such container") ||
		strings.Contains(s, "no such network")
}

// SiteNetworkView returns, for each shared network a site belongs to, the OTHER
// members' containers and their addresses on that shared network — the data the
// architecture view renders as reachable neighbors.
func (e *Engine) SiteNetworkView(ctx context.Context, domain string) ([]NetworkNeighbor, error) {
	site, err := e.db.GetSite(domain)
	if err != nil || site == nil {
		return nil, NotFound("site %q not found", domain)
	}
	nets, err := e.db.ListSiteNetworks(domain)
	if err != nil {
		return nil, err
	}
	var out []NetworkNeighbor
	for _, n := range nets {
		netName := sharedNetworkName(n)
		members, _ := e.db.ListNetworkMembers(n)
		for _, m := range members {
			if m == domain {
				continue
			}
			// Only ever reveal a neighbor's container names/IPs to a co-owned
			// site — never leak one tenant's internals to another, even if a
			// cross-owner membership somehow exists.
			if ms, _ := e.db.GetSite(m); ms == nil || ms.Owner != site.Owner {
				continue
			}
			containers, _ := e.docker.ListSiteContainers(ctx, m)
			for _, c := range containers {
				if c.Name == "" {
					continue
				}
				out = append(out, NetworkNeighbor{
					Network: n,
					Site:    m,
					Service: c.Service,
					Name:    c.Name,
					IP:      e.docker.ContainerIPOnNetwork(ctx, c.Name, netName),
					Running: c.Running,
				})
			}
		}
	}
	return out, nil
}
