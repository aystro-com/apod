package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/aystro/apod/internal/models"
)

// SiteCredentials holds user-facing credentials for a site
type SiteCredentials struct {
	Domain  string            `json:"domain"`
	Driver  string            `json:"driver"`
	URL     string            `json:"url"`
	Secrets map[string]string `json:"secrets,omitempty"`
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
		// Native site: surface the generated secrets apod stores (they're not
		// visible anywhere else). DB identifiers when it has a database, the DB
		// password and any per-driver secrets from the encrypted store.
		if driverHasDatabase(driver) {
			dbName := strings.ReplaceAll(domain, ".", "_")
			creds.Secrets["DB_NAME"] = dbName
			creds.Secrets["DB_USER"] = dbName
			creds.Secrets["DB_HOST"] = "apod-" + domain + "-db"
			if pass, ok, _ := e.getSiteSecret(domain, "db_password"); ok && pass != "" {
				creds.Secrets["DB_PASSWORD"] = pass
			}
		}
		for _, name := range generatedSecretNames {
			if v, ok, _ := e.getSiteSecret(domain, name); ok && v != "" {
				creds.Secrets[strings.ToUpper(name)] = v
			}
		}
	}

	// Driver-declared credentials (e.g. "Odoo master password") — expanded with
	// this site's variables and shown with a friendly label.
	if driver != nil && len(driver.Credentials) > 0 {
		vars := e.siteVars(site)
		for _, c := range driver.Credentials {
			if v := expandVariables(c.Value, vars); v != "" && c.Label != "" {
				creds.Secrets[c.Label] = v
			}
		}
	}

	return creds, nil
}

// driverHasDatabase reports whether a driver provisions its own database — i.e.
// it declares a backup database or ships a service named "db". Used to avoid
// surfacing DB credentials for stateless drivers that have none.
func driverHasDatabase(driver *models.Driver) bool {
	if driver == nil {
		return false
	}
	if len(driver.Backup.Databases) > 0 {
		return true
	}
	if _, ok := driver.Services["db"]; ok {
		return true
	}
	return false
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
