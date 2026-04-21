package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aystro/apod/internal/models"
)

// composeProjectName returns the docker compose project name for a site.
// Docker Compose uses this to namespace all resources (containers, networks, volumes).
func composeProjectName(domain string) string {
	return "apod-" + strings.ReplaceAll(domain, ".", "-")
}

// composeDir returns the directory where compose files live for a site
func (e *Engine) composeDir(owner, domain string) string {
	_, dataRoot := e.SiteDir(owner, domain)
	return filepath.Join(dataRoot, "compose")
}

// composeCmd builds an exec.Cmd for docker compose with the right project and file.
func composeCmd(ctx context.Context, project, compDir string, args ...string) *exec.Cmd {
	base := []string{"compose", "-p", project, "-f", filepath.Join(compDir, "docker-compose.yml")}
	cmd := exec.CommandContext(ctx, "docker", append(base, args...)...)
	cmd.Dir = compDir
	return cmd
}

// sanitizeComposeFile makes a docker-compose.yml safe for apod:
//   - Converts container_name: to hostname: (preserves internal hostname, allows multi-instance)
//   - Removes host port bindings (Traefik handles external routing)
//   - Removes the top-level "name:" field (we use -p flag for project naming)
func sanitizeComposeFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	var out []string
	inPorts := false
	portsIndent := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Remove top-level "name:" (compose project name — we set via -p flag)
		if strings.HasPrefix(line, "name:") {
			continue
		}

		// Convert container_name to hostname
		if strings.HasPrefix(trimmed, "container_name:") {
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			name := strings.TrimSpace(strings.TrimPrefix(trimmed, "container_name:"))
			name = strings.Trim(name, "\"'")
			out = append(out, indent+"hostname: "+name)
			continue
		}

		// Remove ports: section (host port bindings conflict with apod/Traefik)
		if strings.HasPrefix(trimmed, "ports:") && !strings.HasPrefix(trimmed, "ports: [") {
			inPorts = true
			portsIndent = len(line) - len(strings.TrimLeft(line, " \t"))
			continue
		}
		if inPorts {
			lineIndent := len(line) - len(strings.TrimLeft(line, " \t"))
			if trimmed == "" || lineIndent > portsIndent || strings.HasPrefix(trimmed, "-") {
				continue // skip port entries
			}
			inPorts = false
		}

		out = append(out, line)
	}

	return os.WriteFile(path, []byte(strings.Join(out, "\n")), 0644)
}

