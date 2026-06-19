package engine

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// memQuantityRe matches a memory/disk quantity: an integer followed by M or G
// (the only forms parseMemoryMB understands — decimals and "GB" silently parse
// to 0, which would leave a container unbounded or broken on restart).
var memQuantityRe = regexp.MustCompile(`^[0-9]+[MG]$`)

// validateConfigValue rejects malformed values before they reach the DB, so a
// typo ("2GB", "abc", "-1") can't be stored and break the site on next restart.
// Applied to API, CLI, and UI alike. Unknown keys pass through unchanged.
func validateConfigValue(key, value string) error {
	v := strings.TrimSpace(value)
	switch key {
	case "ram", "storage":
		if !memQuantityRe.MatchString(strings.ToUpper(v)) {
			return Invalid("%s must be an integer followed by M or G (e.g. 512M, 2G)", key)
		}
	case "cpu":
		n, err := strconv.ParseFloat(v, 64)
		if err != nil || n <= 0 || n > 256 {
			return Invalid("cpu must be a positive number of cores (e.g. 0.5, 1, 2)")
		}
	case "repo":
		return ValidateRepo(v)
	case "branch":
		return ValidateBranch(v)
	}
	return nil
}

func (e *Engine) SetConfig(ctx context.Context, domain string, key, value string) error {
	if err := validateConfigValue(key, value); err != nil {
		return err
	}

	if err := e.locks.Acquire(domain, "updating config"); err != nil {
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
