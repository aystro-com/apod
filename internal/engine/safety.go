package engine

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// detachCtx returns a context that survives the caller's cancellation, bounded
// by a timeout. Used by container-lifecycle operations that may stop the very
// container serving the request (the apod-ui panel): the web client's
// connection drops the moment we stop it, cancelling the request context, but
// the operation must still finish so the container comes back.
func detachCtx(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), d)
}

// lockState records what a site is currently busy with, so a blocked caller
// (and the UI) can see exactly which operation holds the lock and for how long
// — instead of an opaque "busy" error.
type lockState struct {
	operation string
	since     time.Time
}

// LockInfo is the public, serialisable view of a held lock.
type LockInfo struct {
	Operation string    `json:"operation"` // human-readable, e.g. "deploying"
	Since     time.Time `json:"since"`     // when the operation started
	Held      bool      `json:"held"`      // false ⇒ the site is idle
}

type LockManager struct {
	mu    sync.Mutex
	locks map[string]lockState
}

func NewLockManager() *LockManager {
	return &LockManager{locks: make(map[string]lockState)}
}

// Acquire takes the per-domain lock, tagging it with a human-readable operation
// label and the time it started. operation is what the UI shows ("deploying",
// "restarting", …) and what a blocked caller is told it's waiting on.
func (lm *LockManager) Acquire(domain, operation string) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	if st, held := lm.locks[domain]; held {
		return Conflict("%s is busy: %s (started %s ago) — try again in a moment",
			domain, st.operation, compactDuration(time.Since(st.since)))
	}
	lm.locks[domain] = lockState{operation: operation, since: time.Now()}
	return nil
}

func (lm *LockManager) Release(domain string) {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	delete(lm.locks, domain)
}

// Info reports the operation currently holding a domain's lock, if any.
func (lm *LockManager) Info(domain string) LockInfo {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	st, held := lm.locks[domain]
	return LockInfo{Operation: st.operation, Since: st.since, Held: held}
}

// SiteActivity reports what operation, if any, currently holds a site's lock.
// The UI polls this to show a live "what's this site busy with" indicator and
// to explain a "site is busy" error instead of leaving it opaque.
func (e *Engine) SiteActivity(domain string) LockInfo {
	return e.locks.Info(domain)
}

// compactDuration renders a short, human "2m3s" / "12s" style elapsed time.
func compactDuration(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
