package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aystro/apod/internal/models"
	"gopkg.in/yaml.v3"
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

// composeCmd builds an exec.Cmd for docker compose with the right project and files.
// Includes the override file if it exists (for labels, limits, security).
func composeCmd(ctx context.Context, project, compDir string, args ...string) *exec.Cmd {
	base := []string{"compose", "-p", project, "-f", filepath.Join(compDir, "docker-compose.yml")}
	overridePath := filepath.Join(compDir, "docker-compose.override.yml")
	if _, err := os.Stat(overridePath); err == nil {
		base = append(base, "-f", overridePath)
	}
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

		if strings.HasPrefix(line, "name:") {
			continue
		}

		if strings.HasPrefix(trimmed, "container_name:") {
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			name := strings.TrimSpace(strings.TrimPrefix(trimmed, "container_name:"))
			name = strings.Trim(name, "\"'")
			out = append(out, indent+"hostname: "+name)
			continue
		}

		if strings.HasPrefix(trimmed, "ports:") && !strings.HasPrefix(trimmed, "ports: [") {
			inPorts = true
			portsIndent = len(line) - len(strings.TrimLeft(line, " \t"))
			continue
		}
		if inPorts {
			lineIndent := len(line) - len(strings.TrimLeft(line, " \t"))
			if trimmed == "" || lineIndent > portsIndent || strings.HasPrefix(trimmed, "-") {
				continue
			}
			inPorts = false
		}

		out = append(out, line)
	}

	return os.WriteFile(path, []byte(strings.Join(out, "\n")), 0644)
}

