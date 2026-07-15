// Package filelock provides cross-process advisory file locks for mutable files
// that are updated with a read-modify-write transaction.
package filelock

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gofrs/flock"

	"github.com/cloudposse/atmos/pkg/perf"
)

// ErrAcquire indicates that a lock could not be acquired before the context
// was cancelled or its deadline elapsed.
var ErrAcquire = errors.New("file lock acquisition failed")

const retryDelay = 10 * time.Millisecond

// Lock is a cross-process advisory lock. Its path is deliberately stable and
// is never removed: callers normally use a sibling .lock file, which remains
// valid even when the protected file is atomically replaced.
type Lock struct {
	path string
}

// New creates a lock backed by path. The path is usually the protected path
// with a .lock suffix.
func New(path string) *Lock {
	defer perf.Track(nil, "filelock.New")()

	return &Lock{path: path}
}

// Path returns the lock file path.
func (l *Lock) Path() string {
	defer perf.Track(nil, "filelock.Lock.Path")()

	return l.path
}

// WithExclusive runs fn while holding an exclusive lock, or returns ErrAcquire
// when ctx finishes first. Locks are released even when fn returns an error.
func (l *Lock) WithExclusive(ctx context.Context, fn func() error) error {
	defer perf.Track(nil, "filelock.Lock.WithExclusive")()

	return l.with(ctx, false, fn)
}

// WithShared runs fn while holding a shared lock, or returns ErrAcquire when
// ctx finishes first.
func (l *Lock) WithShared(ctx context.Context, fn func() error) error {
	defer perf.Track(nil, "filelock.Lock.WithShared")()

	return l.with(ctx, true, fn)
}

func (l *Lock) with(ctx context.Context, shared bool, fn func() error) error {
	fl := flock.New(l.path)
	var (
		locked bool
		err    error
	)
	if shared {
		locked, err = fl.TryRLockContext(ctx, retryDelay)
	} else {
		locked, err = fl.TryLockContext(ctx, retryDelay)
	}
	if err != nil {
		return fmt.Errorf("%w: %w", ErrAcquire, err)
	}
	if !locked {
		return fmt.Errorf("%w: %s", ErrAcquire, l.path)
	}
	defer func() { _ = fl.Unlock() }()
	return fn()
}
