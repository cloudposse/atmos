//go:build !windows

package cache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/gofrs/flock"
)

const (
	// MaxLockRetries is the number of times to retry acquiring a lock.
	maxLockRetries = 50
	// LockRetryDelay is the delay between lock retry attempts.
	lockRetryDelay = 10 * time.Millisecond
)

// flockFileLock implements FileLock using flock on Unix systems.
//
// The flock syscall coordinates exclusion across processes, but goroutines within
// the same process contend on it through independent file descriptors with no
// fairness guarantee: the non-blocking TryLock polling in WithLock can starve an
// unlucky goroutine until its bounded retry budget is exhausted. The in-process mu
// serializes goroutines fairly (Go's RWMutex prevents writer starvation) before
// any of them touch the cross-process flock, so flock is never contended from
// within this process. Cross-process coordination still flows through flock.
type flockFileLock struct {
	lockPath string
	mu       sync.RWMutex
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

	// Serialize in-process goroutines fairly before contending on the cross-process
	// flock, so the bounded TryLock retry below never has to compete with our own
	// goroutines (which would otherwise risk starvation under load).
	f.mu.Lock()
	defer f.mu.Unlock()

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

	// Serialize in-process goroutines fairly before contending on the cross-process
	// flock (see WithLock for rationale).
	f.mu.Lock()
	defer f.mu.Unlock()

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

	// Hold the in-process read lock so concurrent readers run together while still
	// excluding in-process writers (see WithLock for rationale).
	f.mu.RLock()
	defer f.mu.RUnlock()

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
