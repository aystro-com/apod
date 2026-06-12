package server

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/aystro/apod/internal/db"
	"github.com/aystro/apod/internal/engine"
)

// cleanupLinuxUser removes a Linux account created by the real engine setup
// path so repeated test runs don't collide on useradd.
func cleanupLinuxUser(t *testing.T, name string) {
	t.Cleanup(func() {
		exec.Command("userdel", "--remove", "--force", name).Run()
	})
}

// newEmptyServer builds a Server with NO users — the first-run state.
func newEmptyServer(t *testing.T) *Server {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return New(engine.NewWithDB(d))
}

func TestSetupStatus(t *testing.T) {
	s := newEmptyServer(t)
	w, data := doJSON(t, s, "GET", "/api/v1/setup/status", "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("got %d", w.Code)
	}
	var needs bool
	json.Unmarshal(data["needs_setup"], &needs)
	if !needs {
		t.Error("fresh instance should need setup")
	}

	// Once a user exists, setup is reported as done.
	populated := newAuthTestServer(t)
	_, data = doJSON(t, populated, "GET", "/api/v1/setup/status", "", "")
	json.Unmarshal(data["needs_setup"], &needs)
	if needs {
		t.Error("populated instance should not need setup")
	}
}

func TestSetupCreatesFirstAdminWithPassword(t *testing.T) {
	s := newEmptyServer(t)
	cleanupLinuxUser(t, "root-admin")

	w, _ := doJSON(t, s, "POST", "/api/v1/setup", "",
		`{"name":"root-admin","password":"a-strong-password"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("setup: got %d: %s", w.Code, w.Body.String())
	}

	// The new admin can log in with the password right away.
	w, data := doJSON(t, s, "POST", "/api/v1/auth/login", "",
		`{"name":"root-admin","password":"a-strong-password"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("login after setup: got %d: %s", w.Code, w.Body.String())
	}
	var user struct {
		Role string `json:"role"`
	}
	json.Unmarshal(data["user"], &user)
	if user.Role != "admin" {
		t.Errorf("first user role = %q, want admin", user.Role)
	}
}

func TestSetupRefusedOnceUsersExist(t *testing.T) {
	s := newAuthTestServer(t)
	w, _ := doJSON(t, s, "POST", "/api/v1/setup", "",
		`{"name":"intruder","password":"a-strong-password"}`)
	if w.Code != http.StatusForbidden {
		t.Errorf("setup on populated instance: got %d, want 403", w.Code)
	}
}

func TestSetupValidatesInput(t *testing.T) {
	s := newEmptyServer(t)
	// Weak password rejected.
	w, _ := doJSON(t, s, "POST", "/api/v1/setup", "",
		`{"name":"root-admin","password":"short"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("weak password: got %d, want 400", w.Code)
	}
	// Bad username rejected.
	w, _ = doJSON(t, s, "POST", "/api/v1/setup", "",
		`{"name":"Bad Name!","password":"a-strong-password"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("bad name: got %d, want 400", w.Code)
	}
}
