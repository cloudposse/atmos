package azure

import (
	"context"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMSALCacheLock_LockFailureRemovesOwnClaim verifies that when the lock cannot be acquired
// after this process created the lock file (here the context is already canceled), the claim is
// removed again. A leftover lock file would make the next az command wait about 5 seconds.
func TestMSALCacheLock_LockFailureRemovesOwnClaim(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows FileLock is a no-op and never fails to lock")
	}
	_, cachePath := newAzureDir(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := withMSALCacheFileLock(ctx, cachePath, func() error {
		t.Fatal("the callback must not run without the lock")
		return nil
	})

	require.ErrorIs(t, err, ErrFileLockTimeout)
	assertNoMSALLockfile(t, cachePath)
}

// TestMSALCacheLock_LockFailureKeepsForeignLockfile is the negative path: a lock file this
// process did not create (taken over as stale) is not removed when the lock fails.
func TestMSALCacheLock_LockFailureKeepsForeignLockfile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows FileLock is a no-op and never fails to lock")
	}
	_, cachePath := newAzureDir(t)
	lockPath := AzureCLIMSALCacheLockPath(cachePath)
	require.NoError(t, os.WriteFile(lockPath, []byte("12345 az"), 0o600))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := withMSALCacheFileLock(ctx, cachePath, func() error { return nil })

	require.ErrorIs(t, err, ErrFileLockTimeout)
	_, statErr := os.Stat(lockPath)
	require.NoError(t, statErr, "a lock file another process created is left alone")
}

// TestMSALLockfileClaim_AbandonOnlyRemovesTheSameFile verifies a claim is removed only while the
// path still names the file this process created, never a file another process created since.
func TestMSALLockfileClaim_AbandonOnlyRemovesTheSameFile(t *testing.T) {
	_, cachePath := newAzureDir(t)
	lockPath := AzureCLIMSALCacheLockPath(cachePath)

	claim := claimMSALLockfile(context.Background(), lockPath)
	require.True(t, claim.created())
	require.NoError(t, os.Remove(lockPath))
	require.NoError(t, os.WriteFile(lockPath, []byte("67890 az"), 0o600))

	claim.abandon(lockPath)
	_, err := os.Stat(lockPath)
	require.NoError(t, err, "a lock file created by another process after this claim is kept")

	stale := claimMSALLockfile(context.Background(), lockPath)
	assert.False(t, stale.created(), "an existing unheld lock file is taken over, not created")
}

// TestMSALCacheLock_DoesNotHoldARemovedLockfile pins that a waiter never ends up holding the lock
// file's old inode after its holder deleted the path and another process created and locked a
// new lock file there. The lock reopens the path on every attempt (gofrs/flock closes the
// descriptor after a failed try), so Atmos waits for the current lock file and never writes the
// cache while the other process holds the lock.
func TestMSALCacheLock_DoesNotHoldARemovedLockfile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows FileLock is a no-op, so lock exclusion cannot be observed")
	}
	shortMSALLockfileWait(t, 50*time.Millisecond)
	_, cachePath := newAzureDir(t)
	lockPath := AzureCLIMSALCacheLockPath(cachePath)

	// An old holder has the first lock file locked; Atmos stops polling and waits on that lock.
	old, err := acquireTestMSALLock(lockPath)
	require.NoError(t, err)

	ran := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- withMSALCacheFileLock(context.Background(), cachePath, func() error {
			close(ran)
			return nil
		})
	}()
	time.Sleep(300 * time.Millisecond)

	// The old holder deletes the path while still locked, a new holder creates and locks a new
	// lock file at the same path, and only then does the old holder unlock its (unlinked) file.
	require.NoError(t, os.Remove(lockPath))
	replacement, err := acquireTestMSALLock(lockPath)
	require.NoError(t, err)
	require.NoError(t, old.lock.Unlock())

	select {
	case <-ran:
		t.Fatal("the cache was locked through a lock file that is no longer the lock file")
	case <-time.After(400 * time.Millisecond):
	}

	require.NoError(t, replacement.release())
	require.NoError(t, <-done)
	<-ran
}
