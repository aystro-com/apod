package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

func parseEnvJSON(s string) (map[string]string, error) {
	var envs map[string]string
	if err := json.Unmarshal([]byte(s), &envs); err != nil {
		return nil, fmt.Errorf("parse env JSON: %w", err)
	}
	return envs, nil
}

func envToJSON(envs map[string]string) (string, error) {
	data, err := json.Marshal(envs)
	if err != nil {
		return "", fmt.Errorf("marshal env: %w", err)
	}
	return string(data), nil
}

func envToSlice(envs map[string]string) []string {
	var slice []string
	for k, v := range envs {
		slice = append(slice, k+"="+v)
	}
	return slice
}

func mergeEnv(existingJSON, key, value string) (string, error) {
	envs, err := parseEnvJSON(existingJSON)
	if err != nil {
		return "", err
	}
	envs[key] = value
	return envToJSON(envs)
}

func removeEnv(existingJSON, key string) (string, error) {
	envs, err := parseEnvJSON(existingJSON)
	if err != nil {
		return "", err
	}
	delete(envs, key)
	return envToJSON(envs)
}

// envKeyRe is the POSIX-ish shape of a valid environment variable name.
var envKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (e *Engine) SetEnv(ctx context.Context, domain, key, value string) error {
	// Validate the key shape and forbid CR/LF in key or value: env values are
	// written verbatim into compose .env files (KEY=VALUE per line), so a
	// newline would let a caller inject extra .env lines (e.g. override another
	// service's secret) for compose-based sites.
	if !envKeyRe.MatchString(key) {
		return Invalid("invalid environment variable name %q", key)
	}
	if strings.ContainsAny(key, "\r\n") || strings.ContainsAny(value, "\r\n") {
		return Invalid("environment values must not contain newlines")
	}

	if err := e.locks.Acquire(domain); err != nil {
		return err
	}
	defer e.locks.Release(domain)

	site, err := e.db.GetSite(domain)
	if err != nil {
		return err
	}

	newEnv, err := mergeEnv(site.Env, key, value)
	if err != nil {
		return err
	}

	return e.db.UpdateSiteConfig(domain, map[string]string{"env": newEnv})
}

func (e *Engine) UnsetEnv(ctx context.Context, domain, key string) error {
	if err := e.locks.Acquire(domain); err != nil {
		return err
	}
	defer e.locks.Release(domain)

	site, err := e.db.GetSite(domain)
	if err != nil {
		return err
	}

	newEnv, err := removeEnv(site.Env, key)
	if err != nil {
		return err
	}

	return e.db.UpdateSiteConfig(domain, map[string]string{"env": newEnv})
}

func (e *Engine) ListEnv(ctx context.Context, domain string) (map[string]string, error) {
	site, err := e.db.GetSite(domain)
	if err != nil {
		return nil, err
	}

	return parseEnvJSON(site.Env)
}
