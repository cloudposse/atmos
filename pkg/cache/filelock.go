// Package cache provides generic file-based caching with platform-appropriate locking.
package cache

import "context"

// FileLock provides file-level locking for cache operations.
type FileLock interface {
	// WithLock executes fn while holding an exclusive lock where supported.
	// Windows cache operations execute best-effort without native file locking.
	WithLock(fn func() error) error

	// WithLockContext executes fn while holding an exclusive lock, blocking until
	// the lock is acquired or ctx is done. Unlike WithLock's bounded retry, it
	// waits the full duration of whatever the holder is doing (e.g. a multi-second
	// download), making it suitable for collapsing a herd around a slow operation.
	// Windows cache operations execute best-effort without native file locking.
	WithLockContext(ctx context.Context, fn func() error) error

	// WithRLock executes fn while holding a shared read lock where supported.
	// Windows cache reads execute best-effort without native file locking.
	WithRLock(fn func() error) error

	// TryWithRLock executes fn while holding a shared read lock when it can be
	// acquired quickly. It returns false, nil when another process holds an
	// exclusive lock. On Windows, it executes without locking and returns true.
	TryWithRLock(fn func() error) (acquired bool, err error)
}
