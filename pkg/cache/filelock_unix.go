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

type flockFileLock struct{ lockPath string }

// NewFileLock preserves the cache package API while using a stable sibling
// lock file that survives atomic cache-file replacement.
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
