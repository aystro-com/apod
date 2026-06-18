package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// socketHandler wraps the router exactly like ListenSocket does, so requests
// are marked as coming from the local control socket.
func socketHandler(s *Server) http.Handler {
	return UnixSocketMiddleware(s.router)
}

// TestDirectSocketIsAdmin verifies the local CLI (direct socket, no proxy
// header) still gets implicit admin on an admin-only endpoint.
func TestDirectSocketIsAdmin(t *testing.T) {
	s := newAuthTestServer(t)
	req := httptest.NewRequest("GET", "/api/v1/users", nil)
	w := httptest.NewRecorder()
	socketHandler(s).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("direct socket admin: got %d, want 200: %s", w.Code, w.Body.String())
	}
}

// TestProxiedSocketRequiresAuth verifies that a request forwarded by the web
// proxy (X-Apod-Proxied set) over the socket does NOT inherit admin and must
// authenticate — closing the unauthenticated-admin bypass via the panel.
func TestProxiedSocketRequiresAuth(t *testing.T) {
	s := newAuthTestServer(t)

	// No credentials -> must be rejected.
	req := httptest.NewRequest("GET", "/api/v1/users", nil)
	req.Header.Set("X-Apod-Proxied", "1")
	w := httptest.NewRecorder()
	socketHandler(s).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("proxied socket without auth: got %d, want 401: %s", w.Code, w.Body.String())
	}

	// Valid admin API key -> allowed.
	req = httptest.NewRequest("GET", "/api/v1/users", nil)
	req.Header.Set("X-Apod-Proxied", "1")
	req.Header.Set("Authorization", "Bearer apod_rootkey")
	w = httptest.NewRecorder()
	socketHandler(s).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("proxied socket with admin key: got %d, want 200: %s", w.Code, w.Body.String())
	}

	// Valid non-admin key -> forbidden on admin endpoint (not implicit admin).
	req = httptest.NewRequest("GET", "/api/v1/users", nil)
	req.Header.Set("X-Apod-Proxied", "1")
	req.Header.Set("Authorization", "Bearer apod_bobkey")
	w = httptest.NewRecorder()
	socketHandler(s).ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("proxied socket with user key on admin endpoint: got %d, want 403: %s", w.Code, w.Body.String())
	}
}
