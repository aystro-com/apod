package engine

import (
	"context"
	"fmt"
	"strings"
)

func buildTraefikRule(domains []string) string {
	var hostRules []string
	for _, d := range domains {
		hostRules = append(hostRules, fmt.Sprintf("Host(`%s`)", d))
	}
	return strings.Join(hostRules, " || ")
}

func (e *Engine) AddDomain(ctx context.Context, siteDomain, newDomain string) error {
	// newDomain is rendered into Traefik Host(`...`) rules and must never be
	// able to break out of the backtick (rule injection / routing hijack).
	if err := ValidateDomain(newDomain); err != nil {
		return err
	}

	if err := e.locks.Acquire(siteDomain); err != nil {
		return err
	}
	defer e.locks.Release(siteDomain)

	site, err := e.db.GetSite(siteDomain)
	if err != nil {
		return fmt.Errorf("get site: %w", err)
	}
	if site == nil {
		return NotFound("site %q not found", siteDomain)
	}

	// Reject a domain already used by any site (its primary domain or an alias)
	// with a clear conflict instead of an opaque UNIQUE-constraint 500.
	if existing, _ := e.db.GetSiteByDomain(newDomain); existing != nil {
		return Conflict("domain %q is already in use by site %q", newDomain, existing.Domain)
	}

	if err := e.db.AddDomain(site.ID, newDomain, false); err != nil {
		return fmt.Errorf("add domain: %w", err)
	}

	return nil
}

func (e *Engine) RemoveDomain(ctx context.Context, siteDomain, removeDomain string) error {
	if err := e.locks.Acquire(siteDomain); err != nil {
		return err
	}
	defer e.locks.Release(siteDomain)

	if removeDomain == siteDomain {
		return fmt.Errorf("cannot remove primary domain %q", siteDomain)
	}

	site, err := e.db.GetSite(siteDomain)
	if err != nil {
		return fmt.Errorf("get site: %w", err)
	}
	// Scope the delete to this site so a caller can't remove another tenant's
	// alias by name (IDOR).
	if err := e.db.RemoveDomainForSite(site.ID, removeDomain); err != nil {
		return fmt.Errorf("remove domain: %w", err)
	}

	return nil
}

func (e *Engine) ListDomains(ctx context.Context, siteDomain string) ([]string, error) {
	site, err := e.db.GetSite(siteDomain)
	if err != nil {
		return nil, fmt.Errorf("get site: %w", err)
	}

	return e.db.ListDomains(site.ID)
}
