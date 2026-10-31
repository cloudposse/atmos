package local

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/gofrs/flock"

	"github.com/cloudposse/atmos/pkg/artifact/blob"
)

const lockRetry = 10 * time.Millisecond

// rootMutexes holds one in-process mutex per root so separate Store values over one directory
// serialize with each other without relying on the file lock alone.
var rootMutexes sync.Map

func rootMutex(root string) *sync.Mutex {
	key := root
	if abs, err := filepath.Abs(root); err == nil {
		key = abs
	}
	mutex, _ := rootMutexes.LoadOrStore(key, &sync.Mutex{})
	return mutex.(*sync.Mutex)
}

// lockRoot takes the per-root lock (in-process mutex, then OS file lock) and returns its release.
func (s *Store) lockRoot(ctx context.Context, op, key string) (func(), error) {
	mutex := rootMutex(s.root)
	mutex.Lock()
	if err := ctx.Err(); err != nil {
		mutex.Unlock()
		return nil, blob.NewError(blob.KindOther, op, key, err)
	}
	if err := mkdirAll(s.root, s.dirPerm); err != nil {
		mutex.Unlock()
		return nil, blob.NewError(blob.KindOther, op, key, fmt.Errorf("create lock directory: %w", err))
	}
	fileLock := flock.New(filepath.Join(s.root, lockName))
	if _, err := fileLock.TryLockContext(ctx, lockRetry); err != nil {
		mutex.Unlock()
		return nil, blob.NewError(blob.KindOther, op, key, fmt.Errorf("acquire root lock: %w", err))
	}
	return func() {
		_ = fileLock.Unlock()
		mutex.Unlock()
	}, nil
}

// guard returns the lock release for a write or delete, or a no-op when no lock is needed.
func (s *Store) guard(ctx context.Context, op, key string, conditional bool) (func(), error) {
	if !conditional && !s.lockAll {
		return func() {}, nil
	}
	return s.lockRoot(ctx, op, key)
}
