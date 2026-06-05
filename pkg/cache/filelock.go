// Package cache provides generic file-based caching with platform-specific locking.
package cache

// FileLock provides file-level locking for cache operations.
// Implementations are platform-specific to handle differences between
// Unix (using flock) and Windows (graceful degradation).
type FileLock interface {
	// WithLock executes fn while holding an exclusive lock.
	// On Unix, this uses flock with retry logic.
	// On Windows, this executes without locking (graceful degradation).
	WithLock(fn func() error) error

	// WithRLock executes fn while holding a shared read lock.
	// On Unix, this uses flock with read lock.
	// On Windows, this executes without locking (graceful degradation).
	WithRLock(fn func() error) error
}