// validateComposeSecurity parses a docker-compose file and rejects directives
// that would let a service escape its container to the host. It is intentionally
// strict about the unambiguous escape vectors and conservative elsewhere to
// avoid breaking legitimate stacks.
func validateComposeSecurity(composeFile string) error {
	data, err := os.ReadFile(composeFile)
	if err != nil {
		return err
	}

	var compose struct {
		Services map[string]struct {
			Privileged  bool        `yaml:"privileged"`
			CapAdd      []string    `yaml:"cap_add"`
			Devices     []any       `yaml:"devices"`
			Pid         string      `yaml:"pid"`
			Ipc         string      `yaml:"ipc"`
			NetworkMode string      `yaml:"network_mode"`
			UsernsMode  string      `yaml:"userns_mode"`
			SecurityOpt []string    `yaml:"security_opt"`
			Volumes     []yaml.Node `yaml:"volumes"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &compose); err != nil {
		return fmt.Errorf("parse compose: %w", err)
	}

	for name, svc := range compose.Services {
		if svc.Privileged {
			return fmt.Errorf("service %q: privileged mode is not allowed", name)
		}
		for _, c := range svc.CapAdd {
			uc := strings.ToUpper(strings.TrimSpace(c))
			if uc == "ALL" || uc == "SYS_ADMIN" || uc == "SYS_PTRACE" || uc == "SYS_MODULE" || uc == "NET_ADMIN" {
				return fmt.Errorf("service %q: cap_add %q is not allowed", name, c)
			}
		}
		if len(svc.Devices) > 0 {
			return fmt.Errorf("service %q: host device passthrough is not allowed", name)
		}
		if isHostNamespace(svc.Pid) {
			return fmt.Errorf("service %q: pid: host is not allowed", name)
		}
		if isHostNamespace(svc.Ipc) {
			return fmt.Errorf("service %q: ipc: host is not allowed", name)
		}
		if isHostNamespace(svc.NetworkMode) {
			return fmt.Errorf("service %q: network_mode: host is not allowed", name)
		}
		if isHostNamespace(svc.UsernsMode) {
			return fmt.Errorf("service %q: userns_mode: host is not allowed", name)
		}
		for _, so := range svc.SecurityOpt {
			ls := strings.ToLower(strings.ReplaceAll(so, " ", ""))
			if strings.Contains(ls, "unconfined") || strings.Contains(ls, "seccomp=unconfined") {
				return fmt.Errorf("service %q: security_opt %q is not allowed", name, so)
			}
		}
		for _, v := range svc.Volumes {
			if err := checkComposeVolume(name, v); err != nil {
				return err
			}
		}
	}
	return nil
}

func isHostNamespace(v string) bool {
	v = strings.ToLower(strings.TrimSpace(strings.Trim(v, "\"'")))
	return v == "host" || strings.HasPrefix(v, "host:")
}

// dangerousMountSources are host paths that must never be bind-mounted into a
// tenant container (they enable container escape or host tampering).
var dangerousMountSources = []string{
	"/var/run/docker.sock", "/run/docker.sock", "/var/run", "/run",
	"/", "/etc", "/root", "/proc", "/sys", "/boot", "/dev",
	"/var/lib/docker", "/home",
}

func checkComposeVolume(service string, node yaml.Node) error {
	var source string
	switch node.Kind {
	case yaml.ScalarNode:
		// Short syntax: "source:target[:mode]"
		parts := strings.SplitN(node.Value, ":", 2)
		source = parts[0]
	case yaml.MappingNode:
		// Long syntax: { type: bind, source: /host/path, target: ... }
		var m struct {
			Source string `yaml:"source"`
		}
		if err := node.Decode(&m); err == nil {
			source = m.Source
		}
	}
	source = strings.TrimSpace(source)
	if source == "" || !strings.HasPrefix(source, "/") {
		// Named volumes and relative paths are fine.
		return nil
	}
	clean := filepath.Clean(source)
	for _, bad := range dangerousMountSources {
		if clean == bad || strings.HasPrefix(clean, bad+"/") {
			return fmt.Errorf("service %q: bind mount of host path %q is not allowed", service, source)
		}
	}
	return nil
}

// discoverComposeServices reads docker-compose.yml and returns the service names.
func discoverComposeServices(composeFile string) ([]string, error) {
	data, err := os.ReadFile(composeFile)
	if err != nil {
		return nil, err
	}
	var compose struct {
		Services map[string]interface{} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &compose); err != nil {
		return nil, err
	}
	var names []string
	for name := range compose.Services {
		names = append(names, name)
	}
	return names, nil
}

// setupComposeCgroup creates a systemd slice for shared resource limits.
// All containers in the compose site share a single memory/PID pool.
func setupComposeCgroup(ctx context.Context, project string, memoryMB int64, pidsLimit int64) (string, error) {
	sliceName := "apod-" + project + ".slice"

	// Create a systemd transient slice with resource limits
	args := []string{"systemd-run", "--unit=" + sliceName, "--scope", "--slice=" + sliceName}
	if memoryMB > 0 {
		args = append(args, fmt.Sprintf("-p=MemoryMax=%dM", memoryMB))
	}
	if pidsLimit > 0 {
		args = append(args, fmt.Sprintf("-p=TasksMax=%d", pidsLimit))
	}
	args = append(args, "true") // just run true to create the slice

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.CombinedOutput() // best effort — slice may already exist

	// Also create the slice file for persistence across reboots
	sliceContent := fmt.Sprintf(`[Slice]
MemoryMax=%dM
TasksMax=%d
`, memoryMB, pidsLimit)
	slicePath := fmt.Sprintf("/etc/systemd/system/%s", sliceName)
	if err := os.WriteFile(slicePath, []byte(sliceContent), 0644); err == nil {
		exec.CommandContext(ctx, "systemctl", "daemon-reload").Run()
		exec.CommandContext(ctx, "systemctl", "start", sliceName).Run()
	}

	return sliceName, nil
}

// generateComposeOverride creates docker-compose.override.yml that adds apod
// labels, security hardening, and shared resource pool to every service.
// All containers share a single cgroup for memory/PIDs — no per-container splitting.
func generateComposeOverride(compDir, domain, project string, services []string, comp *models.DriverCompose, cgroupParent string) error {
	if len(services) == 0 {
		return nil
	}

	var override strings.Builder
	override.WriteString("# Auto-generated by apod — do not edit\n")
	override.WriteString("services:\n")

	for _, svc := range services {
		override.WriteString(fmt.Sprintf("  %s:\n", svc))

		// Labels — makes this container discoverable by apod
		override.WriteString("    labels:\n")
		override.WriteString(fmt.Sprintf("      - \"apod.site=%s\"\n", domain))
		override.WriteString(fmt.Sprintf("      - \"apod.service=%s\"\n", svc))
		override.WriteString("      - \"apod.managed=true\"\n")
		if comp.ShellService != "" && svc == comp.ShellService {
			override.WriteString("      - \"apod.shell=true\"\n")
		}

		// Shared resource pool via cgroup parent
		if cgroupParent != "" {
			override.WriteString(fmt.Sprintf("    cgroup_parent: %s\n", cgroupParent))
		}

		// Security hardening
		override.WriteString("    security_opt:\n")
		override.WriteString("      - no-new-privileges:true\n")
	}

	return os.WriteFile(filepath.Join(compDir, "docker-compose.override.yml"), []byte(override.String()), 0644)
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

	// Sanitize compose file for multi-instance support
	composeFile := filepath.Join(compDir, "docker-compose.yml")
	if err := sanitizeComposeFile(composeFile); err != nil {
		return fmt.Errorf("sanitize compose file: %w", err)
	}

	// Reject compose files that request host-level privileges or escapes. The
	// line-based sanitizer above only rewrites ports/names; this parses the
	// YAML and refuses dangerous directives (privileged, host namespaces,
	// SYS_ADMIN/ALL caps, docker-socket mounts) that would allow a malicious
	// driver or repo to escape the container to host root.
	if err := validateComposeSecurity(composeFile); err != nil {
		return fmt.Errorf("compose security check: %w", err)
	}

	// Discover services and set up shared resource pool
	services, err := discoverComposeServices(composeFile)
	if err != nil {
		return fmt.Errorf("discover services: %w", err)
	}

	project := composeProjectName(opts.Domain)

	memoryMB := parseMemoryMB(opts.RAM)
	if memoryMB == 0 {
		memoryMB = 2048
	}
	totalPids := int64(512) * int64(len(services)) // 512 per service as total pool

	// Create shared cgroup — all containers compete for the same memory/PID pool
	cgroupParent := ""
	if sliceName, err := setupComposeCgroup(ctx, project, memoryMB, totalPids); err == nil {
		cgroupParent = sliceName
	}

	if err := generateComposeOverride(compDir, opts.Domain, project, services, comp, cgroupParent); err != nil {
		return fmt.Errorf("generate override: %w", err)
	}

	// Start compose (automatically picks up override file)
	cmd := composeCmd(ctx, project, compDir, "up", "-d")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker compose up: %s: %w", string(output), err)
	}

	// Connect Traefik to the compose network
	composeNetwork := project + "_default"
	e.docker.ConnectNetwork(ctx, composeNetwork, "apod-traefik")

	// Write Traefik routing config
	if comp.ProxyService != "" && comp.ProxyPort != "" {
		routerName := strings.ReplaceAll(opts.Domain, ".", "-")

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

	// Remove shared cgroup slice
	sliceName := "apod-" + project + ".slice"
	exec.CommandContext(ctx, "systemctl", "stop", sliceName).Run()
	os.Remove(fmt.Sprintf("/etc/systemd/system/%s", sliceName))

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
