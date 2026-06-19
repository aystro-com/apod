package engine

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aystro/apod/internal/models"
)

// Process roles. A service's role decides how apod runs it: web is HTTP-routed
// through Traefik, worker is a background process scalable to N replicas,
// scheduler is a background singleton (e.g. a periodic ticker). These are
// generic, stack-agnostic process types — a Laravel app maps web/queue/schedule
// onto them, but so does any other framework.
const (
	roleWeb       = "web"
	roleWorker    = "worker"
	roleScheduler = "scheduler"
)

// maxReplicas caps how many containers a single worker can scale to, to bound
// resource use from a stray API call.
const maxReplicas = 20

// effectiveRole infers a service's process role. An empty role is
// backward-compatible: a service named "app" with no explicit role is treated
// as web, and every other unroled service stays a plain single-instance backing
// service (databases, caches, …) exactly as before.
func effectiveRole(svcName, role string) string {
	if role != "" {
		return role
	}
	if svcName == "app" {
		return roleWeb
	}
	return ""
}

// scalableRole reports whether a role's replica count can be changed by the
// user. Only workers scale in v1; web and scheduler are singletons.
func scalableRole(role string) bool {
	return role == roleWorker
}

// resolveReplicas returns how many containers to run for a service. Non-worker
// roles are always singletons. A worker uses its scaling override when one is
// set (including 0, to pause), otherwise the driver default (clamped to >= 1).
func resolveReplicas(role string, driverReplicas int, override *int) int {
	if role != roleWorker {
		return 1
	}
	if override != nil {
		if *override < 0 {
			return 0
		}
		return *override
	}
	if driverReplicas < 1 {
		return 1
	}
	return driverReplicas
}

// replicaContainerName returns the container name for replica index i (0-based).
// Index 0 keeps the legacy "apod-<domain>-<svc>" name so existing
// single-instance services are unaffected; higher indexes get a "-N" suffix.
func replicaContainerName(domain, svc string, i int) string {
	if i == 0 {
		return fmt.Sprintf("apod-%s-%s", domain, svc)
	}
	return fmt.Sprintf("apod-%s-%s-%d", domain, svc, i)
}

// ProcessInfo describes one process (service) of a site for the API/UI graph.
type ProcessInfo struct {
	Service    string         `json:"service"`
	Role       string         `json:"role"`
	Image      string         `json:"image"`
	Command    string         `json:"command"`
	Replicas   int            `json:"replicas"` // desired
	Running    int            `json:"running"`  // currently up
	Scalable   bool           `json:"scalable"`
	Containers []ContainerRef `json:"containers"` // actual containers (name + private IP)
}

// ContainerRef identifies one running container of a service for the
// architecture view: its name and its IP on the site's private network.
type ContainerRef struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
}

// ListProcesses returns the process topology of a site: each driver service with
// its role, desired replica count (driver default overlaid with any scaling
// override), and how many containers are currently running.
func (e *Engine) ListProcesses(ctx context.Context, domain string) ([]ProcessInfo, error) {
	site, err := e.db.GetSite(domain)
	if err != nil {
		return nil, err
	}
	if site == nil {
		return nil, fmt.Errorf("site %q not found", domain)
	}
	driver, err := e.drivers.Load(site.Driver)
	if err != nil {
		return nil, err
	}

	// Compose sites have no driver.Services map — their process model lives in
	// the actual containers. List those so the architecture view shows the real
	// running containers instead of empty placeholders.
	if driver.Type == "compose" {
		return e.composeProcesses(ctx, domain, driver)
	}

	overrides, err := e.db.ListProcessScaling(domain)
	if err != nil {
		return nil, err
	}
	overrideFor := func(svc string) *int {
		if n, ok := overrides[svc]; ok {
			return &n
		}
		return nil
	}

	// Pull the live containers once so each service can list its actual
	// containers (name + private IP) and count how many are up.
	refsByService := map[string][]ContainerRef{}
	runningByService := map[string]int{}
	if containers, lerr := e.docker.ListSiteContainers(ctx, domain); lerr == nil {
		for _, c := range containers {
			if c.Name != "" {
				refsByService[c.Service] = append(refsByService[c.Service], ContainerRef{Name: c.Name, IP: c.IP})
			}
			if c.Running {
				runningByService[c.Service]++
			}
		}
		for svc := range refsByService {
			sortContainerRefs(refsByService[svc])
		}
	}

	var out []ProcessInfo
	for svcName, svc := range driver.Services {
		role := effectiveRole(svcName, svc.Role)
		desired := resolveReplicas(role, svc.Replicas, overrideFor(svcName))
		out = append(out, ProcessInfo{
			Service:    svcName,
			Role:       role,
			Image:      svc.Image,
			Command:    svc.Command,
			Replicas:   desired,
			Running:    runningByService[svcName],
			Scalable:   scalableRole(role),
			Containers: refsByService[svcName],
		})
	}
	return out, nil
}

