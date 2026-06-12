package engine

import (
	"strings"
	"testing"
)

func TestCreateAPITokenValidation(t *testing.T) {
	e := newAuthTestEngine(t)

	if _, err := e.CreateAPIToken("alice", "", []string{"read"}, false, 0); err == nil {
		t.Error("empty token name accepted")
	}
	if _, err := e.CreateAPIToken("alice", "ci", []string{"root"}, false, 0); err == nil {
		t.Error("unknown ability accepted")
	}
	if _, err := e.CreateAPIToken("alice", "ci", nil, false, 0); err == nil {
		t.Error("token with no abilities accepted")
	}
	if _, err := e.CreateAPIToken("ghost", "ci", []string{"read"}, false, 0); err == nil {
		t.Error("token for unknown user accepted")
	}
}

func TestCreateAndValidateAPIToken(t *testing.T) {
	e := newAuthTestEngine(t)

	raw, err := e.CreateAPIToken("alice", "ci", []string{"read", "deploy"}, false, 0)
	if err != nil {
		t.Fatalf("CreateAPIToken: %v", err)
	}
	if !strings.HasPrefix(raw, "apod_pat_") {
		t.Errorf("token %q missing apod_pat_ prefix", raw)
	}

	user, info, err := e.ValidateAPIToken(raw)
	if err != nil {
		t.Fatalf("ValidateAPIToken: %v", err)
	}
	if user == nil || user.Name != "alice" {
		t.Fatalf("got user %+v", user)
	}
	if !info.HasAbility("read") || !info.HasAbility("deploy") {
		t.Error("granted abilities missing")
	}
	if info.HasAbility("write") {
		t.Error("write ability granted but not requested")
	}
	if info.Sensitive {
		t.Error("sensitive flag set but not requested")
	}
}

func TestValidateAPITokenGarbage(t *testing.T) {
	e := newAuthTestEngine(t)
	if u, _, _ := e.ValidateAPIToken("apod_pat_bogus"); u != nil {
		t.Error("bogus token validated")
	}
}

func TestRevokeAPIToken(t *testing.T) {
	e := newAuthTestEngine(t)
	raw, _ := e.CreateAPIToken("alice", "ci", []string{"read"}, false, 0)
	tokens, err := e.ListAPITokens("alice")
	if err != nil || len(tokens) != 1 {
		t.Fatalf("ListAPITokens: %v %d", err, len(tokens))
	}

	if err := e.RevokeAPIToken("alice", tokens[0].ID); err != nil {
		t.Fatalf("RevokeAPIToken: %v", err)
	}
	if u, _, _ := e.ValidateAPIToken(raw); u != nil {
		t.Error("revoked token still valid")
	}
}

func TestPasswordChangeRevokesTokensToo(t *testing.T) {
	// API key reset is the "compromised credentials" action — it must also
	// revoke personal access tokens, not just sessions.
	e := newAuthTestEngine(t)
	raw, _ := e.CreateAPIToken("alice", "ci", []string{"read"}, false, 0)
	if _, err := e.ResetAPIKey(t.Context(), "alice"); err != nil {
		t.Fatalf("ResetAPIKey: %v", err)
	}
	if u, _, _ := e.ValidateAPIToken(raw); u != nil {
		t.Error("PAT survived API key reset")
	}
}

func TestIsAPIToken(t *testing.T) {
	if !IsAPIToken("apod_pat_x") {
		t.Error("apod_pat_ not recognized")
	}
	if IsAPIToken("apod_sess_x") || IsAPIToken("apod_plainkey") {
		t.Error("non-PAT misidentified")
	}
}
