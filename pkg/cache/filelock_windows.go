//go:build windows

package cache

import (
	"time"
)

// noopFileLock implements FileLock without actual locking for Windows.
// Windows file locking has timeout issues, so we gracefully degrade
// to no locking since the cache is non-critical functionality.
type noopFileLock struct{}

// NewFileLock creates a new FileLock for the given path.
// On Windows, this returns a no-op implementation.
func NewFileLock(_ string) FileLock {
	return &noopFileLock{}
}

// WithLock executes fn without locking on Windows.
func (n *noopFileLock) WithLock(fn func() error) error {
	// Add a small delay after operations to let Windows release file handles.
	defer func() {
		time.Sleep(50 * time.Millisecond)
	}()

	return fn()
}

// WithRLock executes fn without locking on Windows.
func (n *noopFileLock) WithRLock(fn func() error) error {
	return fn()
}
