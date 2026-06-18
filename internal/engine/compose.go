package engine

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

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

	// Pre-parse to learn which services already declare an explicit hostname —
	// for those, container_name must be dropped rather than converted, or the
	// service would end up with two hostname keys (invalid YAML). This bit a
	// real LinuxServer compose (syncthing sets both).
	servicesWithHostname := map[string]bool{}
	var doc struct {
		Services map[string]struct {
			Hostname string `yaml:"hostname"`
		} `yaml:"services"`
	}
	if yaml.Unmarshal(data, &doc) == nil {
		for name, svc := range doc.Services {
			if strings.TrimSpace(svc.Hostname) != "" {
				servicesWithHostname[name] = true
			}
		}
	}

	lines := strings.Split(string(data), "\n")
	var out []string
	inPorts := false
	portsIndent := 0
	serviceIndent := -1
	currentService := ""

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		indentWidth := len(line) - len(strings.TrimLeft(line, " \t"))

		// Track the current service so container_name handling can consult
		// whether that service already has a hostname.
		if trimmed == "services:" {
			serviceIndent = -1 // set on the next non-empty child
		} else if serviceIndent == -1 && trimmed != "" && !strings.HasPrefix(trimmed, "#") && len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "services:" {
			serviceIndent = indentWidth
		}
		if serviceIndent >= 0 && indentWidth == serviceIndent && strings.HasSuffix(trimmed, ":") && !strings.Contains(trimmed, " ") {
			currentService = strings.TrimSuffix(trimmed, ":")
		}

		if strings.HasPrefix(line, "name:") {
			continue
		}

		if strings.HasPrefix(trimmed, "container_name:") {
			// Drop it when the service already has an explicit hostname,
			// otherwise convert it so the name stays internally addressable.
			if servicesWithHostname[currentService] {
				continue
			}
			indent := line[:indentWidth]
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

	compDir := filepath.Dir(composeFile)
	for name, svc := range compose.Services {
		if svc.Privileged {
			return fmt.Errorf("service %q: privileged mode is not allowed", name)
		}
		// Allowlist: only capabilities Docker already grants by default may be
		// added (adding them is a no-op). Anything else — DAC_READ_SEARCH,
		// SYS_ADMIN, SYS_PTRACE, BPF, … — is an escalation and is rejected.
		for _, c := range svc.CapAdd {
			uc := strings.ToUpper(strings.TrimSpace(strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(c)), "CAP_")))
			if !defaultDockerCaps[uc] {
				return fmt.Errorf("service %q: cap_add %q is not allowed", name, c)
			}
		}
		if len(svc.Devices) > 0 {
			return fmt.Errorf("service %q: host device passthrough is not allowed", name)
		}
		// Reject host namespaces AND joining another container's/service's
		// namespace (pid/ipc/network "container:..."/"service:..."), which would
		// let a tenant peer into Traefik or another tenant's container.
		if err := checkComposeNamespace(name, "pid", svc.Pid); err != nil {
			return err
		}
		if err := checkComposeNamespace(name, "ipc", svc.Ipc); err != nil {
			return err
		}
		if err := checkComposeNamespace(name, "network_mode", svc.NetworkMode); err != nil {
			return err
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
			if err := checkComposeVolume(name, compDir, v); err != nil {
				return err
			}
		}
	}
	return nil
}

// defaultDockerCaps is the set of Linux capabilities Docker grants a container
// by default (without --privileged). Adding any of these via cap_add is a no-op;
// anything outside the set is a privilege escalation we refuse.
var defaultDockerCaps = map[string]bool{
	"CHOWN": true, "DAC_OVERRIDE": true, "FSETID": true, "FOWNER": true,
	"MKNOD": true, "NET_RAW": true, "SETGID": true, "SETUID": true,
	"SETFCAP": true, "SETPCAP": true, "NET_BIND_SERVICE": true,
	"SYS_CHROOT": true, "KILL": true, "AUDIT_WRITE": true,
}

func isHostNamespace(v string) bool {
	v = strings.ToLower(strings.TrimSpace(strings.Trim(v, "\"'")))
	return v == "host" || strings.HasPrefix(v, "host:")
}

// checkComposeNamespace rejects sharing a host namespace or joining another
// container's/service's namespace for pid/ipc/network_mode.
func checkComposeNamespace(service, field, v string) error {
	lv := strings.ToLower(strings.TrimSpace(strings.Trim(v, "\"'")))
	if isHostNamespace(lv) {
		return fmt.Errorf("service %q: %s: host is not allowed", service, field)
	}
	if strings.HasPrefix(lv, "container:") || strings.HasPrefix(lv, "service:") {
		return fmt.Errorf("service %q: %s joining another container/service namespace is not allowed", service, field)
	}
	return nil
}

