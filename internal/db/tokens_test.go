package db

import (
	"testing"
	"time"
)

func TestCreateAndGetAPIToken(t *testing.T) {
	d := openTestDB(t)
	d.CreateUser("alice", "keyhash1", "user", 5001)

	if err := d.CreateAPIToken("alice", "ci", "tokhash1", "read,deploy", false, nil); err != nil {
		t.Fatalf("CreateAPIToken: %v", err)
	}

	tok, err := d.GetAPITokenByHash("tokhash1")
	if err != nil {
		t.Fatalf("GetAPITokenByHash: %v", err)
	}
	if tok == nil {
		t.Fatal("token not found")
	}
	if tok.UserName != "alice" || tok.Abilities != "read,deploy" || tok.Sensitive {
		t.Errorf("got %+v", tok)
	}
}

func TestGetAPITokenUnknown(t *testing.T) {
	d := openTestDB(t)
	tok, err := d.GetAPITokenByHash("nope")
	if err != nil {
		t.Fatalf("GetAPITokenByHash: %v", err)
	}
	if tok != nil {
		t.Errorf("got %+v for unknown hash, want nil", tok)
	}
}

func TestGetAPITokenExpired(t *testing.T) {
	d := openTestDB(t)
	d.CreateUser("alice", "keyhash1", "user", 5001)
	past := time.Now().Add(-time.Hour)
	d.CreateAPIToken("alice", "old", "oldhash", "read", false, &past)

	tok, err := d.GetAPITokenByHash("oldhash")
	if err != nil {
		t.Fatalf("GetAPITokenByHash: %v", err)
	}
	if tok != nil {
		t.Error("expired token still valid")
	}
}

func TestListAndDeleteAPITokens(t *testing.T) {
	d := openTestDB(t)
	d.CreateUser("alice", "keyhash1", "user", 5001)
	d.CreateUser("bob", "keyhash2", "user", 5002)
	d.CreateAPIToken("alice", "ci", "h1", "read", false, nil)
	d.CreateAPIToken("alice", "deploy-bot", "h2", "deploy", false, nil)
	d.CreateAPIToken("bob", "bobs", "h3", "read,write", true, nil)

	tokens, err := d.ListAPITokens("alice")
	if err != nil {
		t.Fatalf("ListAPITokens: %v", err)
	}
	if len(tokens) != 2 {
		t.Fatalf("got %d tokens, want 2", len(tokens))
	}

	// Deleting is scoped to the owner — bob cannot delete alice's token.
	if err := d.DeleteAPIToken("bob", tokens[0].ID); err == nil {
		t.Error("cross-user token delete succeeded")
	}
	if err := d.DeleteAPIToken("alice", tokens[0].ID); err != nil {
		t.Fatalf("DeleteAPIToken: %v", err)
	}
	tokens, _ = d.ListAPITokens("alice")
	if len(tokens) != 1 {
		t.Errorf("got %d tokens after delete, want 1", len(tokens))
	}
}

func TestDeleteAPITokensForUser(t *testing.T) {
	d := openTestDB(t)
	d.CreateUser("alice", "keyhash1", "user", 5001)
	d.CreateAPIToken("alice", "ci", "h1", "read", false, nil)
	if err := d.DeleteAPITokensForUser("alice"); err != nil {
		t.Fatalf("DeleteAPITokensForUser: %v", err)
	}
	if tok, _ := d.GetAPITokenByHash("h1"); tok != nil {
		t.Error("token survived user-wide revocation")
	}
}

func TestUserTOTPLifecycle(t *testing.T) {
	d := openTestDB(t)
	d.CreateUser("alice", "keyhash1", "user", 5001)

	secret, enabled, err := d.GetUserTOTP("alice")
	if err != nil {
		t.Fatalf("GetUserTOTP: %v", err)
	}
	if secret != "" || enabled {
		t.Errorf("fresh user has totp state: %q %v", secret, enabled)
	}

	if err := d.SetUserTOTPSecret("alice", "BASE32SECRET"); err != nil {
		t.Fatalf("SetUserTOTPSecret: %v", err)
	}
	if err := d.SetUserTOTPEnabled("alice", true); err != nil {
		t.Fatalf("SetUserTOTPEnabled: %v", err)
	}
	secret, enabled, _ = d.GetUserTOTP("alice")
	if secret != "BASE32SECRET" || !enabled {
		t.Errorf("got %q %v", secret, enabled)
	}

	// Disabling clears the secret.
	if err := d.ClearUserTOTP("alice"); err != nil {
		t.Fatalf("ClearUserTOTP: %v", err)
	}
	secret, enabled, _ = d.GetUserTOTP("alice")
	if secret != "" || enabled {
		t.Errorf("totp not cleared: %q %v", secret, enabled)
	}
}

func TestTOTPLastStepPreventsReplayStorage(t *testing.T) {
	d := openTestDB(t)
	d.CreateUser("alice", "keyhash1", "user", 5001)

	if step, _ := d.GetUserTOTPLastStep("alice"); step != 0 {
		t.Errorf("fresh user last step = %d, want 0", step)
	}
	if err := d.SetUserTOTPLastStep("alice", 12345); err != nil {
		t.Fatalf("SetUserTOTPLastStep: %v", err)
	}
	if step, _ := d.GetUserTOTPLastStep("alice"); step != 12345 {
		t.Errorf("got %d, want 12345", step)
	}
}

func TestRecoveryCodes(t *testing.T) {
	d := openTestDB(t)
	d.CreateUser("alice", "keyhash1", "user", 5001)

	if err := d.SetUserRecoveryCodes("alice", `["h1","h2"]`); err != nil {
		t.Fatalf("SetUserRecoveryCodes: %v", err)
	}
	codes, err := d.GetUserRecoveryCodes("alice")
	if err != nil {
		t.Fatalf("GetUserRecoveryCodes: %v", err)
	}
	if codes != `["h1","h2"]` {
		t.Errorf("got %q", codes)
	}
}

func TestCountUsers(t *testing.T) {
	d := openTestDB(t)
	n, err := d.CountUsers()
	if err != nil {
		t.Fatalf("CountUsers: %v", err)
	}
	if n != 0 {
		t.Errorf("got %d, want 0", n)
	}
	d.CreateUser("alice", "keyhash1", "user", 5001)
	if n, _ := d.CountUsers(); n != 1 {
		t.Errorf("got %d, want 1", n)
	}
}
