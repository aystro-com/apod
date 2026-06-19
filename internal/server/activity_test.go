package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aystro/apod/internal/db"
	"github.com/aystro/apod/internal/engine"
	"github.com/aystro/apod/internal/models"
)

func get(t *testing.T, s *Server, path, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	return w
}

func activityTestServer(t *testing.T) *Server {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	if err := d.CreateUser("root", engine.HashAPIKey("apod_rootkey"), "admin", 5000); err != nil {
		t.Fatal(err)
	}
	if err := d.CreateUser("bob", engine.HashAPIKey("apod_bobkey"), "user", 5001); err != nil {
		t.Fatal(err)
	}
	if err := d.CreateSite(&models.Site{Domain: "bob-app.test", Driver: "x", Owner: "bob"}); err != nil {
		t.Fatal(err)
	}
	return New(engine.NewWithDB(d))
}

func TestSiteActivityEndpoint(t *testing.T) {
	s := activityTestServer(t)
	path := "/api/v1/sites/bob-app.test/activity"

	// Owner reads activity for an idle site: 200 and held=false.
	w := get(t, s, path, "apod_bobkey")
	if w.Code != http.StatusOK {
		t.Fatalf("owner activity: got %d, want 200", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, `"held":false`) {
		t.Errorf("idle activity body = %s, want held=false", body)
	}

	// SECURITY: an unauthenticated request is rejected.
	if w := get(t, s, path, ""); w.Code == http.StatusOK {
		t.Errorf("unauthenticated activity should not be 200, got %d", w.Code)
	}
}

// The generic /events alias serves the same progress stream as /deploy/events,
// so the UI can subscribe to any operation's progress through one endpoint.
func TestSiteEventsAliasStreams(t *testing.T) {
	s := activityTestServer(t)
	s.handler.engine.EmitProgress("bob-app.test", engine.ProgressEvent{
		Step: "Destroyed", Status: "done", Percent: 100,
	})

	w := get(t, s, "/api/v1/sites/bob-app.test/events", "apod_bobkey")
	if w.Code != http.StatusOK {
		t.Fatalf("events alias: got %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("content-type = %q, want text/event-stream", ct)
	}
	if body := w.Body.String(); !strings.Contains(body, `"step":"Destroyed"`) {
		t.Errorf("stream body missing buffered event:\n%s", body)
	}
}