// dangerousMountSources are host paths that must never be bind-mounted into a
// tenant container (they enable container escape or host tampering).
var dangerousMountSources = []string{
	"/var/run/docker.sock", "/run/docker.sock", "/var/run", "/run",
	"/", "/etc", "/root", "/proc", "/sys", "/boot", "/dev",
	"/var/lib/docker", "/home",
}

// apodControlSocketDir is the one host path a (native, admin-authored) driver is
// allowed to mount despite being under /run: the apod-ui panel proxies the API
// through the daemon control socket that lives here.
const apodControlSocketDir = "/run/apod"

// nativeDangerousMounts is the blocklist for native (admin-authored) driver
// volumes. It omits /home — native site data legitimately lives under
// /home/<owner>/sites/… — but keeps every container-escape / host-tamper path.
var nativeDangerousMounts = []string{
	"/var/run/docker.sock", "/run/docker.sock", "/var/run", "/run",
	"/", "/etc", "/root", "/proc", "/sys", "/boot", "/dev", "/var/lib/docker",
}

// validateNativeHostMount applies the dangerous-mount blocklist to a native
// driver's bind-mount source. Unlike tenant compose, native drivers may mount
// the apod control-socket dir (the panel needs it) and paths under /home.
// Relative/named sources are allowed.
func validateNativeHostMount(source string) error {
	source = strings.TrimSpace(source)
	if !strings.HasPrefix(source, "/") {
		return nil // named volume or relative path
	}
	clean := filepath.Clean(source)
	if clean == apodControlSocketDir || strings.HasPrefix(clean, apodControlSocketDir+"/") {
		return nil
	}
	for _, bad := range nativeDangerousMounts {
		if clean == bad || strings.HasPrefix(clean, bad+"/") {
			return Invalid("bind mount of host path %q is not allowed", source)
		}
	}
	return nil
}

