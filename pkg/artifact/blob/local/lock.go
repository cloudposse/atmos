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

func rootMutex(root string) chan struct{} {
	key := root
	if abs, err := filepath.Abs(root); err == nil {
		key = abs
	}
	mutex, _ := rootMutexes.LoadOrStore(key, make(chan struct{}, 1))
	return mutex.(chan struct{})
}

// lockRoot takes the per-root lock (in-process mutex, then OS file lock) and returns its release.
func (s *Store) lockRoot(ctx context.Context, op, key string) (func(), error) {
	mutex := rootMutex(s.root)
	select {
	case mutex <- struct{}{}:
	case <-ctx.Done():
		return nil, blob.NewError(blob.KindOther, op, key, ctx.Err())
	}
	if err := ctx.Err(); err != nil {
		<-mutex
		return nil, blob.NewError(blob.KindOther, op, key, err)
	}
	if err := mkdirAll(s.root, s.dirPerm); err != nil {
		<-mutex
		return nil, blob.NewError(blob.KindOther, op, key, fmt.Errorf("create lock directory: %w", err))
	}
	fileLock := flock.New(filepath.Join(s.root, lockName))
	if _, err := fileLock.TryLockContext(ctx, lockRetry); err != nil {
		<-mutex
		return nil, blob.NewError(blob.KindOther, op, key, fmt.Errorf("acquire root lock: %w", err))
	}
	return func() {
		_ = fileLock.Unlock()
		<-mutex
	}, nil
}

// guard returns the lock release for a write or delete, or a no-op when no lock is needed.
func (s *Store) guard(ctx context.Context, op, key string, conditional bool) (func(), error) {
	if s.plainFiles && !conditional && !s.lockAll {
		return func() {}, nil
	}
	release, err := s.lockRoot(ctx, op, key)
	if err != nil {
		return nil, err
	}
	if !s.plainFiles {
		root, openErr := s.openRoot(false)
		if openErr == nil {
			openErr = s.recoverCommit(root)
			_ = root.Close()
		}
		if openErr != nil {
			release()
			return nil, wrapIO(op, key, openErr)
		}
	}
	return release, nil
}
