package engine

import (
	"fmt"
	"sync"
)

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
