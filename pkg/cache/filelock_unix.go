//go:build !windows

package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/gofrs/flock"
)

const (
	// MaxLockRetries is the number of times to retry acquiring a lock. At
	// lockRetryDelay between attempts, this gives a 2s total budget — enough
	// slack for legitimate contention (e.g. several goroutines calling
	// Set/Delete against the same cache directory's single lock file
	// concurrently) on a loaded CI runner without hanging indefinitely on a
	// genuinely stuck/orphaned lock.
	maxLockRetries = 200
	// LockRetryDelay is the delay between lock retry attempts.
	lockRetryDelay = 10 * time.Millisecond
)

// flockFileLock implements FileLock using flock on Unix systems.
type flockFileLock struct {
	lockPath string
}

// NewFileLock creates a new FileLock for the given path.
// The lock file is created at path + ".lock" to prevent lock loss during atomic renames.
func NewFileLock(path string) FileLock {
	defer perf.Track(nil, "cache.NewFileLock")()

	return &flockFileLock{
		lockPath: path + ".lock",
	}
}

// WithLock executes fn while holding an exclusive lock.
func (f *flockFileLock) WithLock(fn func() error) error {
	defer perf.Track(nil, "cache.flockFileLock.WithLock")()

	lock := flock.New(f.lockPath)

	// Try to acquire lock with reasonable retries for concurrent access.
	var locked bool
	var err error

	for i := 0; i < maxLockRetries; i++ {
		locked, err = lock.TryLock()
		if err != nil {
			return errors.Join(errUtils.ErrCacheLocked, err)
		}
		if locked {
			break
		}
		// Wait a short time before retrying.
		time.Sleep(lockRetryDelay)
	}

	if !locked {
		return fmt.Errorf("%w: cache file is locked by another process", errUtils.ErrCacheLocked)
	}

	defer func() {
		if err := lock.Unlock(); err != nil {
			log.Trace("Failed to unlock cache file", "error", err, "path", f.lockPath)
		}
	}()

	return fn()
}

// WithLockContext executes fn while holding an exclusive lock, blocking until the
// lock is acquired or ctx is done. Acquisition polls at lockRetryDelay intervals for
// the full lifetime of ctx, so a healthy but slow holder (e.g. a multi-second
// download) is waited out rather than failing on a fixed budget.
func (f *flockFileLock) WithLockContext(ctx context.Context, fn func() error) error {
	defer perf.Track(nil, "cache.flockFileLock.WithLockContext")()

	lock := flock.New(f.lockPath)

	locked, err := lock.TryLockContext(ctx, lockRetryDelay)
	if err != nil {
		return errors.Join(errUtils.ErrCacheLocked, err)
	}
	if !locked {
		// ctx was canceled or its deadline passed before the lock was acquired.
		return fmt.Errorf("%w: cache file is locked by another process", errUtils.ErrCacheLocked)
	}

	defer func() {
		if uerr := lock.Unlock(); uerr != nil {
			log.Trace("Failed to unlock cache file", "error", uerr, "path", f.lockPath)
		}
	}()

	return fn()
}

// WithRLock executes fn while holding a shared read lock.
func (f *flockFileLock) WithRLock(fn func() error) error {
	defer perf.Track(nil, "cache.flockFileLock.WithRLock")()

	lock := flock.New(f.lockPath)

	// Use TryRLock to avoid blocking indefinitely which can cause deadlocks.
	locked, err := lock.TryRLock()
	if err != nil {
		return errors.Join(errUtils.ErrCacheLocked, err)
	}
	if !locked {
		// If we can't get the lock immediately, return without error.
		// This prevents deadlocks during concurrent access.
		// The caller should handle the case where fn wasn't executed.
		return fn() // Execute without lock - cache is non-critical.
	}

	defer func() {
		if err := lock.Unlock(); err != nil {
			log.Trace("Failed to unlock cache file during read", "error", err, "path", f.lockPath)
		}
	}()

	return fn()
}
