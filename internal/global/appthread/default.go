package appthread

import "sync"

var (
	defaultMu   sync.RWMutex
	defaultPool *Pool
)

// SetDefault installs the process-wide pool returned by Default.
//
// The composition root calls this once during startup so that code paths without an injected
// pool still share the same bounded concurrency budget. It returns the previously installed pool.
func SetDefault(pool *Pool) *Pool {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	previous := defaultPool
	defaultPool = pool
	return previous
}

// Default returns the process-wide pool, lazily creating a conservative fallback if unset.
//
// Prefer injecting a *Pool where ownership matters; Default exists for leaf code that cannot
// reasonably thread a pool through its call chain.
func Default() *Pool {
	defaultMu.RLock()
	pool := defaultPool
	defaultMu.RUnlock()
	if pool != nil {
		return pool
	}
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultPool == nil {
		defaultPool = NewPool(Options{Name: "global"})
	}
	return defaultPool
}
