package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// SiteCredentials holds user-facing credentials for a site
type SiteCredentials struct {
	Domain   string            `json:"domain"`
	Driver   string            `json:"driver"`
	URL      string            `json:"url"`
	Secrets  map[string]string `json:"secrets,omitempty"`
}

// GetSiteCredentials returns the user-facing credentials for a site.
// For compose sites, reads key values from the .env file.
// For normal sites, returns the DB credentials.
func (e *Engine) GetSiteCredentials(ctx context.Context, domain string) (*SiteCredentials, error) {
	site, err := e.db.GetSite(domain)
	if err != nil {
		return nil, err
	}

	driver, _ := e.drivers.Load(site.Driver)
	creds := &SiteCredentials{
		Domain:  domain,
		Driver:  site.Driver,
		URL:     "https://" + domain,
		Secrets: make(map[string]string),
	}

	if driver != nil && driver.Type == "compose" {
		// Read secrets from compose .env
		compDir := e.composeDir(site.Owner, domain)
		envPath := filepath.Join(compDir, ".env")
		if data, err := os.ReadFile(envPath); err == nil {
			envMap := parseEnvFile(string(data))

			// Expose relevant credentials
			expose := []string{
				"DASHBOARD_USERNAME", "DASHBOARD_PASSWORD",
				"ANON_KEY", "SERVICE_ROLE_KEY",
				"POSTGRES_PASSWORD", "JWT_SECRET",
			}
			for _, key := range expose {
				if val, ok := envMap[key]; ok && val != "" {
					creds.Secrets[key] = val
				}
			}
		}
	} else {
		// Normal site — show DB name/user (password is in container env)
		dbName := strings.ReplaceAll(domain, ".", "_")
		creds.Secrets["DB_NAME"] = dbName
		creds.Secrets["DB_USER"] = dbName
		creds.Secrets["DB_HOST"] = "apod-" + domain + "-db"
	}

	return creds, nil
}

func parseEnvFile(content string) map[string]string {
	m := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.IndexByte(line, '='); idx > 0 {
			m[line[:idx]] = line[idx+1:]
		}
	}
	return m
}
