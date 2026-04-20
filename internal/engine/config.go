package engine

import (
	"context"
	"fmt"
	"strings"
)

func parseConfigFlag(s string) (string, string, error) {
	parts := strings.SplitN(s, "=", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid config format: %q (expected key=value)", s)
	}
	return parts[0], parts[1], nil
}

func (e *Engine) SetConfig(ctx context.Context, domain string, key, value string) error {
	if err := e.locks.Acquire(domain); err != nil {
		return err
	}
	defer e.locks.Release(domain)

	fields := map[string]string{key: value}
	if err := e.db.UpdateSiteConfig(domain, fields); err != nil {
		return fmt.Errorf("update config: %w", err)
	}

	// For resource changes, containers need recreation — this will be wired up
	// when the full container recreation logic is added
	return nil
}

func (e *Engine) GetConfig(ctx context.Context, domain string) (map[string]string, error) {
	site, err := e.db.GetSite(domain)
	if err != nil {
		return nil, err
	}

	config := map[string]string{
		"domain": site.Domain,
		"driver": site.Driver,
		"status": site.Status,
		"ram":    site.RAM,
		"cpu":    site.CPU,
		"env":    site.Env,
		"repo":   site.Repo,
		"branch": site.Branch,
	}
	return config, nil
}
