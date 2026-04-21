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

// composeProjectName returns the docker compose project name for a site
func composeProjectName(domain string) string {
	return "apod-" + strings.ReplaceAll(domain, ".", "-")
}

// composeDir returns the directory where compose files live for a site
func (e *Engine) composeDir(owner, domain string) string {
	_, dataRoot := e.SiteDir(owner, domain)
	return filepath.Join(dataRoot, "compose")
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

	cloneTarget := compDir
	if comp.Path != "" {
		// Clone to a temp location, then move the subdirectory
		tmpDir := compDir + "-tmp"
		os.RemoveAll(tmpDir)
		cmd := exec.CommandContext(ctx, "git", "clone", "--branch", branch, "--single-branch", "--depth", "1", comp.Repo, tmpDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("clone compose repo: %s: %w", string(output), err)
		}
		// Move the subdirectory to the compose dir
		os.RemoveAll(compDir)
		if err := os.Rename(filepath.Join(tmpDir, comp.Path), compDir); err != nil {
			return fmt.Errorf("move compose subdir: %w", err)
		}
		os.RemoveAll(tmpDir)
	} else {
		os.RemoveAll(compDir)
		cmd := exec.CommandContext(ctx, "git", "clone", "--branch", branch, "--single-branch", "--depth", "1", comp.Repo, cloneTarget)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("clone compose repo: %s: %w", string(output), err)
		}
	}

	// Generate .env: start from .env.example if it exists, then override with driver vars
	envPath := filepath.Join(compDir, ".env")
	envExamplePath := filepath.Join(compDir, ".env.example")

	envMap := make(map[string]string)
	envOrder := []string{}

	// Read .env.example as base
	if data, err := os.ReadFile(envExamplePath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if idx := strings.IndexByte(line, '='); idx > 0 {
				key := line[:idx]
				val := line[idx+1:]
				envMap[key] = val
				envOrder = append(envOrder, key)
			}
		}
	}

	// Override with driver-specified env vars
	for envKey, varRef := range comp.Env {
		value := expandVariables(varRef, vars)
		if _, exists := envMap[envKey]; !exists {
			envOrder = append(envOrder, envKey)
		}
		envMap[envKey] = value
	}

	// Write .env preserving order
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

	// Remove hardcoded container_name entries so Docker Compose uses <project>-<service>-1 naming.
	// This allows multiple instances of the same compose stack on one server.
	// Service-to-service communication uses service names via compose DNS (unaffected).
	project := composeProjectName(opts.Domain)
	composeFile := filepath.Join(compDir, "docker-compose.yml")
	if data, err := os.ReadFile(composeFile); err == nil {
		lines := strings.Split(string(data), "\n")
		var cleaned []string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "container_name:") {
				continue // remove hardcoded container names
			}
			cleaned = append(cleaned, line)
		}
		os.WriteFile(composeFile, []byte(strings.Join(cleaned, "\n")), 0644)
	}

	// Start compose
	cmd := exec.CommandContext(ctx, "docker", "compose", "-p", project, "-f", composeFile, "up", "-d")
	cmd.Dir = compDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker compose up: %s: %w", string(output), err)
	}

	// Connect Traefik to the compose network so it can route traffic
	composeNetwork := project + "_default"
	e.docker.ConnectNetwork(ctx, composeNetwork, "apod-traefik")

	// Add Traefik routing config for the proxy service
	if comp.ProxyService != "" && comp.ProxyPort != "" {
		// Docker compose names containers as <project>-<service>-1
		containerName := project + "-" + comp.ProxyService + "-1"
		routerName := strings.ReplaceAll(opts.Domain, ".", "-")
		labels := TraefikLabels(opts.Domain, []string{opts.Domain}, comp.ProxyPort, "")

		// Apply labels by stopping, removing, and recreating with labels
		// Actually, we can't add labels to running containers.
		// Instead, use Traefik's file-based config or just add labels via docker compose labels
		// For now, use the compose-native approach: modify docker-compose.yml to add Traefik labels

		// Simpler: use Traefik file provider
		traefikConfig := fmt.Sprintf(`[http.routers.%s]
  rule = "Host(\x60%s\x60)"
  service = "%s"
  entrypoints = ["websecure"]
  [http.routers.%s.tls]
    certResolver = "letsencrypt"

[http.services.%s.loadBalancer]
  [[http.services.%s.loadBalancer.servers]]
    url = "http://%s:%s"
`, routerName, opts.Domain, routerName, routerName, routerName, routerName, containerName, comp.ProxyPort)

		traefikDir := "/etc/apod/traefik/dynamic"
		os.MkdirAll(traefikDir, 0755)
		configPath := filepath.Join(traefikDir, opts.Domain+".toml")
		if err := os.WriteFile(configPath, []byte(traefikConfig), 0644); err != nil {
			return fmt.Errorf("write traefik config: %w", err)
		}

		_ = labels // not used with file provider
	}

	return nil
}

// StopComposeSite stops a compose-based site
func (e *Engine) StopComposeSite(ctx context.Context, domain, owner string) error {
	compDir := e.composeDir(owner, domain)
	project := composeProjectName(domain)
	cmd := exec.CommandContext(ctx, "docker", "compose", "-p", project, "-f", filepath.Join(compDir, "docker-compose.yml"), "stop")
	cmd.Dir = compDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker compose stop: %s: %w", string(output), err)
	}
	return nil
}

// StartComposeSite starts a compose-based site
func (e *Engine) StartComposeSite(ctx context.Context, domain, owner string) error {
	compDir := e.composeDir(owner, domain)
	project := composeProjectName(domain)
	cmd := exec.CommandContext(ctx, "docker", "compose", "-p", project, "-f", filepath.Join(compDir, "docker-compose.yml"), "start")
	cmd.Dir = compDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker compose start: %s: %w", string(output), err)
	}
	return nil
}

// DestroyComposeSite destroys a compose-based site
func (e *Engine) DestroyComposeSite(ctx context.Context, domain, owner string) error {
	compDir := e.composeDir(owner, domain)
	project := composeProjectName(domain)

	cmd := exec.CommandContext(ctx, "docker", "compose", "-p", project, "-f", filepath.Join(compDir, "docker-compose.yml"), "down", "-v", "--remove-orphans")
	cmd.Dir = compDir
	cmd.CombinedOutput() // best effort

	// Remove Traefik config
	os.Remove(filepath.Join("/etc/apod/traefik/dynamic", domain+".toml"))

	return nil
}

// ExecInComposeSite runs a command in a compose service
func (e *Engine) ExecInComposeSite(ctx context.Context, domain, owner, service string, cmd []string) (string, error) {
	compDir := e.composeDir(owner, domain)
	project := composeProjectName(domain)

	args := []string{"compose", "-p", project, "-f", filepath.Join(compDir, "docker-compose.yml"), "exec", "-T", service}
	args = append(args, cmd...)

	command := exec.CommandContext(ctx, "docker", args...)
	command.Dir = compDir
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("compose exec: %s: %w", string(output), err)
	}
	return string(output), nil
}
