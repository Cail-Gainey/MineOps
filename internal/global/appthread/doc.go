// Package appthread provides the application-wide bounded goroutine pool used by MineOps.
//
// It is the single "thread management" boundary: every ad-hoc background goroutine and
// every concurrent fan-out (for example the download source speed test) should run through
// a Pool so that total concurrency stays bounded, panics are converted into structured
// crash logs instead of crashing the process, and shutdown can wait for in-flight work.
//
// A process-wide Pool is available through Default; scoped Pools can be created with NewPool
// and injected where deterministic ownership and testing are preferred.
package appthread