// sortContainerRefs orders containers by name for stable rendering.
func sortContainerRefs(refs []ContainerRef) {
	sort.Slice(refs, func(i, j int) bool { return refs[i].Name < refs[j].Name })
}

// isContainerNotReady reports whether an exec error is the transient "container
// is still starting / restarting" condition, which is worth retrying (as
// opposed to a real command failure).
func isContainerNotReady(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "is restarting") ||
		strings.Contains(s, "is not running") ||
		strings.Contains(s, "is paused")
}

// composeProcesses builds the process list for a compose-managed site from its
// actual containers (grouped by the apod.service label), marking the configured
// proxy service as the web entrypoint so it lands in the App column.
func (e *Engine) composeProcesses(ctx context.Context, domain string, driver *models.Driver) ([]ProcessInfo, error) {
	containers, err := e.docker.ListSiteContainers(ctx, domain)
	if err != nil {
		return nil, err
	}
	proxySvc := ""
	if driver.Compose != nil {
		proxySvc = driver.Compose.ProxyService
	}
	return aggregateComposeProcesses(containers, proxySvc), nil
}

// aggregateComposeProcesses groups a site's containers by service (counting
// running replicas) and marks the proxy service as the web entrypoint. Pure, so
// it's testable without Docker.
func aggregateComposeProcesses(containers []SiteContainer, proxySvc string) []ProcessInfo {
	type agg struct {
		image          string
		total, running int
		refs           []ContainerRef
	}
	byService := map[string]*agg{}
	var order []string
	for _, c := range containers {
		svc := c.Service
		if svc == "" {
			svc = "service"
		}
		a := byService[svc]
		if a == nil {
			a = &agg{image: c.Image}
			byService[svc] = a
			order = append(order, svc)
		}
		a.total++
		if c.Running {
			a.running++
		}
		if c.Name != "" {
			a.refs = append(a.refs, ContainerRef{Name: c.Name, IP: c.IP})
		}
	}
	sort.Strings(order)

	out := make([]ProcessInfo, 0, len(order))
	for _, svc := range order {
		a := byService[svc]
		role := "" // a backing service by default
		if svc == proxySvc {
			role = roleWeb
		}
		sortContainerRefs(a.refs)
		out = append(out, ProcessInfo{
			Service:    svc,
			Role:       role,
			Image:      a.image,
			Replicas:   a.total,
			Running:    a.running,
			Scalable:   false, // compose replica scaling isn't wired through apod
			Containers: a.refs,
		})
	}
	return out
}

// desiredReplicasFor resolves the replica count for one service, applying any
// persisted scaling override.
func (e *Engine) desiredReplicasFor(domain, svcName string, svc models.DriverService) int {
	role := effectiveRole(svcName, svc.Role)
	var override *int
	if n, ok, _ := e.db.GetProcessReplicas(domain, svcName); ok {
		override = &n
	}
	return resolveReplicas(role, svc.Replicas, override)
}

// serviceContainers returns the container IDs for one service of a site,
// scoped by both the site and service labels (the service name alone is shared
// across sites).
func (e *Engine) serviceContainers(ctx context.Context, domain, svcName string) ([]string, error) {
	return e.docker.ListContainersByLabels(ctx, map[string]string{
		labelPrefix + "site":    domain,
		labelPrefix + "service": svcName,
	})
}

