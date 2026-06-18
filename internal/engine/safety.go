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

type LockManager struct {
	mu    sync.Mutex
	locks map[string]bool
}

func NewLockManager() *LockManager {
	return &LockManager{locks: make(map[string]bool)}
}

func (lm *LockManager) Acquire(domain string) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	if lm.locks[domain] {
		return fmt.Errorf("site %q is locked by another operation", domain)
	}
	lm.locks[domain] = true
	return nil
}

func (lm *LockManager) Release(domain string) {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	delete(lm.locks, domain)
}
