package db

import "testing"

func TestUserPasswordHash(t *testing.T) {
	d := openTestDB(t)
	if err := d.CreateUser("alice", "keyhash1", "user", 5001); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// No password set yet
	hash, err := d.GetUserPasswordHash("alice")
	if err != nil {
		t.Fatalf("GetUserPasswordHash: %v", err)
	}
	if hash != "" {
		t.Errorf("got hash %q for fresh user, want empty", hash)
	}

	if err := d.SetUserPasswordHash("alice", "bcrypt-hash-here"); err != nil {
		t.Fatalf("SetUserPasswordHash: %v", err)
	}
	hash, err = d.GetUserPasswordHash("alice")
	if err != nil {
		t.Fatalf("GetUserPasswordHash after set: %v", err)
	}
	if hash != "bcrypt-hash-here" {
		t.Errorf("got hash %q, want bcrypt-hash-here", hash)
	}
}

func TestSetPasswordHashUnknownUser(t *testing.T) {
	d := openTestDB(t)
	if err := d.SetUserPasswordHash("ghost", "x"); err == nil {
		t.Error("expected error for unknown user")
	}
}

func TestListUsersHasPassword(t *testing.T) {
	d := openTestDB(t)
	d.CreateUser("alice", "keyhash1", "user", 5001)
	d.CreateUser("bob", "keyhash2", "user", 5002)
	d.SetUserPasswordHash("alice", "some-hash")

	users, err := d.ListUsers()
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	byName := map[string]bool{}
	for _, u := range users {
		byName[u.Name] = u.HasPassword
	}
	if !byName["alice"] {
		t.Error("alice should have has_password=true")
	}
	if byName["bob"] {
		t.Error("bob should have has_password=false")
	}
}