// ScaleProcess sets the desired replica count for a worker service and
// reconciles running containers to match. New replicas are cloned from an
// existing one so they share the same image, env (including generated secrets),
// command, and limits. Only worker-role services can be scaled.
func (e *Engine) ScaleProcess(ctx context.Context, domain, svcName string, replicas int) error {
	if err := e.locks.Acquire(domain); err != nil {
		return err
	}
	defer e.locks.Release(domain)

	if replicas < 0 || replicas > maxReplicas {
		return Invalid("replicas must be between 0 and %d", maxReplicas)
	}
	site, err := e.db.GetSite(domain)
	if err != nil || site == nil {
		return NotFound("site %q not found", domain)
	}
	driver, err := e.drivers.Load(site.Driver)
	if err != nil {
		return err
	}
	svc, ok := driver.Services[svcName]
	if !ok {
		return NotFound("service %q not found in driver %q", svcName, site.Driver)
	}
	if !scalableRole(effectiveRole(svcName, svc.Role)) {
		return Invalid("service %q is not a scalable worker", svcName)
	}

	if err := e.db.SetProcessReplicas(domain, svcName, replicas); err != nil {
		return err
	}
	if err := e.reconcileService(ctx, domain, svcName, replicas); err != nil {
		return err
	}
	// New replica containers must join the site's shared networks too.
	e.reconnectSharedNetworks(ctx, domain)
	e.LogActivity(domain, "process_scale", fmt.Sprintf("%s -> %d", svcName, replicas), "success")
	return nil
}

// reconcileService creates or removes replica containers so the running count
// matches desired. Replicas are indexed 0..desired-1; index 0 carries the
// legacy container name. It clones configuration from any existing replica.
func (e *Engine) reconcileService(ctx context.Context, domain, svcName string, desired int) error {
	current, err := e.serviceContainers(ctx, domain, svcName)
	if err != nil {
		return err
	}
	have := len(current)
	switch {
	case have == desired:
		return nil
	case have > desired:
		// Remove the highest-indexed replicas first.
		for i := have - 1; i >= desired; i-- {
			name := replicaContainerName(domain, svcName, i)
			e.docker.StopContainer(ctx, name)
			if err := e.docker.RemoveContainer(ctx, name); err != nil {
				return fmt.Errorf("remove replica %s: %w", name, err)
			}
		}
		return nil
	default:
		// Scale up: clone an existing replica's config for each new container.
		template, err := e.docker.InspectReplica(ctx, replicaContainerName(domain, svcName, 0))
		if err != nil {
			return fmt.Errorf("inspect replica 0 of %s: %w", svcName, err)
		}
		siteNetwork := fmt.Sprintf("apod-site-%s", strings.ReplaceAll(domain, ".", "-"))
		for i := have; i < desired; i++ {
			cfg := template
			cfg.Name = replicaContainerName(domain, svcName, i)
			cfg.Labels = cloneLabels(template.Labels)
			cfg.Labels[labelPrefix+"replica"] = strconv.Itoa(i)
			// Stay on the isolated site network only (no default bridge).
			cfg.NetworkName = siteNetwork
			id, err := e.docker.CreateContainer(ctx, cfg)
			if err != nil {
				return fmt.Errorf("create replica %s: %w", cfg.Name, err)
			}
			if err := e.docker.StartContainer(ctx, id); err != nil {
				return fmt.Errorf("start replica %s: %w", cfg.Name, err)
			}
		}
		return nil
	}
}

// RestartProcess restarts every container of a service (all replicas).
func (e *Engine) RestartProcess(ctx context.Context, domain, svcName string) error {
	if err := e.locks.Acquire(domain); err != nil {
		return err
	}
	defer e.locks.Release(domain)

	// Detach from the request context. Restarting the panel's own container
	// drops the web client's connection the instant we stop it, which would
	// otherwise cancel ctx and leave the container stopped (a 404 panel that
	// never comes back). The restart must run to completion regardless.
	ctx, cancel := detachCtx(ctx, 2*time.Minute)
	defer cancel()

	ids, err := e.serviceContainers(ctx, domain, svcName)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return fmt.Errorf("no containers for service %q", svcName)
	}
	for _, id := range ids {
		// Atomic restart (single Docker call) — no window where the container is
		// left stopped.
		if err := e.docker.RestartContainer(ctx, id); err != nil {
			return fmt.Errorf("restart %s: %w", id, err)
		}
	}
	e.LogActivity(domain, "process_restart", svcName, "success")
	return nil
}

func cloneLabels(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