func checkComposeVolume(service, compDir string, node yaml.Node) error {
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
	if source == "" {
		return nil
	}
	// A named volume (no slash, not "./" or "../") is fine.
	if !strings.HasPrefix(source, "/") && !strings.HasPrefix(source, ".") {
		return nil
	}
	// Resolve relative bind mounts against the compose project dir the way Docker
	// does, so "../../../var/run/docker.sock" can't slip past an absolute-only
	// blocklist. Reject anything that escapes the project dir entirely.
	clean := filepath.Clean(source)
	if !strings.HasPrefix(source, "/") {
		abs := filepath.Clean(filepath.Join(compDir, source))
		if abs != compDir && !strings.HasPrefix(abs, compDir+string(filepath.Separator)) {
			return fmt.Errorf("service %q: relative bind mount %q escapes the project directory", service, source)
		}
		clean = abs
	}
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

// CreateSiteFromCompose stands up a site directly from a raw docker-compose.yml
// — no git repo and no hand-written driver. It auto-detects the web service and
// port, generates a compose driver from the file, persists it (so later
// stop/start/destroy/exec reload it like any driver) and creates the site.
func (e *Engine) CreateSiteFromCompose(ctx context.Context, opts CreateSiteOpts, composeContent string) error {
	if strings.TrimSpace(composeContent) == "" {
		return Invalid("compose file is empty")
	}
	// A common mistake is pasting an apod *driver* into the compose box. Driver
	// YAML references apod variables (${site_root}, …) that docker compose would
	// try to interpolate and fail on — detect it and point the user at the right
	// option instead of letting it die deep inside `docker compose up`.
	if v := apodDriverVariable(composeContent); v != "" {
		return Invalid("this looks like an apod driver, not a docker-compose file — it references %s, which Compose doesn't understand. Use the Driver option, or paste a plain docker-compose.yml.", v)
	}
	// Fail fast with a clear message if no web service can be routed.
	svc, port, err := composeWebTarget([]byte(composeContent))
	if err != nil {
		return Invalid("could not determine the web service from the compose file: %v — give the web service a 'ports:' entry", err)
	}

	driverName := strings.ReplaceAll(opts.Domain, ".", "-")
	drv := &models.Driver{
		Name:        driverName,
		Version:     "1.0",
		Description: "Imported from docker-compose for " + opts.Domain,
		Type:        "compose",
		Compose: &models.DriverCompose{
			File:         composeContent,
			ProxyService: svc,
			ProxyPort:    port,
		},
	}
	out, err := yaml.Marshal(drv)
	if err != nil {
		return fmt.Errorf("build driver from compose: %w", err)
	}
	if err := e.drivers.Save(driverName, string(out)); err != nil {
		return fmt.Errorf("save generated driver: %w", err)
	}

	opts.Driver = driverName
	return e.CreateSite(ctx, opts)
}

// CreateComposeSite creates a site using docker compose
func (e *Engine) CreateComposeSite(ctx context.Context, opts CreateSiteOpts, driver *models.Driver, vars map[string]string) error {
	comp := driver.Compose
	if comp == nil {
		return fmt.Errorf("driver %q has no compose configuration", opts.Driver)
	}

	compDir := e.composeDir(opts.Owner, opts.Domain)

	// A compose site sources its docker-compose.yml either inline (comp.File —
	// a raw compose pasted/uploaded by the user) or from a git repo. Inline is
	// what lets apod ingest a stock docker-compose.yml with no wrapper.
	if comp.File != "" {
		os.RemoveAll(compDir)
		if err := os.MkdirAll(compDir, 0755); err != nil {
			return fmt.Errorf("create compose dir: %w", err)
		}
		if err := os.WriteFile(filepath.Join(compDir, "docker-compose.yml"), []byte(comp.File), 0644); err != nil {
			return fmt.Errorf("write inline compose file: %w", err)
		}
	} else {
		// Clone the compose repo. Validate the driver-supplied repo/branch and
		// use the same git hardening as the rest of the engine so a malicious
		// driver definition can't inject a dangerous transport or git option.
		branch := comp.Branch
		if branch == "" {
			branch = "master"
		}
		if err := ValidateRepo(comp.Repo); err != nil {
			return err
		}
		if err := ValidateBranch(branch); err != nil {
			return err
		}

		if comp.Path != "" {
			// comp.Path is joined into the cloned repo; reject absolute paths or
			// any "../" escape so it can't move a directory from outside the repo.
			cleanPath := filepath.Clean(comp.Path)
			if filepath.IsAbs(cleanPath) || cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) {
				return Invalid("invalid compose path %q", comp.Path)
			}
			tmpDir := compDir + "-tmp"
			os.RemoveAll(tmpDir)
			args := append(gitHardeningArgs(), "clone", "--branch", branch, "--single-branch", "--depth", "1", "--", comp.Repo, tmpDir)
			cmd := exec.CommandContext(ctx, "git", args...)
			if output, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("clone compose repo: %s: %w", string(output), err)
			}
			os.RemoveAll(compDir)
			if err := os.Rename(filepath.Join(tmpDir, cleanPath), compDir); err != nil {
				return fmt.Errorf("move compose subdir: %w", err)
			}
			os.RemoveAll(tmpDir)
		} else {
			os.RemoveAll(compDir)
			args := append(gitHardeningArgs(), "clone", "--branch", branch, "--single-branch", "--depth", "1", "--", comp.Repo, compDir)
			cmd := exec.CommandContext(ctx, "git", args...)
			if output, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("clone compose repo: %s: %w", string(output), err)
			}
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

	composeFile := filepath.Join(compDir, "docker-compose.yml")

	// Auto-detect the web service/port to route to when the driver doesn't
	// specify them — this must happen BEFORE sanitize strips the ports. It lets
	// a stock compose file run with no hand-written proxy_service/proxy_port.
	proxyService, proxyPort := comp.ProxyService, comp.ProxyPort
	if proxyService == "" || proxyPort == "" {
		if raw, rerr := os.ReadFile(composeFile); rerr == nil {
			if svc, port, derr := composeWebTarget(raw); derr == nil {
				if proxyService == "" {
					proxyService = svc
				}
				if proxyPort == "" {
					proxyPort = port
				}
			}
		}
	}

	// Sanitize compose file for multi-instance support
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
	e.emitProgress(opts.Domain, "Validating configuration", "running", "", 30)

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

	// Start compose, streaming its lifecycle lines as live deploy progress.
	e.emitProgress(opts.Domain, "Pulling images & starting containers", "running", "", 45)
	if err := e.composeUpStreaming(ctx, opts.Domain, project, compDir); err != nil {
		return err
	}

	e.emitProgress(opts.Domain, "Configuring routing", "running", "", 85)
	// Connect Traefik to the compose network
	composeNetwork := project + "_default"
	e.docker.ConnectNetwork(ctx, composeNetwork, "apod-traefik")

	// Write Traefik routing config
	if proxyService != "" && proxyPort != "" {
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
			routerName, routerName, proxyService, proxyPort)

		traefikDir := "/etc/apod/traefik/dynamic"
		os.MkdirAll(traefikDir, 0755)
		if err := os.WriteFile(filepath.Join(traefikDir, opts.Domain+".toml"), []byte(traefikConfig), 0644); err != nil {
			return fmt.Errorf("write traefik config: %w", err)
		}
	}

	return nil
}

