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
	if err := e.locks.Acquire(siteDomain); err != nil {
		return err
	}
	defer e.locks.Release(siteDomain)

	site, err := e.db.GetSite(siteDomain)
	if err != nil {
		return fmt.Errorf("get site: %w", err)
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

	if err := e.db.RemoveDomain(removeDomain); err != nil {
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