// CreateComposeSite creates a site using docker compose
func (e *Engine) CreateComposeSite(ctx context.Context, opts CreateSiteOpts, driver *models.Driver, vars map[string]string) error {
	comp := driver.Compose
	if comp == nil {
		return fmt.Errorf("driver %q has no compose configuration", opts.Driver)
	}

	compDir := e.composeDir(opts.Owner, opts.Domain)

	// Clone the compose repo
	branch := comp.Branch
	if branch == "" {
		branch = "master"
	}

	if comp.Path != "" {
		tmpDir := compDir + "-tmp"
		os.RemoveAll(tmpDir)
		cmd := exec.CommandContext(ctx, "git", "clone", "--branch", branch, "--single-branch", "--depth", "1", comp.Repo, tmpDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("clone compose repo: %s: %w", string(output), err)
		}
		os.RemoveAll(compDir)
		if err := os.Rename(filepath.Join(tmpDir, comp.Path), compDir); err != nil {
			return fmt.Errorf("move compose subdir: %w", err)
		}
		os.RemoveAll(tmpDir)
	} else {
		os.RemoveAll(compDir)
		cmd := exec.CommandContext(ctx, "git", "clone", "--branch", branch, "--single-branch", "--depth", "1", comp.Repo, compDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("clone compose repo: %s: %w", string(output), err)
		}
	}

	// Generate .env: start from .env.example as base, override with driver vars
	envPath := filepath.Join(compDir, ".env")
	envExamplePath := filepath.Join(compDir, ".env.example")

	envMap := make(map[string]string)
	var envOrder []string

	if data, err := os.ReadFile(envExamplePath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if idx := strings.IndexByte(line, '='); idx > 0 {
				key := line[:idx]
				envMap[key] = line[idx+1:]
				envOrder = append(envOrder, key)
			}
		}
	}

	for envKey, varRef := range comp.Env {
		value := expandVariables(varRef, vars)
		if _, exists := envMap[envKey]; !exists {
			envOrder = append(envOrder, envKey)
		}
		envMap[envKey] = value
	}

	var envContent strings.Builder
	for _, key := range envOrder {
		envContent.WriteString(key + "=" + envMap[key] + "\n")
	}
	if err := os.WriteFile(envPath, []byte(envContent.String()), 0600); err != nil {
		return fmt.Errorf("write compose .env: %w", err)
	}

	// Write any driver files
	for _, f := range driver.Files {
		path := expandVariables(f.Path, vars)
		content := expandVariables(f.Content, vars)
		os.MkdirAll(filepath.Dir(path), 0755)
		perm := os.FileMode(0644)
		if strings.HasSuffix(path, ".sh") {
			perm = 0755
		}
		os.WriteFile(path, []byte(content), perm)
	}

	// Sanitize compose file: convert container_name to hostname for multi-instance support
	composeFile := filepath.Join(compDir, "docker-compose.yml")
	if err := sanitizeComposeFile(composeFile); err != nil {
		return fmt.Errorf("sanitize compose file: %w", err)
	}

	// Start compose
	project := composeProjectName(opts.Domain)
	cmd := composeCmd(ctx, project, compDir, "up", "-d")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker compose up: %s: %w", string(output), err)
	}

	// Connect Traefik to the compose network
	composeNetwork := project + "_default"
	e.docker.ConnectNetwork(ctx, composeNetwork, "apod-traefik")

	// Write Traefik routing config using the compose service name on the compose network
	if comp.ProxyService != "" && comp.ProxyPort != "" {
		routerName := strings.ReplaceAll(opts.Domain, ".", "-")

		// Traefik resolves the service name via the compose network DNS
		traefikConfig := fmt.Sprintf(`[http.routers.%s]
  rule = "Host(`+"`"+`%s`+"`"+`)"
  service = "%s"
  entrypoints = ["websecure"]
  [http.routers.%s.tls]
    certResolver = "letsencrypt"

[http.routers.%s-http]
  rule = "Host(`+"`"+`%s`+"`"+`)"
  service = "%s"
  entrypoints = ["web"]

[http.services.%s.loadBalancer]
  [[http.services.%s.loadBalancer.servers]]
    url = "http://%s:%s"
`, routerName, opts.Domain, routerName, routerName,
			routerName, opts.Domain, routerName,
			routerName, routerName, comp.ProxyService, comp.ProxyPort)

		traefikDir := "/etc/apod/traefik/dynamic"
		os.MkdirAll(traefikDir, 0755)
		if err := os.WriteFile(filepath.Join(traefikDir, opts.Domain+".toml"), []byte(traefikConfig), 0644); err != nil {
			return fmt.Errorf("write traefik config: %w", err)
		}
	}

	return nil
}

// StopComposeSite stops a compose-based site
func (e *Engine) StopComposeSite(ctx context.Context, domain, owner string) error {
	project := composeProjectName(domain)
	cmd := composeCmd(ctx, project, e.composeDir(owner, domain), "stop")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker compose stop: %s: %w", string(output), err)
	}
	return nil
}

// StartComposeSite starts a compose-based site
func (e *Engine) StartComposeSite(ctx context.Context, domain, owner string) error {
	project := composeProjectName(domain)
	cmd := composeCmd(ctx, project, e.composeDir(owner, domain), "start")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker compose start: %s: %w", string(output), err)
	}
	return nil
}

// DestroyComposeSite destroys a compose-based site
func (e *Engine) DestroyComposeSite(ctx context.Context, domain, owner string) error {
	project := composeProjectName(domain)
	cmd := composeCmd(ctx, project, e.composeDir(owner, domain), "down", "-v", "--remove-orphans")
	cmd.CombinedOutput() // best effort

	// Remove Traefik routing config
	os.Remove(filepath.Join("/etc/apod/traefik/dynamic", domain+".toml"))
	return nil
}

// ExecInComposeSite runs a command in a compose service
func (e *Engine) ExecInComposeSite(ctx context.Context, domain, owner, service string, cmdArgs []string) (string, error) {
	project := composeProjectName(domain)
	args := append([]string{"exec", "-T", service}, cmdArgs...)
	cmd := composeCmd(ctx, project, e.composeDir(owner, domain), args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("compose exec: %s: %w", string(output), err)
	}
	return string(output), nil
}
