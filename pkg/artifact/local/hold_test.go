package local

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
)

// holdLock takes a repository lock for a test. The lock is also released when the test ends, so an
// assertion that aborts the test never leaves the resource held. Releasing it explicitly first is
// fine: the cleanup's second release only reports that the handle is gone, and that is ignored.
func holdLock(t *testing.T, repo *Repository, request artifact.LockRequest) artifact.LockHandle {
	t.Helper()
	handle, err := repo.Lock(context.Background(), request)
	require.NoError(t, err)
	t.Cleanup(func() { _ = handle.Release(context.Background()) })
	return handle
}

func TestHoldLockReleasesWhenTheTestEnds(t *testing.T) {
	repo := newRepo(t, dirSpec())

	t.Run("a subtest that never releases", func(t *testing.T) {
		_ = holdLock(t, repo, lockRequest("r"))
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	next, err := repo.Lock(ctx, lockRequest("r"))
	require.NoError(t, err, "the lock a finished subtest still held is free for the next one")
	require.NoError(t, next.Release(context.Background()))
}
