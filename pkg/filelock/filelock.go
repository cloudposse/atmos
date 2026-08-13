// Package filelock provides cross-process advisory file locks for mutable files
// that are updated with a read-modify-write transaction.
package filelock

import (
	"context"
	"errors"
	"fmt"
	"sync"
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

// processLocks closes a platform-specific gap in advisory file locks: some
// operating systems associate a lock with the process rather than a file
// descriptor, so two handles opened by separate goroutines in the *same*
// process can both appear to acquire it. OS locks still protect other
// processes; this coordinator supplies the missing local reader/writer
// exclusion before taking that OS lock.
var processLocks sync.Map // map[string]*processLock

type processLock struct {
	mu      sync.Mutex
	readers int
	writer  bool
	waiters []*processLockWaiter
}

type processLockWaiter struct {
	shared  bool
	granted bool
	done    chan struct{}
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
	releaseProcess, processErr := acquireProcessLock(ctx, l.path, shared)
	if processErr != nil {
		return fmt.Errorf("%w: %w", ErrAcquire, processErr)
	}
	defer releaseProcess()

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

func acquireProcessLock(ctx context.Context, path string, shared bool) (func(), error) {
	value, _ := processLocks.LoadOrStore(path, &processLock{})
	lock := value.(*processLock)
	waiter := &processLockWaiter{shared: shared, done: make(chan struct{})}

	lock.mu.Lock()
	lock.waiters = append(lock.waiters, waiter)
	lock.grantWaiters()
	lock.mu.Unlock()

	select {
	case <-waiter.done:
		return func() { lock.release(shared) }, nil
	case <-ctx.Done():
		lock.mu.Lock()
		if waiter.granted {
			lock.releaseLocked(shared)
		} else {
			for i, candidate := range lock.waiters {
				if candidate == waiter {
					lock.waiters = append(lock.waiters[:i], lock.waiters[i+1:]...)
					break
				}
			}
			lock.grantWaiters()
		}
		lock.mu.Unlock()
		return nil, ctx.Err()
	}
}

func (l *processLock) release(shared bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.releaseLocked(shared)
}

func (l *processLock) releaseLocked(shared bool) {
	if shared {
		l.readers--
	} else {
		l.writer = false
	}
	l.grantWaiters()
}

func (l *processLock) grantWaiters() {
	if l.writer || len(l.waiters) == 0 {
		return
	}
	if !l.waiters[0].shared {
		if l.readers != 0 {
			return
		}
		waiter := l.waiters[0]
		l.waiters = l.waiters[1:]
		l.writer = true
		waiter.granted = true
		close(waiter.done)
		return
	}
	for len(l.waiters) > 0 && l.waiters[0].shared {
		waiter := l.waiters[0]
		l.waiters = l.waiters[1:]
		l.readers++
		waiter.granted = true
		close(waiter.done)
	}
}
