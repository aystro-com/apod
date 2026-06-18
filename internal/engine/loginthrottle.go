package engine

import (
	"sync"
	"time"
)

// Login lockout parameters. After maxLoginFailures failed attempts within the
// tracking window, an account is locked for lockoutDuration. Tuned to slow
// online brute force without trivially letting an attacker lock a victim out
// for long.
const (
	maxLoginFailures = 10
	lockoutDuration  = 15 * time.Minute
	failureWindow    = 15 * time.Minute
)

type loginAttempt struct {
	failures    int
	firstFail   time.Time
	lockedUntil time.Time
}

// loginThrottle tracks failed login attempts per account name in memory. The
// key is the submitted name (whether or not it exists), so a locked response is
// identical for real and bogus usernames and cannot be used to enumerate
// accounts.
type loginThrottle struct {
	mu  sync.Mutex
	by  map[string]*loginAttempt
	now func() time.Time
}

func newLoginThrottle() *loginThrottle {
	return &loginThrottle{by: make(map[string]*loginAttempt), now: time.Now}
}

// Locked reports whether the account is currently locked out.
func (t *loginThrottle) Locked(name string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	a := t.by[name]
	return a != nil && t.now().Before(a.lockedUntil)
}

// RecordFailure records a failed attempt and locks the account once the
// threshold is reached within the tracking window.
func (t *loginThrottle) RecordFailure(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	a := t.by[name]
	if a == nil || now.Sub(a.firstFail) > failureWindow {
		a = &loginAttempt{firstFail: now}
		t.by[name] = a
	}
	a.failures++
	if a.failures >= maxLoginFailures {
		a.lockedUntil = now.Add(lockoutDuration)
	}
	t.gc(now)
}

// Reset clears the record after a successful login.
func (t *loginThrottle) Reset(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.by, name)
}

// gc drops stale entries so the map can't grow without bound from random
// usernames. Caller holds the lock.
func (t *loginThrottle) gc(now time.Time) {
	for k, a := range t.by {
		if now.After(a.lockedUntil) && now.Sub(a.firstFail) > failureWindow {
			delete(t.by, k)
		}
	}
}
