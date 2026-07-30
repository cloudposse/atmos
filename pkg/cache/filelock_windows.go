//go:build windows

package cache

import (
	"context"
	"time"
)

// noopFileLock keeps cache operations best-effort on Windows. Native Windows
// file locks have historically caused cache-operation timeouts; the cache is
// non-critical and must not block the primary command path.
type noopFileLock struct{}

// NewFileLock returns the Windows best-effort cache lock implementation.
func NewFileLock(_ string) FileLock {
	return &noopFileLock{}
}

// WithLock executes fn without a native lock and gives Windows time to release
// file handles before another cache operation starts.
func (n *noopFileLock) WithLock(fn func() error) error {
	defer time.Sleep(50 * time.Millisecond)

	return fn()
}

// WithLockContext executes fn without a native lock. Cache writes are
// intentionally best-effort on Windows, including when ctx is canceled.
func (n *noopFileLock) WithLockContext(_ context.Context, fn func() error) error {
	defer time.Sleep(50 * time.Millisecond)

	return fn()
}

// WithRLock executes cache reads without a native lock on Windows.
func (n *noopFileLock) WithRLock(fn func() error) error {
	return fn()
}
