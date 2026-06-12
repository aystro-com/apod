package db

import (
	"testing"
	"time"
)

func TestCreateAndGetSession(t *testing.T) {
	d := openTestDB(t)
	if err := d.CreateUser("alice", "keyhash1", "user", 5001); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	expires := time.Now().Add(1 * time.Hour)
	if err := d.CreateSession("tokenhash1", "alice", expires); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	name, err := d.GetSessionUser("tokenhash1")
	if err != nil {
		t.Fatalf("GetSessionUser: %v", err)
	}
	if name != "alice" {
		t.Errorf("got user %q, want alice", name)
	}
}

func TestGetSessionUnknownToken(t *testing.T) {
	d := openTestDB(t)
	name, err := d.GetSessionUser("nope")
	if err != nil {
		t.Fatalf("GetSessionUser: %v", err)
	}
	if name != "" {
		t.Errorf("got user %q for unknown token, want empty", name)
	}
}

func TestGetSessionExpired(t *testing.T) {
	d := openTestDB(t)
	d.CreateUser("alice", "keyhash1", "user", 5001)
	expired := time.Now().Add(-1 * time.Minute)
	if err := d.CreateSession("oldtoken", "alice", expired); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	name, err := d.GetSessionUser("oldtoken")
	if err != nil {
		t.Fatalf("GetSessionUser: %v", err)
	}
	if name != "" {
		t.Errorf("expired session returned user %q, want empty", name)
	}
}

func TestDeleteSession(t *testing.T) {
	d := openTestDB(t)
	d.CreateUser("alice", "keyhash1", "user", 5001)
	d.CreateSession("tok", "alice", time.Now().Add(time.Hour))

	if err := d.DeleteSession("tok"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	name, _ := d.GetSessionUser("tok")
	if name != "" {
		t.Error("session still valid after delete")
	}
}

func TestDeleteSessionsForUser(t *testing.T) {
	d := openTestDB(t)
	d.CreateUser("alice", "keyhash1", "user", 5001)
	d.CreateUser("bob", "keyhash2", "user", 5002)
	d.CreateSession("tok-a1", "alice", time.Now().Add(time.Hour))
	d.CreateSession("tok-a2", "alice", time.Now().Add(time.Hour))
	d.CreateSession("tok-b", "bob", time.Now().Add(time.Hour))

	if err := d.DeleteSessionsForUser("alice"); err != nil {
		t.Fatalf("DeleteSessionsForUser: %v", err)
	}

	if name, _ := d.GetSessionUser("tok-a1"); name != "" {
		t.Error("alice session 1 survived")
	}
	if name, _ := d.GetSessionUser("tok-a2"); name != "" {
		t.Error("alice session 2 survived")
	}
	if name, _ := d.GetSessionUser("tok-b"); name != "bob" {
		t.Error("bob's session was deleted too")
	}
}

func TestDeleteExpiredSessions(t *testing.T) {
	d := openTestDB(t)
	d.CreateUser("alice", "keyhash1", "user", 5001)
	d.CreateSession("live", "alice", time.Now().Add(time.Hour))
	d.CreateSession("dead", "alice", time.Now().Add(-time.Hour))

	if err := d.DeleteExpiredSessions(); err != nil {
		t.Fatalf("DeleteExpiredSessions: %v", err)
	}

	var count int
	d.Conn().QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&count)
	if count != 1 {
		t.Errorf("got %d sessions after cleanup, want 1", count)
	}
}
