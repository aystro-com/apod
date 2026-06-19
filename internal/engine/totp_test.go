package engine

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"
)

// RFC 6238 Appendix B test vector (SHA-1, 8 digits truncated to 6 here we
// verify our 6-digit implementation against a known secret/time).
func TestTOTPCodeKnownVector(t *testing.T) {
	// RFC 6238 test secret "12345678901234567890"
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).
		EncodeToString([]byte("12345678901234567890"))

	// At time 59s, RFC 6238 SHA-1 gives 94287082 (8 digits) → 287082 (6 digits).
	code, err := totpCode(secret, time.Unix(59, 0))
	if err != nil {
		t.Fatalf("totpCode: %v", err)
	}
	if code != "287082" {
		t.Errorf("got %s, want 287082", code)
	}

	// At time 1111111109, RFC gives 07081804 → 081804.
	code, _ = totpCode(secret, time.Unix(1111111109, 0))
	if code != "081804" {
		t.Errorf("got %s, want 081804", code)
	}
}

func TestVerifyTOTPWindow(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).
		EncodeToString([]byte("12345678901234567890"))
	now := time.Unix(1111111109, 0)

	// Current step code is valid.
	if step, ok := verifyTOTP(secret, "081804", now); !ok || step == 0 {
		t.Error("current code rejected")
	}
	// Previous step (time 1111111080..., code at step-1: time 59+? use code
	// for now-30s) is accepted within the ±1 window.
	prev, _ := totpCode(secret, now.Add(-30*time.Second))
	if _, ok := verifyTOTP(secret, prev, now); !ok {
		t.Error("previous-window code rejected")
	}
	// A code two steps away is rejected.
	far, _ := totpCode(secret, now.Add(-90*time.Second))
	if _, ok := verifyTOTP(secret, far, now); ok {
		t.Error("stale code accepted")
	}
	// Garbage is rejected.
	if _, ok := verifyTOTP(secret, "000000", now); ok {
		t.Error("wrong code accepted")
	}
}

func TestSetup2FA(t *testing.T) {
	e := newAuthTestEngine(t)
	secret, uri, err := e.Setup2FA("alice")
	if err != nil {
		t.Fatalf("Setup2FA: %v", err)
	}
	if len(secret) < 16 {
		t.Errorf("secret too short: %q", secret)
	}
	if !strings.HasPrefix(uri, "otpauth://totp/") || !strings.Contains(uri, secret) {
		t.Errorf("bad otpauth uri: %q", uri)
	}
	// Setup alone must NOT enable 2FA — login still works without a code.
	e.SetUserPassword("alice", "correct-horse-battery")
	if _, _, err := e.LoginWithPassword("alice", "correct-horse-battery", ""); err != nil {
		t.Errorf("login blocked before 2FA was confirmed: %v", err)
	}
}

func TestEnable2FARequiresValidCode(t *testing.T) {
	e := newAuthTestEngine(t)
	e.Setup2FA("alice")

	if _, err := e.Enable2FA("alice", "000000"); err == nil {
		t.Fatal("2FA enabled with a wrong code")
	}
}

func TestFull2FALoginFlow(t *testing.T) {
	e := newAuthTestEngine(t)
	e.SetUserPassword("alice", "correct-horse-battery")
	secret, _, _ := e.Setup2FA("alice")

	code, _ := totpCode(secret, time.Now())
	recovery, err := e.Enable2FA("alice", code)
	if err != nil {
		t.Fatalf("Enable2FA: %v", err)
	}
	if len(recovery) != 8 {
		t.Fatalf("got %d recovery codes, want 8", len(recovery))
	}

	// Login without a code is rejected with a distinct error.
	_, _, err = e.LoginWithPassword("alice", "correct-horse-battery", "")
	if err == nil || !strings.Contains(err.Error(), "2fa_required") {
		t.Fatalf("expected 2fa_required error, got %v", err)
	}

	// Login with the current TOTP code succeeds...
	code, _ = totpCode(secret, time.Now())
	token, _, err := e.LoginWithPassword("alice", "correct-horse-battery", code)
	if err != nil {
		t.Fatalf("login with code: %v", err)
	}
	if token == "" {
		t.Fatal("no session token")
	}

	// ...but the SAME code cannot be replayed.
	if _, _, err := e.LoginWithPassword("alice", "correct-horse-battery", code); err == nil {
		t.Fatal("TOTP code replay accepted")
	}
}

func TestRecoveryCodeLogin(t *testing.T) {
	e := newAuthTestEngine(t)
	e.SetUserPassword("alice", "correct-horse-battery")
	secret, _, _ := e.Setup2FA("alice")
	code, _ := totpCode(secret, time.Now())
	recovery, _ := e.Enable2FA("alice", code)

	// A recovery code works in place of a TOTP code…
	if _, _, err := e.LoginWithPassword("alice", "correct-horse-battery", recovery[0]); err != nil {
		t.Fatalf("recovery login: %v", err)
	}
	// …exactly once.
	if _, _, err := e.LoginWithPassword("alice", "correct-horse-battery", recovery[0]); err == nil {
		t.Fatal("recovery code reuse accepted")
	}
	// Other codes still work.
	if _, _, err := e.LoginWithPassword("alice", "correct-horse-battery", recovery[1]); err != nil {
		t.Fatalf("second recovery code: %v", err)
	}
}

// Logging in with a code from the PREVIOUS drift window must advance the replay
// floor past the whole window, so a code from the current window can no longer
// be used. Guards against a regression that set the floor to just the matched
// step (leaving a replay gap inside the ±1 window).
func TestTOTPDriftWindowReplayClosed(t *testing.T) {
	e := newAuthTestEngine(t)
	e.SetUserPassword("alice", "correct-horse-battery")
	secret, _, _ := e.Setup2FA("alice")
	now := time.Now()
	enrollCode, _ := totpCode(secret, now)
	if _, err := e.Enable2FA("alice", enrollCode); err != nil {
		t.Fatalf("enable: %v", err)
	}

	// Log in with the previous-window code (clock-drift tolerance accepts it).
	prevCode, _ := totpCode(secret, now.Add(-30*time.Second))
	if _, _, err := e.LoginWithPassword("alice", "correct-horse-battery", prevCode); err != nil {
		t.Fatalf("previous-window login should succeed: %v", err)
	}
	// The current-window code must now be rejected — the floor advanced past it.
	curCode, _ := totpCode(secret, now)
	if curCode != prevCode {
		if _, _, err := e.LoginWithPassword("alice", "correct-horse-battery", curCode); err == nil {
			t.Fatal("current-window code accepted after a previous-window login (replay window open)")
		}
	}
}

func TestDisable2FA(t *testing.T) {
	e := newAuthTestEngine(t)
	e.SetUserPassword("alice", "correct-horse-battery")
	secret, _, _ := e.Setup2FA("alice")
	code, _ := totpCode(secret, time.Now())
	e.Enable2FA("alice", code)

	// Disabling requires a valid current code.
	if err := e.Disable2FA("alice", "000000"); err == nil {
		t.Fatal("2FA disabled with wrong code")
	}
	code, _ = totpCode(secret, time.Now().Add(30*time.Second))
	if err := e.Disable2FA("alice", code); err != nil {
		t.Fatalf("Disable2FA: %v", err)
	}
	// Login no longer needs a code.
	if _, _, err := e.LoginWithPassword("alice", "correct-horse-battery", ""); err != nil {
		t.Errorf("login after disable: %v", err)
	}
}