// composeProgressKeywords are the docker-compose lifecycle words we surface as
// deploy progress. Restricting to this set means only recognised status lines
// are streamed to watchers — never arbitrary or sensitive output.
var composeProgressKeywords = []string{
	"Pulling", "Pulled", "Creating", "Created", "Starting", "Started", "Running",
}

// sanitizeProgressLine strips control characters and caps length so a single
// noisy compose line (progress bars use carriage returns/ANSI) cannotinjを
// inject garbage or unbounded data into the event stream.
func sanitizeProgressLine(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '\t' {
			r = ' '
		}
		if r < 0x20 || r == 0x7f {
			continue // drop control chars (incl. CR and ANSI escape bytes)
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if len(out) > 160 {
		out = out[:160]
	}
	return out
}

// firstLine returns the text up to the first newline — used to keep a deploy
// failure message to a single, UI-friendly line.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func isComposeProgressLine(line string) bool {
	for _, kw := range composeProgressKeywords {
		if strings.Contains(line, kw) {
			return true
		}
	}
	return false
}

// composeUpStreaming runs `docker compose up -d`, emitting recognised lifecycle
// lines as deploy progress detail in real time. It preserves the original error
// semantics (the failing output is included in the returned error).
func (e *Engine) composeUpStreaming(ctx context.Context, domain, project, compDir string) error {
	cmd := composeCmd(ctx, project, compDir, "up", "-d")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("docker compose up: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("docker compose up: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("docker compose up: %w", err)
	}

	var mu sync.Mutex
	var tail []string
	last := ""
	scan := func(r io.Reader) {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		for sc.Scan() {
			line := sanitizeProgressLine(sc.Text())
			if line == "" {
				continue
			}
			mu.Lock()
			tail = append(tail, line)
			if len(tail) > 40 {
				tail = tail[len(tail)-40:]
			}
			emit := isComposeProgressLine(line) && line != last
			last = line
			mu.Unlock()
			if emit {
				e.emitProgress(domain, "Pulling images & starting containers", "running", line, 60)
			}
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); scan(stdout) }()
	go func() { defer wg.Done(); scan(stderr) }()
	wg.Wait()

	if err := cmd.Wait(); err != nil {
		mu.Lock()
		msg := composeFailureMessage(tail)
		mu.Unlock()
		// A failed `docker compose up` is almost always the user's compose file
		// (bad image, invalid volume, port clash). Surface it as a client error
		// so the real reason reaches them instead of a masked 500.
		return Invalid("docker compose failed: %s", msg)
	}
	return nil
}

// composeFailureMessage picks the most useful line from the tail of compose's
// output — the last line that looks like an error — falling back to the last
// line. Interpolation warnings are skipped so the actual failure shows through.
func composeFailureMessage(tail []string) string {
	errKeywords := []string{"invalid", "error", "failed", "no such", "denied", "not found", "cannot", "unable"}
	for i := len(tail) - 1; i >= 0; i-- {
		low := strings.ToLower(tail[i])
		if strings.Contains(low, "variable is not set") {
			continue // skip interpolation warnings
		}
		for _, kw := range errKeywords {
			if strings.Contains(low, kw) {
				return tail[i]
			}
		}
	}
	if len(tail) > 0 {
		return tail[len(tail)-1]
	}
	return "see server logs for details"
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

// execInComposeSiteInput runs a command in a compose service with input streamed
// to stdin (no argv length limit), returning an error on non-zero exit.
func (e *Engine) execInComposeSiteInput(ctx context.Context, domain, owner, service string, cmdArgs []string, input []byte) error {
	project := composeProjectName(domain)
	args := append([]string{"exec", "-T", service}, cmdArgs...)
	cmd := composeCmd(ctx, project, e.composeDir(owner, domain), args...)
	cmd.Stdin = bytes.NewReader(input)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("compose exec: %s: %w", string(out), err)
	}
	return nil
}

// execInComposeSiteStdout runs a command in a compose service and returns stdout
// only (no stderr), required for capturing dumps without pollution.
func (e *Engine) execInComposeSiteStdout(ctx context.Context, domain, owner, service string, cmdArgs []string) ([]byte, error) {
	project := composeProjectName(domain)
	args := append([]string{"exec", "-T", service}, cmdArgs...)
	cmd := composeCmd(ctx, project, e.composeDir(owner, domain), args...)
	out, err := cmd.Output() // stdout only
	if err != nil {
		return nil, fmt.Errorf("compose exec: %w", err)
	}
	return out, nil
}
