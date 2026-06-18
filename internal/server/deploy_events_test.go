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

// sseGet issues a raw GET (not the JSON envelope helper). A terminal progress
// event must already be buffered for the domain so the handler returns instead
// of blocking on the live channel.
func sseGet(t *testing.T, s *Server, domain, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/v1/sites/"+domain+"/deploy/events", nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	return w
}

func TestDeployEventsOwnershipAndStream(t *testing.T) {
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
	if err := d.CreateSite(&models.Site{Domain: "alice-app.test", Driver: "x", Owner: "alice"}); err != nil {
		t.Fatal(err)
	}
	if err := d.CreateSite(&models.Site{Domain: "bob-app.test", Driver: "x", Owner: "bob"}); err != nil {
		t.Fatal(err)
	}

	eng := engine.NewWithDB(d)
	s := New(eng)

	// Buffer a terminal event for each domain so the stream returns at once.
	term := engine.ProgressEvent{Step: "Ready", Status: "done", Percent: 100}
	eng.EmitProgress("alice-app.test", term)
	eng.EmitProgress("bob-app.test", term)

	// SECURITY: bob (non-admin) cannot watch alice's deployment.
	if w := sseGet(t, s, "alice-app.test", "apod_bobkey"); w.Code != http.StatusForbidden {
		t.Errorf("non-owner watch: got %d, want 403", w.Code)
	}

	// SECURITY: an unauthenticated request never reaches the stream.
	if w := sseGet(t, s, "bob-app.test", ""); w.Code == http.StatusOK {
		t.Errorf("unauthenticated deploy stream should not be 200, got %d", w.Code)
	}

	// The owner can watch their own deployment and receives the SSE stream.
	w := sseGet(t, s, "bob-app.test", "apod_bobkey")
	if w.Code != http.StatusOK {
		t.Fatalf("owner watch: got %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("content-type = %q, want text/event-stream", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "data: ") || !strings.Contains(body, `"step":"Ready"`) {
		t.Errorf("stream body missing SSE event:\n%s", body)
	}

	// An admin can watch any site.
	if w := sseGet(t, s, "alice-app.test", "apod_rootkey"); w.Code != http.StatusOK {
		t.Errorf("admin watch: got %d, want 200", w.Code)
	}
}
