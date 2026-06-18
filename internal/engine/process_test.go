package engine

import "testing"

func TestEffectiveRole(t *testing.T) {
	cases := []struct {
		svc, role, want string
	}{
		{"app", "", roleWeb},          // legacy: app with no role => web
		{"db", "", ""},                // legacy backing service stays plain
		{"queue", "worker", roleWorker},
		{"cron", "scheduler", roleScheduler},
		{"app", "worker", roleWorker}, // explicit role wins over the name
	}
	for _, c := range cases {
		if got := effectiveRole(c.svc, c.role); got != c.want {
			t.Errorf("effectiveRole(%q,%q)=%q want %q", c.svc, c.role, got, c.want)
		}
	}
}

func TestScalableRole(t *testing.T) {
	if !scalableRole(roleWorker) {
		t.Error("workers must be scalable")
	}
	for _, r := range []string{roleWeb, roleScheduler, ""} {
		if scalableRole(r) {
			t.Errorf("role %q must not be user-scalable in v1", r)
		}
	}
}

func TestResolveReplicas(t *testing.T) {
	n := 3
	zero := 0
	cases := []struct {
		role     string
		driver   int
		override *int
		want     int
	}{
		{roleWeb, 5, &n, 1},        // web is a singleton regardless
		{roleScheduler, 5, &n, 1},  // scheduler is a singleton
		{"", 5, &n, 1},             // plain backing service is a singleton
		{roleWorker, 2, nil, 2},    // worker uses the driver default
		{roleWorker, 0, nil, 1},    // unset driver default => at least 1
		{roleWorker, 2, &n, 3},     // override beats the driver default
		{roleWorker, 2, &zero, 0},  // scale-to-zero (pause) is allowed
	}
	for i, c := range cases {
		if got := resolveReplicas(c.role, c.driver, c.override); got != c.want {
			t.Errorf("case %d resolveReplicas(%q,%d,%v)=%d want %d", i, c.role, c.driver, c.override, got, c.want)
		}
	}
}

func TestReplicaContainerName(t *testing.T) {
	if got := replicaContainerName("ex.com", "queue", 0); got != "apod-ex.com-queue" {
		t.Errorf("replica 0 should keep the legacy name, got %q", got)
	}
	if got := replicaContainerName("ex.com", "queue", 2); got != "apod-ex.com-queue-2" {
		t.Errorf("replica N name wrong, got %q", got)
	}
}
