package server

import (
	"encoding/json"
	"net/http"
	"testing"
)

func createToken(t *testing.T, s *Server, bearer, name string, abilities []string, sensitive bool) string {
	t.Helper()
	body, _ := json.Marshal(map[string]interface{}{
		"name":      name,
		"abilities": abilities,
		"sensitive": sensitive,
	})
	w, data := doJSON(t, s, "POST", "/api/v1/tokens", bearer, string(body))
	if w.Code != http.StatusCreated {
		t.Fatalf("create token: got %d: %s", w.Code, w.Body.String())
	}
	var token string
	json.Unmarshal(data["token"], &token)
	return token
}

func TestPATReadOnlyEnforcement(t *testing.T) {
	s := newAuthTestServer(t)
	pat := createToken(t, s, "apod_bobkey", "ro", []string{"read"}, false)

	// GET works.
	if w, _ := doJSON(t, s, "GET", "/api/v1/sites", pat, ""); w.Code != http.StatusOK {
		t.Errorf("read with read token: got %d", w.Code)
	}
	// Mutations are rejected with 403.
	w, _ := doJSON(t, s, "POST", "/api/v1/sites", pat, `{"domain":"x.com","driver":"php"}`)
	if w.Code != http.StatusForbidden {
		t.Errorf("write with read token: got %d, want 403", w.Code)
	}
}

func TestPATDeployAbility(t *testing.T) {
	s := newAuthTestServer(t)
	deployPat := createToken(t, s, "apod_bobkey", "cd", []string{"deploy"}, false)

	// Deploy-class endpoints pass the ability gate (404 = no such site is
	// fine — what matters is it wasn't rejected with 403).
	w, _ := doJSON(t, s, "POST", "/api/v1/sites/nosite.com/restart", deployPat, "")
	if w.Code == http.StatusForbidden {
		t.Errorf("restart with deploy token rejected: %d", w.Code)
	}
	// Generic writes are still rejected.
	w, _ = doJSON(t, s, "POST", "/api/v1/sites/nosite.com/env", deployPat, `{"key":"A","value":"b"}`)
	if w.Code != http.StatusForbidden {
		t.Errorf("env write with deploy-only token: got %d, want 403", w.Code)
	}
}

func TestPATSensitiveDataGuard(t *testing.T) {
	s := newAuthTestServer(t)
	plain := createToken(t, s, "apod_bobkey", "plain", []string{"read", "write"}, false)
	trusted := createToken(t, s, "apod_bobkey", "trusted", []string{"read"}, true)

	// Secret-bearing endpoints are blocked without the sensitive flag…
	for _, path := range []string{
		"/api/v1/sites/x.com/env",
		"/api/v1/sites/x.com/info",
		"/api/v1/sites/x.com/db/export",
		"/api/v1/sites/x.com/webhook",
	} {
		if w, _ := doJSON(t, s, "GET", path, plain, ""); w.Code != http.StatusForbidden {
			t.Errorf("%s without sensitive flag: got %d, want 403", path, w.Code)
		}
	}
	// …and pass the guard with it (404/500 for missing site is fine).
	if w, _ := doJSON(t, s, "GET", "/api/v1/sites/x.com/env", trusted, ""); w.Code == http.StatusForbidden {
		t.Errorf("sensitive read with flag rejected: %d", w.Code)
	}
}

func TestPATCannotManageTokensOrAuth(t *testing.T) {
	s := newAuthTestServer(t)
	pat := createToken(t, s, "apod_bobkey", "full", []string{"read", "write", "deploy"}, true)

	// A PAT must never mint more credentials or change auth settings.
	w, _ := doJSON(t, s, "POST", "/api/v1/tokens", pat, `{"name":"evil","abilities":["read"]}`)
	if w.Code != http.StatusForbidden {
		t.Errorf("PAT minted a PAT: got %d, want 403", w.Code)
	}
	w, _ = doJSON(t, s, "POST", "/api/v1/users/bob/password", pat, `{"password":"newpassword1"}`)
	if w.Code != http.StatusForbidden {
		t.Errorf("PAT changed a password: got %d, want 403", w.Code)
	}
	w, _ = doJSON(t, s, "POST", "/api/v1/auth/2fa/setup", pat, "")
	if w.Code != http.StatusForbidden {
		t.Errorf("PAT touched 2FA setup: got %d, want 403", w.Code)
	}
}

func TestTokenListAndRevoke(t *testing.T) {
	s := newAuthTestServer(t)
	createToken(t, s, "apod_bobkey", "ci", []string{"read"}, false)

	w, data := doJSON(t, s, "GET", "/api/v1/tokens", "apod_bobkey", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list tokens: %d", w.Code)
	}
	var tokens []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	json.Unmarshal(data["tokens"], &tokens)
	if len(tokens) != 1 || tokens[0].Name != "ci" {
		t.Fatalf("got %+v", tokens)
	}

	w, _ = doJSON(t, s, "DELETE", "/api/v1/tokens", "apod_bobkey",
		`{"id":`+jsonInt(tokens[0].ID)+`}`)
	if w.Code != http.StatusOK {
		t.Errorf("revoke: got %d", w.Code)
	}
}

func jsonInt(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}
