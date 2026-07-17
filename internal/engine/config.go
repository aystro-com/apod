package engine

import (
	"context"
	"fmt"
	"log"
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
	// The env column is writable through UpdateSiteConfig, so a config write of
	// key="env" would otherwise bypass the key/no-newline rules SetEnv enforces.
	// Validate the whole map here so it can't inject extra compose .env lines.
	if key == "env" {
		envs, err := parseEnvJSON(value)
		if err != nil {
			return Invalid("invalid env JSON")
		}
		if err := validateEnvMap(envs); err != nil {
			return err
		}
	}

	if err := e.locks.Acquire(domain, "updating config"); err != nil {
		return err
	}
	defer e.locks.Release(domain)

	fields := map[string]string{key: value}
	if err := e.db.UpdateSiteConfig(domain, fields); err != nil {
		return fmt.Errorf("update config: %w", err)
	}

	// Apply resource changes to the live site so they actually take effect —
	// previously the new value was persisted but nothing enforced it (not even on
	// restart, which only start/stops existing containers), so the user's change
	// silently did nothing. CPU/memory update in place via `docker update`; a
	// storage change re-applies the owner's disk quota. Best-effort: the value is
	// already saved, so a failure here is logged, not fatal.
	switch key {
	case "ram", "cpu":
		if site, err := e.db.GetSite(domain); err == nil && site != nil {
			memMB := int64(parseMemoryMB(site.RAM))
			cpus, _ := strconv.ParseFloat(strings.TrimSpace(site.CPU), 64)
			ids, _ := e.docker.ListContainersByLabel(ctx, labelPrefix+"site", domain)
			for _, id := range ids {
				if err := e.docker.UpdateContainerResources(ctx, id, memMB, cpus); err != nil {
					log.Printf("apply %s=%s to %s: %v", key, value, domain, err)
				}
			}
		}
	case "storage":
		if site, err := e.db.GetSite(domain); err == nil && site != nil {
			if err := e.ApplyDiskQuota(ctx, site.Owner); err != nil {
				log.Printf("apply disk quota for %s: %v", site.Owner, err)
			}
		}
	}
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
