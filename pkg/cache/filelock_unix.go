//go:build !windows

package cache

import (
	"context"
	"errors"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filelock"
	"github.com/cloudposse/atmos/pkg/perf"
)

const cacheLockTimeout = 2 * time.Second

// tryReadLockTimeout bounds the "best-effort, non-blocking" read-lock attempts
// in WithRLock and TryWithRLock. It must stay well under cacheLockTimeout so
// these still fail fast under genuine contention, but 1ms (the original
// value) was tight enough that ordinary scheduling/IO jitter on a loaded CI
// runner -- not actual lock contention -- could make an uncontended
// acquisition miss its deadline, which TryWithRLock's callers (e.g. workdir
// metadata reads) treat identically to "lock held by another process."
// 50ms comfortably exceeds filelock's internal retryDelay (10ms), so a
// briefly-held exclusive lock also has a real chance to clear within budget.
const tryReadLockTimeout = 50 * time.Millisecond

type flockFileLock struct{ lockPath string }

// NewFileLock preserves the cache package API while using a stable sibling
// lock file that survives atomic cache-file replacement.
func NewFileLock(path string) FileLock {
	defer perf.Track(nil, "cache.NewFileLock")()

	return NewFileLockAtPath(path + ".lock")
}

// NewFileLockAtPath creates a FileLock that uses lockPath directly. Use this
// when compatibility requires a lock name other than targetPath + ".lock".
func NewFileLockAtPath(lockPath string) FileLock {
	defer perf.Track(nil, "cache.NewFileLockAtPath")()

	return &flockFileLock{lockPath: lockPath}
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
	// behavior by reading without a lock when one isn't quickly available.
	ctx, cancel := context.WithTimeout(context.Background(), tryReadLockTimeout)
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

// TryWithRLock executes fn only when a shared read lock can be acquired
// quickly (see tryReadLockTimeout).
func (f *flockFileLock) TryWithRLock(fn func() error) (bool, error) {
	defer perf.Track(nil, "cache.flockFileLock.TryWithRLock")()

	ctx, cancel := context.WithTimeout(context.Background(), tryReadLockTimeout)
	defer cancel()
	err := filelock.New(f.lockPath).WithShared(ctx, fn)
	if errors.Is(err, filelock.ErrAcquire) {
		if ctx.Err() != nil {
			return false, nil
		}
		return false, cacheLockError(err)
	}
	return true, err
}
