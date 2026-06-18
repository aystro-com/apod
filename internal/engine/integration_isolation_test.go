//go:build dockerintegration

package engine

import (
	"context"
	"strings"
	"testing"
)

// TestIntegrationSiteIsolation verifies that a container created on a named
// site network joins that network ONLY (not Docker's default bridge), and that
// containers on two different site networks cannot reach each other by raw IP.
func TestIntegrationSiteIsolation(t *testing.T) {
	e := newDockerEngine(t)
	ctx := context.Background()

	netA := "apod-site-itisoA-test"
	netB := "apod-site-itisoB-test"
	nameA := "apod-itisoA-test-app"
	nameB := "apod-itisoB-test-app"

	cleanup := func() {
		for _, n := range []string{nameA, nameB} {
			e.docker.StopContainer(ctx, n)
			e.docker.RemoveContainer(ctx, n)
		}
		e.docker.RemoveNetwork(ctx, netA)
		e.docker.RemoveNetwork(ctx, netB)
	}
	cleanup()
	t.Cleanup(cleanup)

	if err := e.docker.PullImage(ctx, itImage); err != nil {
		t.Fatalf("pull image: %v", err)
	}
	mk := func(name, net string) string {
		if err := e.docker.EnsureNetwork(ctx, net); err != nil {
			t.Fatalf("network %s: %v", net, err)
		}
		id, err := e.docker.CreateContainer(ctx, ContainerConfig{
			Name:        name,
			Image:       itImage,
			Args:        []string{"sleep", "3600"},
			NetworkName: net,
		})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if err := e.docker.StartContainer(ctx, id); err != nil {
			t.Fatalf("start %s: %v", name, err)
		}
		return id
	}
	mk(nameA, netA)
	mk(nameB, netB)

	// Container A must be on its site network ONLY — never the default bridge.
	infoA, err := e.docker.cli.ContainerInspect(ctx, nameA)
	if err != nil {
		t.Fatalf("inspect A: %v", err)
	}
	if _, onBridge := infoA.NetworkSettings.Networks["bridge"]; onBridge {
		t.Error("container must not join the default bridge network")
	}
	if _, onSite := infoA.NetworkSettings.Networks[netA]; !onSite {
		t.Errorf("container should be on its site network %s", netA)
	}
	if n := len(infoA.NetworkSettings.Networks); n != 1 {
		t.Errorf("expected exactly 1 network, got %d", n)
	}

	// B's IP must be unreachable from A (different isolated networks).
	infoB, _ := e.docker.cli.ContainerInspect(ctx, nameB)
	var bIP string
	for _, n := range infoB.NetworkSettings.Networks {
		bIP = n.IPAddress
	}
	if bIP == "" {
		t.Fatal("could not determine B's IP")
	}
	out, _ := e.docker.ExecInContainer(ctx, nameA,
		[]string{"sh", "-c", "ping -c1 -W2 " + bIP + " >/dev/null 2>&1 && echo REACHABLE || echo BLOCKED"})
	if !strings.Contains(out, "BLOCKED") {
		t.Errorf("cross-site reach to %s should be BLOCKED, got %q", bIP, out)
	}
}
