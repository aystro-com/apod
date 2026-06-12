package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aystro/apod/internal/totp"
)

// loginSession logs bob in and returns a session token.
func loginSession(t *testing.T, s *Server, name, password string) string {
	t.Helper()
	w, data := doJSON(t, s, "POST", "/api/v1/auth/login", "",
		`{"name":"`+name+`","password":"`+password+`"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("login: got %d: %s", w.Code, w.Body.String())
	}
	var token string
	json.Unmarshal(data["token"], &token)
	return token
}

func TestTwoFactorHTTPFlow(t *testing.T) {
	s := newAuthTestServer(t)
	setPassword(t, s, "apod_rootkey", "bob", "a-long-password")
	session := loginSession(t, s, "bob", "a-long-password")

	// Setup returns a secret and otpauth URI.
	w, data := doJSON(t, s, "POST", "/api/v1/auth/2fa/setup", session, "")
	if w.Code != http.StatusOK {
		t.Fatalf("2fa setup: got %d: %s", w.Code, w.Body.String())
	}
	var secret, uri string
	json.Unmarshal(data["secret"], &secret)
	json.Unmarshal(data["uri"], &uri)
	if secret == "" || !strings.HasPrefix(uri, "otpauth://totp/") {
		t.Fatalf("bad setup payload: %q %q", secret, uri)
	}

	// Enabling with a wrong code fails.
	w, _ = doJSON(t, s, "POST", "/api/v1/auth/2fa/enable", session, `{"code":"000000"}`)
	if w.Code != http.StatusBadRequest && w.Code != http.StatusUnauthorized {
		t.Errorf("enable with wrong code: got %d", w.Code)
	}

	// Enabling with the current code returns recovery codes.
	code := currentTOTPForTest(t, secret)
	w, data = doJSON(t, s, "POST", "/api/v1/auth/2fa/enable", session, `{"code":"`+code+`"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("2fa enable: got %d: %s", w.Code, w.Body.String())
	}
	var recovery []string
	json.Unmarshal(data["recovery_codes"], &recovery)
	if len(recovery) != 8 {
		t.Fatalf("got %d recovery codes", len(recovery))
	}

	// Password-only login now fails with the distinct 2fa_required marker.
	w, _ = doJSON(t, s, "POST", "/api/v1/auth/login", "",
		`{"name":"bob","password":"a-long-password"}`)
	if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), "2fa_required") {
		t.Fatalf("expected 2fa_required 401, got %d: %s", w.Code, w.Body.String())
	}

	// Login with a recovery code works.
	w, _ = doJSON(t, s, "POST", "/api/v1/auth/login", "",
		`{"name":"bob","password":"a-long-password","code":"`+recovery[0]+`"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("recovery login: got %d: %s", w.Code, w.Body.String())
	}
}

func TestTwoFactorStatusEndpoint(t *testing.T) {
	s := newAuthTestServer(t)
	w, data := doJSON(t, s, "GET", "/api/v1/auth/me", "apod_bobkey", "")
	if w.Code != http.StatusOK {
		t.Fatalf("me: got %d", w.Code)
	}
	var enabled bool
	json.Unmarshal(data["totp_enabled"], &enabled)
	if enabled {
		t.Error("fresh user reports 2FA enabled")
	}
}

// currentTOTPForTest computes the current code for a secret, used to drive the
// 2FA HTTP flow in tests.
func currentTOTPForTest(t *testing.T, secret string) string {
	t.Helper()
	code, err := totp.Code(secret, time.Now())
	if err != nil {
		t.Fatalf("totp.Code: %v", err)
	}
	return code
}
