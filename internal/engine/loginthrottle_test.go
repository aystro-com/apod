package engine

import (
	"testing"
	"time"
)

func TestLoginThrottleLocksAndUnlocks(t *testing.T) {
	now := time.Now()
	tr := newLoginThrottle()
	tr.now = func() time.Time { return now }

	for i := 0; i < maxLoginFailures-1; i++ {
		tr.RecordFailure("alice")
	}
	if tr.Locked("alice") {
		t.Fatalf("locked too early after %d failures", maxLoginFailures-1)
	}
	tr.RecordFailure("alice") // hits threshold
	if !tr.Locked("alice") {
		t.Fatal("expected lock at threshold")
	}

	// Still locked just before the window elapses.
	now = now.Add(lockoutDuration - time.Second)
	if !tr.Locked("alice") {
		t.Fatal("should remain locked within lockout window")
	}
	// Unlocked after the window.
	now = now.Add(2 * time.Second)
	if tr.Locked("alice") {
		t.Fatal("should be unlocked after lockout window")
	}
}

func TestLoginThrottleResetOnSuccess(t *testing.T) {
	tr := newLoginThrottle()
	for i := 0; i < maxLoginFailures-1; i++ {
		tr.RecordFailure("bob")
	}
	tr.Reset("bob")
	tr.RecordFailure("bob")
	if tr.Locked("bob") {
		t.Fatal("reset should have cleared prior failures")
	}
}

func TestLoginThrottleIsolatesAccounts(t *testing.T) {
	tr := newLoginThrottle()
	for i := 0; i < maxLoginFailures; i++ {
		tr.RecordFailure("victim")
	}
	if tr.Locked("other") {
		t.Fatal("one account's failures must not lock another")
	}
}
