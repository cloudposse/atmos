// Package cache provides generic file-based caching with cross-process locking.
package cache

import (
	"context"
	"errors"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filelock"
	"github.com/cloudposse/atmos/pkg/perf"
)

// FileLock provides file-level locking for cache operations.
type FileLock interface {
	// WithLock executes fn while holding an exclusive lock.
	// It uses a bounded, cross-process exclusive lock.
	WithLock(fn func() error) error

	// WithLockContext executes fn while holding an exclusive lock, blocking until
	// the lock is acquired or ctx is done. Unlike WithLock's bounded retry, it
	// waits the full duration of whatever the holder is doing (e.g. a multi-second
	// download), making it suitable for collapsing a herd around a slow operation.
	// It uses the same real cross-process lock on every supported platform.
	WithLockContext(ctx context.Context, fn func() error) error

	// WithRLock executes fn while holding a shared read lock.
	// It uses a cross-process shared read lock when immediately available.
	WithRLock(fn func() error) error
}

const cacheLockTimeout = 2 * time.Second

type flockFileLock struct{ lockPath string }

// NewFileLock preserves the cache package API while using the shared,
// cross-platform locking implementation. The stable sibling lock file is
// intentionally retained across atomic file replacement.
func NewFileLock(path string) FileLock {
	defer perf.Track(nil, "cache.NewFileLock")()

	return &flockFileLock{lockPath: path + ".lock"}
}

func (l *flockFileLock) WithLock(fn func() error) error {
	defer perf.Track(nil, "cache.flockFileLock.WithLock")()

	ctx, cancel := context.WithTimeout(context.Background(), cacheLockTimeout)
	defer cancel()
	return cacheLockError(filelock.New(l.lockPath).WithExclusive(ctx, fn))
}

func (l *flockFileLock) WithLockContext(ctx context.Context, fn func() error) error {
	defer perf.Track(nil, "cache.flockFileLock.WithLockContext")()

	return cacheLockError(filelock.New(l.lockPath).WithExclusive(ctx, fn))
}

func (l *flockFileLock) WithRLock(fn func() error) error {
	defer perf.Track(nil, "cache.flockFileLock.WithRLock")()

	// Cache reads are deliberately best-effort. Preserve the old non-blocking
	// behavior by reading without a lock when one is immediately unavailable.
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	err := filelock.New(l.lockPath).WithShared(ctx, fn)
	if errors.Is(err, filelock.ErrAcquire) && ctx.Err() != nil {
		return fn()
	}
	return cacheLockError(err)
}

func cacheLockError(err error) error {
	if errors.Is(err, filelock.ErrAcquire) {
		return errors.Join(errUtils.ErrCacheLocked, err)
	}
	return err
}
