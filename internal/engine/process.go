package engine

import (
	"context"
	"fmt"
	"strconv"
	"strings"

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
	Service  string `json:"service"`
	Role     string `json:"role"`
	Image    string `json:"image"`
	Command  string `json:"command"`
	Replicas int    `json:"replicas"` // desired
	Running  int    `json:"running"`  // currently up
	Scalable bool   `json:"scalable"`
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

	var out []ProcessInfo
	for svcName, svc := range driver.Services {
		role := effectiveRole(svcName, svc.Role)
		desired := resolveReplicas(role, svc.Replicas, overrideFor(svcName))
		running := 0
		if ids, lerr := e.serviceContainers(ctx, domain, svcName); lerr == nil {
			running = len(ids)
		}
		out = append(out, ProcessInfo{
			Service:  svcName,
			Role:     role,
			Image:    svc.Image,
			Command:  svc.Command,
			Replicas: desired,
			Running:  running,
			Scalable: scalableRole(role),
		})
	}
	return out, nil
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

	ids, err := e.serviceContainers(ctx, domain, svcName)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return fmt.Errorf("no containers for service %q", svcName)
	}
	for _, id := range ids {
		if err := e.docker.StopContainer(ctx, id); err != nil {
			return fmt.Errorf("stop %s: %w", id, err)
		}
		if err := e.docker.StartContainer(ctx, id); err != nil {
			return fmt.Errorf("start %s: %w", id, err)
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
