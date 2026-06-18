//go:build dockerintegration

// These tests drive the real Docker daemon. Run with:
//
//	go test -tags dockerintegration ./internal/engine/ -run Integration -v
//
// They exercise the worker-replica orchestration (scale up/down, restart,
// list) end-to-end against live containers using a tiny alpine image, without
// the heavy app stack.
package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aystro/apod/internal/db"
	"github.com/aystro/apod/internal/models"
)

const itDomain = "it-worker.test"
const itImage = "alpine:latest"

func newDockerEngine(t *testing.T) *Engine {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "it.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	docker, err := NewDocker()
	if err != nil {
		t.Fatalf("docker client: %v", err)
	}
	if err := docker.Ping(context.Background()); err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}

	// Minimal driver: one scalable worker, plus a plain backing service.
	driverDir := t.TempDir()
	driverYAML := `name: alpinetest
version: "1.0"
services:
  worker:
    image: "alpine:latest"
    role: worker
    replicas: 1
    command: "sleep 3600"
  cache:
    image: "alpine:latest"
    command: "sleep 3600"
`
	if err := os.WriteFile(filepath.Join(driverDir, "alpinetest.yaml"), []byte(driverYAML), 0644); err != nil {
		t.Fatalf("write driver: %v", err)
	}

	return &Engine{
		db:      d,
		docker:  docker,
		drivers: NewDriverLoader(driverDir),
		locks:   NewLockManager(),
	}
}

func siteNetworkName(domain string) string {
	return "apod-site-" + strings.ReplaceAll(domain, ".", "-")
}

// seedReplicaZero creates the network and the first worker container the way
// CreateSite would, so the scaling path has a template to clone from.
func seedReplicaZero(t *testing.T, e *Engine) {
	t.Helper()
	ctx := context.Background()
	net := siteNetworkName(itDomain)
	if err := e.docker.EnsureNetwork(ctx, net); err != nil {
		t.Fatalf("ensure network: %v", err)
	}
	if err := e.docker.PullImage(ctx, itImage); err != nil {
		t.Fatalf("pull image: %v", err)
	}
	name := replicaContainerName(itDomain, "worker", 0)
	id, err := e.docker.CreateContainer(ctx, ContainerConfig{
		Name:  name,
		Image: itImage,
		Args:  []string{"sleep", "3600"},
		Labels: map[string]string{
			labelPrefix + "site":    itDomain,
			labelPrefix + "service": "worker",
			labelPrefix + "role":    roleWorker,
			labelPrefix + "replica": "0",
			labelPrefix + "managed": "true",
		},
	})
	if err != nil {
		t.Fatalf("create replica 0: %v", err)
	}
	if err := e.docker.ConnectNetwork(ctx, net, id); err != nil {
		t.Fatalf("connect network: %v", err)
	}
	if err := e.docker.StartContainer(ctx, id); err != nil {
		t.Fatalf("start replica 0: %v", err)
	}
}

func cleanupSite(e *Engine) {
	ctx := context.Background()
	ids, _ := e.docker.ListContainersByLabel(ctx, labelPrefix+"site", itDomain)
	for _, id := range ids {
		e.docker.StopContainer(ctx, id)
		e.docker.RemoveContainer(ctx, id)
	}
	e.docker.RemoveNetwork(ctx, siteNetworkName(itDomain))
}

func TestIntegrationWorkerScaling(t *testing.T) {
	e := newDockerEngine(t)
	ctx := context.Background()

	if err := e.db.CreateSite(&models.Site{
		Domain: itDomain, Driver: "alpinetest", Status: "running", Owner: "tester",
	}); err != nil {
		t.Fatalf("seed site: %v", err)
	}

	cleanupSite(e) // start clean
	seedReplicaZero(t, e)
	t.Cleanup(func() { cleanupSite(e) })

	count := func() int {
		ids, err := e.serviceContainers(ctx, itDomain, "worker")
		if err != nil {
			t.Fatalf("list worker containers: %v", err)
		}
		return len(ids)
	}

	if got := count(); got != 1 {
		t.Fatalf("expected 1 worker before scaling, got %d", got)
	}

	// Scale up to 3 — reconcile must clone replica 0 into two more containers.
	if err := e.ScaleProcess(ctx, itDomain, "worker", 3); err != nil {
		t.Fatalf("scale up: %v", err)
	}
	if got := count(); got != 3 {
		t.Fatalf("expected 3 workers after scale-up, got %d", got)
	}
	// The cloned replicas must carry the right names and replica labels.
	for i := 0; i < 3; i++ {
		name := replicaContainerName(itDomain, "worker", i)
		exists, err := e.docker.ContainerExists(ctx, name)
		if err != nil || !exists {
			t.Fatalf("expected replica %d (%s) to exist (err=%v)", i, name, err)
		}
	}

	// ListProcesses should report 3 running / 3 desired for the worker.
	procs, err := e.ListProcesses(ctx, itDomain)
	if err != nil {
		t.Fatalf("list processes: %v", err)
	}
	var worker *ProcessInfo
	for i := range procs {
		if procs[i].Service == "worker" {
			worker = &procs[i]
		}
	}
	if worker == nil {
		t.Fatal("worker process not listed")
	}
	if worker.Role != roleWorker || !worker.Scalable {
		t.Errorf("worker role/scalable wrong: %+v", worker)
	}
	if worker.Replicas != 3 || worker.Running != 3 {
		t.Errorf("expected 3 desired/3 running, got %d/%d", worker.Replicas, worker.Running)
	}

	// Restart all replicas — count stays 3, containers come back up.
	if err := e.RestartProcess(ctx, itDomain, "worker"); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if got := count(); got != 3 {
		t.Fatalf("expected 3 workers after restart, got %d", got)
	}

	// Scale down to 1 — highest-indexed replicas are removed.
	if err := e.ScaleProcess(ctx, itDomain, "worker", 1); err != nil {
		t.Fatalf("scale down: %v", err)
	}
	if got := count(); got != 1 {
		t.Fatalf("expected 1 worker after scale-down, got %d", got)
	}
	if exists, _ := e.docker.ContainerExists(ctx, replicaContainerName(itDomain, "worker", 0)); !exists {
		t.Error("replica 0 must survive scale-down to 1")
	}
	if exists, _ := e.docker.ContainerExists(ctx, replicaContainerName(itDomain, "worker", 2)); exists {
		t.Error("replica 2 must be removed on scale-down")
	}

	// Scale to zero (pause) — no worker containers remain.
	if err := e.ScaleProcess(ctx, itDomain, "worker", 0); err != nil {
		t.Fatalf("scale to zero: %v", err)
	}
	if got := count(); got != 0 {
		t.Fatalf("expected 0 workers after scale-to-zero, got %d", got)
	}

	// The override is persisted.
	if n, ok, _ := e.db.GetProcessReplicas(itDomain, "worker"); !ok || n != 0 {
		t.Errorf("expected persisted override 0, got (%d,%v)", n, ok)
	}

}

func TestIntegrationScaleRejectsNonWorker(t *testing.T) {
	e := newDockerEngine(t)
	ctx := context.Background()
	if err := e.db.CreateSite(&models.Site{
		Domain: itDomain, Driver: "alpinetest", Status: "running", Owner: "tester",
	}); err != nil {
		t.Fatalf("seed site: %v", err)
	}
	t.Cleanup(func() { cleanupSite(e) })

	// "cache" has no role => plain backing service, not scalable.
	if err := e.ScaleProcess(ctx, itDomain, "cache", 3); err == nil {
		t.Error("scaling a non-worker service should be rejected")
	}
}
