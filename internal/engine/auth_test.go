package engine

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/aystro/apod/internal/db"
)

// newAuthTestEngine builds an Engine backed by a temp SQLite DB.
// Auth never touches Docker, so the other engine subsystems stay nil.
func newAuthTestEngine(t *testing.T) *Engine {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	if err := d.CreateUser("alice", HashAPIKey("apod_alicekey"), "user", 5001); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return NewWithDB(d)
}

func TestSetUserPasswordTooShort(t *testing.T) {
	e := newAuthTestEngine(t)
	if err := e.SetUserPassword("alice", "short"); err == nil {
		t.Error("expected error for short password")
	}
}

func TestSetUserPasswordUnknownUser(t *testing.T) {
	e := newAuthTestEngine(t)
	if err := e.SetUserPassword("ghost", "longenoughpassword"); err == nil {
		t.Error("expected error for unknown user")
	}
}

func TestLoginWithPassword(t *testing.T) {
	e := newAuthTestEngine(t)
	if err := e.SetUserPassword("alice", "correct-horse-battery"); err != nil {
		t.Fatalf("SetUserPassword: %v", err)
	}

	token, user, err := e.LoginWithPassword("alice", "correct-horse-battery")
	if err != nil {
		t.Fatalf("LoginWithPassword: %v", err)
	}
	if !strings.HasPrefix(token, "apod_sess_") {
		t.Errorf("token %q missing apod_sess_ prefix", token)
	}
	if user == nil || user.Name != "alice" {
		t.Errorf("got user %+v, want alice", user)
	}
}

func TestLoginWithWrongPassword(t *testing.T) {
	e := newAuthTestEngine(t)
	e.SetUserPassword("alice", "correct-horse-battery")

	_, _, err := e.LoginWithPassword("alice", "wrong-password")
	if err == nil {
		t.Fatal("expected error for wrong password")
	}
	_, _, err2 := e.LoginWithPassword("ghost", "whatever-password")
	if err2 == nil {
		t.Fatal("expected error for unknown user")
	}
	// Same generic message for both — no username enumeration.
	if err.Error() != err2.Error() {
		t.Errorf("error messages differ: %q vs %q", err.Error(), err2.Error())
	}
}

func TestLoginWithoutPasswordSet(t *testing.T) {
	e := newAuthTestEngine(t)
	if _, _, err := e.LoginWithPassword("alice", "anything-at-all"); err == nil {
		t.Error("expected error when no password is set")
	}
}

func TestValidateSessionToken(t *testing.T) {
	e := newAuthTestEngine(t)
	e.SetUserPassword("alice", "correct-horse-battery")
	token, _, _ := e.LoginWithPassword("alice", "correct-horse-battery")

	user, err := e.ValidateSessionToken(token)
	if err != nil {
		t.Fatalf("ValidateSessionToken: %v", err)
	}
	if user == nil || user.Name != "alice" {
		t.Errorf("got %+v, want alice", user)
	}

	if u, _ := e.ValidateSessionToken("apod_sess_bogus"); u != nil {
		t.Error("bogus token validated")
	}
}

func TestLogout(t *testing.T) {
	e := newAuthTestEngine(t)
	e.SetUserPassword("alice", "correct-horse-battery")
	token, _, _ := e.LoginWithPassword("alice", "correct-horse-battery")

	if err := e.Logout(token); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if u, _ := e.ValidateSessionToken(token); u != nil {
		t.Error("token still valid after logout")
	}
}

func TestPasswordChangeRevokesSessions(t *testing.T) {
	e := newAuthTestEngine(t)
	e.SetUserPassword("alice", "correct-horse-battery")
	token, _, _ := e.LoginWithPassword("alice", "correct-horse-battery")

	if err := e.SetUserPassword("alice", "brand-new-password1"); err != nil {
		t.Fatalf("SetUserPassword: %v", err)
	}
	if u, _ := e.ValidateSessionToken(token); u != nil {
		t.Error("old session survived password change")
	}
}

func TestResetAPIKeyRevokesSessions(t *testing.T) {
	e := newAuthTestEngine(t)
	e.SetUserPassword("alice", "correct-horse-battery")
	token, _, _ := e.LoginWithPassword("alice", "correct-horse-battery")

	if _, err := e.ResetAPIKey(t.Context(), "alice"); err != nil {
		t.Fatalf("ResetAPIKey: %v", err)
	}
	if u, _ := e.ValidateSessionToken(token); u != nil {
		t.Error("session survived API key reset")
	}
}

func TestIsSessionToken(t *testing.T) {
	if !IsSessionToken("apod_sess_abc") {
		t.Error("apod_sess_ token not recognized")
	}
	if IsSessionToken("apod_regularapikey") {
		t.Error("API key misidentified as session token")
	}
}
