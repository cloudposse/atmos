// Package cache provides generic file-based caching with platform-specific locking.
package cache

import "context"

// FileLock provides file-level locking for cache operations.
// Implementations are platform-specific to handle differences between
// Unix (using flock) and Windows (graceful degradation).
type FileLock interface {
	// WithLock executes fn while holding an exclusive lock.
	// On Unix, this uses flock with bounded retry logic.
	// On Windows, this executes without locking (graceful degradation).
	WithLock(fn func() error) error

	// WithLockContext executes fn while holding an exclusive lock, blocking until
	// the lock is acquired or ctx is done. Unlike WithLock's bounded retry, it
	// waits the full duration of whatever the holder is doing (e.g. a multi-second
	// download), making it suitable for collapsing a herd around a slow operation.
	// On Windows, this executes without locking (graceful degradation).
	WithLockContext(ctx context.Context, fn func() error) error

	// WithRLock executes fn while holding a shared read lock.
	// On Unix, this uses flock with read lock.
	// On Windows, this executes without locking (graceful degradation).
	WithRLock(fn func() error) error

	// TryWithRLock executes fn while holding a shared read lock when it can be
	// acquired immediately. It returns false, nil when another process holds an
	// exclusive lock. On Windows, it executes without locking and returns true.
	TryWithRLock(fn func() error) (acquired bool, err error)
}
