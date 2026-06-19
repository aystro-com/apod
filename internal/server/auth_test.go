package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aystro/apod/internal/db"
	"github.com/aystro/apod/internal/engine"
)

// newAuthTestServer builds a Server over a temp SQLite DB with two users:
// root (admin) and bob (user). Auth endpoints never touch Docker.
func newAuthTestServer(t *testing.T) *Server {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	if err := d.CreateUser("root", engine.HashAPIKey("apod_rootkey"), "admin", 5000); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	if err := d.CreateUser("bob", engine.HashAPIKey("apod_bobkey"), "user", 5001); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return New(engine.NewWithDB(d))
}

func doJSON(t *testing.T, s *Server, method, path, bearer string, body string) (*httptest.ResponseRecorder, map[string]json.RawMessage) {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	var envelope struct {
		OK    bool                       `json:"ok"`
		Data  map[string]json.RawMessage `json:"data"`
		Error string                     `json:"error"`
	}
	raw := w.Body.Bytes()
	json.Unmarshal(raw, &envelope)
	// Restore the body so callers can still inspect w.Body.String().
	w.Body = bytes.NewBuffer(raw)
	return w, envelope.Data
}

func setPassword(t *testing.T, s *Server, adminKey, user, password string) {
	t.Helper()
	w, _ := doJSON(t, s, "POST", "/api/v1/users/"+user+"/password", adminKey,
		`{"password":"`+password+`"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("set password: got %d: %s", w.Code, w.Body.String())
	}
}

func TestLoginSuccess(t *testing.T) {
	s := newAuthTestServer(t)
	setPassword(t, s, "apod_rootkey", "bob", "a-long-password")

	w, data := doJSON(t, s, "POST", "/api/v1/auth/login", "",
		`{"name":"bob","password":"a-long-password"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}

	var token string
	json.Unmarshal(data["token"], &token)
	if !strings.HasPrefix(token, "apod_sess_") {
		t.Errorf("token %q missing apod_sess_ prefix", token)
	}
	var user struct {
		Name string `json:"name"`
		Role string `json:"role"`
	}
	json.Unmarshal(data["user"], &user)
	if user.Name != "bob" || user.Role != "user" {
		t.Errorf("got user %+v", user)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	s := newAuthTestServer(t)
	setPassword(t, s, "apod_rootkey", "bob", "a-long-password")

	w, _ := doJSON(t, s, "POST", "/api/v1/auth/login", "",
		`{"name":"bob","password":"wrong"}`)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", w.Code)
	}
}

func TestLoginMissingFields(t *testing.T) {
	s := newAuthTestServer(t)
	w, _ := doJSON(t, s, "POST", "/api/v1/auth/login", "", `{"name":"bob"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", w.Code)
	}
}

func TestMeWithAPIKey(t *testing.T) {
	s := newAuthTestServer(t)
	w, data := doJSON(t, s, "GET", "/api/v1/auth/me", "apod_bobkey", "")
	if w.Code != http.StatusOK {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}
	var name string
	json.Unmarshal(data["name"], &name)
	if name != "bob" {
		t.Errorf("got name %q, want bob", name)
	}
}

func TestMeWithSessionToken(t *testing.T) {
	s := newAuthTestServer(t)
	setPassword(t, s, "apod_rootkey", "bob", "a-long-password")
	_, loginData := doJSON(t, s, "POST", "/api/v1/auth/login", "",
		`{"name":"bob","password":"a-long-password"}`)
	var token string
	json.Unmarshal(loginData["token"], &token)

	w, data := doJSON(t, s, "GET", "/api/v1/auth/me", token, "")
	if w.Code != http.StatusOK {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}
	var role string
	json.Unmarshal(data["role"], &role)
	if role != "user" {
		t.Errorf("got role %q, want user", role)
	}
}

func TestMeUnauthenticated(t *testing.T) {
	s := newAuthTestServer(t)
	w, _ := doJSON(t, s, "GET", "/api/v1/auth/me", "", "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", w.Code)
	}
}

func TestSessionTokenWorksOnRegularEndpoints(t *testing.T) {
	s := newAuthTestServer(t)
	setPassword(t, s, "apod_rootkey", "bob", "a-long-password")
	_, loginData := doJSON(t, s, "POST", "/api/v1/auth/login", "",
		`{"name":"bob","password":"a-long-password"}`)
	var token string
	json.Unmarshal(loginData["token"], &token)

	w, _ := doJSON(t, s, "GET", "/api/v1/sites", token, "")
	if w.Code != http.StatusOK {
		t.Errorf("session token rejected on /sites: %d", w.Code)
	}
}

func TestLogoutRevokesSession(t *testing.T) {
	s := newAuthTestServer(t)
	setPassword(t, s, "apod_rootkey", "bob", "a-long-password")
	_, loginData := doJSON(t, s, "POST", "/api/v1/auth/login", "",
		`{"name":"bob","password":"a-long-password"}`)
	var token string
	json.Unmarshal(loginData["token"], &token)

	if w, _ := doJSON(t, s, "POST", "/api/v1/auth/logout", token, ""); w.Code != http.StatusOK {
		t.Fatalf("logout: got %d", w.Code)
	}
	if w, _ := doJSON(t, s, "GET", "/api/v1/auth/me", token, ""); w.Code != http.StatusUnauthorized {
		t.Errorf("token still valid after logout: %d", w.Code)
	}
}

func TestUserCanSetOwnPasswordOnly(t *testing.T) {
	s := newAuthTestServer(t)

	// bob sets his own password — allowed
	w, _ := doJSON(t, s, "POST", "/api/v1/users/bob/password", "apod_bobkey",
		`{"password":"bobs-new-password"}`)
	if w.Code != http.StatusOK {
		t.Errorf("self password set: got %d, want 200", w.Code)
	}

	// bob tries to set root's password — forbidden
	w, _ = doJSON(t, s, "POST", "/api/v1/users/root/password", "apod_bobkey",
		`{"password":"evil-password-here"}`)
	if w.Code != http.StatusForbidden {
		t.Errorf("cross-user password set: got %d, want 403", w.Code)
	}

	// admin sets bob's password — allowed
	w, _ = doJSON(t, s, "POST", "/api/v1/users/bob/password", "apod_rootkey",
		`{"password":"admin-set-password"}`)
	if w.Code != http.StatusOK {
		t.Errorf("admin password set: got %d, want 200", w.Code)
	}
}

// Once a user has a password, a self-service change must prove knowledge of the
// current one — a valid session/key alone is not enough (stolen-session defense).
func TestSelfPasswordChangeRequiresCurrent(t *testing.T) {
	s := newAuthTestServer(t)

	// Set an initial password (no current password needed for the first set).
	if w, _ := doJSON(t, s, "POST", "/api/v1/users/bob/password", "apod_bobkey",
		`{"password":"bobs-first-password"}`); w.Code != http.StatusOK {
		t.Fatalf("initial password set: got %d, want 200", w.Code)
	}

	// Changing it without the current password is rejected.
	if w, _ := doJSON(t, s, "POST", "/api/v1/users/bob/password", "apod_bobkey",
		`{"password":"bobs-second-password"}`); w.Code != http.StatusBadRequest {
		t.Errorf("change without current password: got %d, want 400", w.Code)
	}

	// Supplying the correct current password succeeds.
	if w, _ := doJSON(t, s, "POST", "/api/v1/users/bob/password", "apod_bobkey",
		`{"current_password":"bobs-first-password","password":"bobs-second-password"}`); w.Code != http.StatusOK {
		t.Errorf("change with current password: got %d, want 200", w.Code)
	}
}

func TestPasswordTooShortRejected(t *testing.T) {
	s := newAuthTestServer(t)
	w, _ := doJSON(t, s, "POST", "/api/v1/users/bob/password", "apod_rootkey",
		`{"password":"short"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", w.Code)
	}
}
