package engine

import "testing"

func TestAggregateComposeProcesses(t *testing.T) {
	// A single-service compose site (web: nginx, one replica) — the proxy
	// service must be marked web so it lands in the App column.
	got := aggregateComposeProcesses([]SiteContainer{
		{Service: "web", Image: "nginx:alpine", Running: true},
	}, "web")
	if len(got) != 1 {
		t.Fatalf("got %d processes, want 1", len(got))
	}
	p := got[0]
	if p.Service != "web" || p.Role != roleWeb {
		t.Errorf("web service should be the web role, got %+v", p)
	}
	if p.Image != "nginx:alpine" || p.Replicas != 1 || p.Running != 1 {
		t.Errorf("unexpected process info: %+v", p)
	}
}

func TestAggregateComposeProcessesGroupsAndCounts(t *testing.T) {
	// app (2 replicas, 1 down) is the proxy → web; db is a backing service.
	got := aggregateComposeProcesses([]SiteContainer{
		{Service: "app", Image: "img", Running: true},
		{Service: "app", Image: "img", Running: false},
		{Service: "db", Image: "postgres", Running: true},
	}, "app")

	if len(got) != 2 {
		t.Fatalf("got %d services, want 2", len(got))
	}
	// Deterministic order (sorted): app, db.
	if got[0].Service != "app" || got[0].Role != roleWeb {
		t.Errorf("app should be web: %+v", got[0])
	}
	if got[0].Replicas != 2 || got[0].Running != 1 {
		t.Errorf("app counts wrong: %+v", got[0])
	}
	if got[1].Service != "db" || got[1].Role != "" {
		t.Errorf("db should be a plain service: %+v", got[1])
	}
}

func TestAggregateComposeProcessesEmpty(t *testing.T) {
	if got := aggregateComposeProcesses(nil, "web"); len(got) != 0 {
		t.Errorf("no containers should yield no processes, got %d", len(got))
	}
}
