package engine

import (
	"errors"
	"testing"
)

// validateEnvMap must reject a newline in a VALUE (not just a key) — that's the
// actual compose .env line-injection vector, and a prior test only exercised the
// key path so the value check could regress silently.
func TestValidateEnvMapRejectsValueNewline(t *testing.T) {
	if err := validateEnvMap(map[string]string{"VALID_KEY": "line1\nINJECTED=evil"}); err == nil {
		t.Error("value with a newline should be rejected")
	}
	if err := validateEnvMap(map[string]string{"VALID_KEY": "carriage\rreturn"}); err == nil {
		t.Error("value with a carriage return should be rejected")
	}
	if err := validateEnvMap(map[string]string{"GOOD": "fine"}); err != nil {
		t.Errorf("clean env should pass: %v", err)
	}
}

// DestroySite must reject an invalid domain BEFORE it can reach os.RemoveAll
// (with purge). A regression that moved the guard would let "../.." escape.
func TestDestroySiteRejectsBadDomainBeforeDeleting(t *testing.T) {
	e := &Engine{locks: NewLockManager()}
	for _, bad := range []string{"../../etc", "..", "Example.com", "a/b"} {
		if err := e.DestroySite(t.Context(), bad, true); err == nil {
			t.Errorf("DestroySite(%q) = nil, want validation error", bad)
		}
	}
}

// SiteActivity must reflect exactly what the lock manager holds, so the panel's
// "why is it locked" banner is truthful.
func TestSiteActivityReflectsLock(t *testing.T) {
	e := &Engine{locks: NewLockManager()}

	if info := e.SiteActivity("ex.com"); info.Held {
		t.Fatalf("idle site should not be held: %+v", info)
	}

	if err := e.locks.Acquire("ex.com", "deploying"); err != nil {
		t.Fatal(err)
	}
	info := e.SiteActivity("ex.com")
	if !info.Held || info.Operation != "deploying" {
		t.Errorf("held activity = %+v, want held=true operation=deploying", info)
	}
	if info.Since.IsZero() {
		t.Error("a held lock should carry a start time")
	}

	e.locks.Release("ex.com")
	if info := e.SiteActivity("ex.com"); info.Held {
		t.Errorf("released site should be idle: %+v", info)
	}
}

// beginOp/finishOp are the streaming entry points every non-deploy operation
// (clone, destroy, backup, restore) now uses. A subscriber must see a fresh
// run that ends with a clean terminal event.
func TestBeginFinishOpStreamSuccess(t *testing.T) {
	e := &Engine{}

	e.beginOp("ex.com", "Backing up")
	e.finishOp("ex.com", "Backup complete", "backup #1 created", nil)

	replay, _, cancel := e.SubscribeProgress("ex.com")
	cancel()
	if len(replay) < 2 {
		t.Fatalf("want a begin + terminal event, got %d: %+v", len(replay), replay)
	}
	if replay[0].Step != "Backing up" {
		t.Errorf("first step = %q, want %q", replay[0].Step, "Backing up")
	}
	last := replay[len(replay)-1]
	if last.Status != "done" || last.Percent != 100 || !last.Terminal() {
		t.Errorf("terminal event = %+v, want done/100/terminal", last)
	}
}

func TestFinishOpErrorIsTerminalAndSanitized(t *testing.T) {
	e := &Engine{}

	// A second op on the same domain must start a fresh stream (Begin clears the
	// prior run's buffer).
	e.beginOp("ex.com", "Restoring backup")
	e.finishOp("ex.com", "Restored", "", errors.New("boom\nstack trace with secrets"))

	replay, _, cancel := e.SubscribeProgress("ex.com")
	cancel()
	if len(replay) == 0 {
		t.Fatal("expected events after begin/finish")
	}
	last := replay[len(replay)-1]
	if last.Status != "error" || !last.Terminal() {
		t.Errorf("terminal event = %+v, want error/terminal", last)
	}
	// Only the first line is surfaced — no multi-line stack/secret leakage.
	if last.Detail == "" || last.Detail != "boom" {
		t.Errorf("error detail = %q, want sanitized first line %q", last.Detail, "boom")
	}
}
