package azure

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	msalcache "github.com/AzureAD/microsoft-authentication-library-for-go/apps/cache"
	"github.com/gofrs/flock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertNoMSALLockfile fails when the msal-extensions lock file of cachePath is left behind.
// The Azure CLI treats an existing `<cache>.lockfile` as held and waits up to 5 seconds for it
// to disappear on every command, so Atmos must remove it when it releases the lock, as
// msal-extensions does.
func assertNoMSALLockfile(t *testing.T, cachePath string) {
	t.Helper()
	_, err := os.Lstat(AzureCLIMSALCacheLockPath(cachePath))
	assert.True(t, os.IsNotExist(err), "the msal-extensions lock file must be removed on release (stat error: %v)", err)
}

// TestUpdateAzureCLIMSALCache_RemovesLockfile reproduces the 1.235.0 upgrade regression where
// Atmos left `msal_token_cache.json.lockfile` behind after updating the Azure CLI cache.
func TestUpdateAzureCLIMSALCache_RemovesLockfile(t *testing.T) {
	home, cachePath := newAzureDir(t)
	require.NoError(t, updateMSALCache(newTestMSALCacheUpdate(home)))
	assertNoMSALLockfile(t, cachePath)
	assert.NotEmpty(t, readMSALCacheSections(t, cachePath)[FieldAccessToken])
}

// TestMSALCache_ReplaceAndExportRemoveLockfile covers the same regression for the MSAL token
// cache used during authentication.
func TestMSALCache_ReplaceAndExportRemoveLockfile(t *testing.T) {
	cachePath := newRealmCacheFile(t, []byte(`{}`))
	c, err := NewMSALCache(cachePath, "")
	require.NoError(t, err)

	require.NoError(t, c.Replace(context.Background(), &mockUnmarshaler{}, msalcache.ReplaceHints{}))
	assertNoMSALLockfile(t, cachePath)

	marshaler := &mockMarshaler{data: []byte(`{"AccessToken":{"atmos-token":{"secret":"new"}}}`)}
	require.NoError(t, c.Export(context.Background(), marshaler, msalcache.ExportHints{}))
	assertNoMSALLockfile(t, cachePath)
	assert.Contains(t, readCacheSections(t, cachePath)["AccessToken"], "atmos-token")
}

// TestMSALLockfile_WaitsForLiveLockfile verifies the msal-extensions convention in the other
// direction: while another process (az) holds the lock file, Atmos waits for it to be released
// and removed before it reads and writes the cache.
func TestMSALLockfile_WaitsForLiveLockfile(t *testing.T) {
	home, cachePath := newAzureDir(t)
	require.NoError(t, os.WriteFile(cachePath, []byte(`{}`), 0o600))
	lockPath := AzureCLIMSALCacheLockPath(cachePath)

	holder, err := acquireTestMSALLock(lockPath)
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- updateMSALCache(newTestMSALCacheUpdate(home)) }()

	select {
	case err := <-done:
		t.Fatalf("update finished (err=%v) while the lock file was held", err)
	case <-time.After(300 * time.Millisecond):
	}

	// Release the way msal-extensions does: unlock, then delete the lock file.
	require.NoError(t, holder.release())
	require.NoError(t, <-done)
	assertNoMSALLockfile(t, cachePath)
}

// TestMSALLockfile_StaleLockfileIsTakenOver verifies a lock file nobody holds (left by a crashed
// process or an older Atmos build) does not block the update, and is removed afterwards.
func TestMSALLockfile_StaleLockfileIsTakenOver(t *testing.T) {
	home, cachePath := newAzureDir(t)
	require.NoError(t, os.WriteFile(AzureCLIMSALCacheLockPath(cachePath), []byte("12345 az"), 0o600))

	start := time.Now()
	require.NoError(t, updateMSALCache(newTestMSALCacheUpdate(home)))
	assert.Less(t, time.Since(start), msalLockfileWait, "an unheld lock file must not be waited on")
	assertNoMSALLockfile(t, cachePath)
}

// TestUpdateAzureCLIMSALCache_KeepsSymlink reproduces the 1.235.0 upgrade regression where a
// symlinked `msal_token_cache.json` was replaced by a regular file. The write now goes to the
// link's target and the link is kept.
func TestUpdateAzureCLIMSALCache_KeepsSymlink(t *testing.T) {
	home, cachePath := newAzureDir(t)
	target := filepath.Join(t.TempDir(), "shared-msal-cache.json")
	require.NoError(t, os.WriteFile(target, azOwnedMSALCache(t), 0o600))
	requireSymlink(t, target, cachePath)

	require.NoError(t, updateMSALCache(newTestMSALCacheUpdate(home)))

	assertStillSymlink(t, cachePath, target)
	sections := readMSALCacheSections(t, target)
	assert.Contains(t, sections["Account"], "az-owned-account", "the az session in the target is kept")
	assert.NotEmpty(t, sections[FieldAccessToken], "the Atmos tokens are written to the target")
}

// TestMSALCache_ExportKeepsSymlink covers the same regression for the MSAL token cache export.
func TestMSALCache_ExportKeepsSymlink(t *testing.T) {
	target := newRealmCacheFile(t, []byte(`{"Account":{"az-owned-account":{"username":"me"}}}`))
	link := filepath.Join(t.TempDir(), "msal_token_cache.json")
	requireSymlink(t, target, link)

	c, err := NewMSALCache(link, "")
	require.NoError(t, err)
	marshaler := &mockMarshaler{data: []byte(`{"AccessToken":{"atmos-token":{"secret":"new"}}}`)}
	require.NoError(t, c.Export(context.Background(), marshaler, msalcache.ExportHints{}))

	assertStillSymlink(t, link, target)
	sections := readCacheSections(t, target)
	assert.Contains(t, sections["Account"], "az-owned-account")
	assert.Contains(t, sections["AccessToken"], "atmos-token")
}

// requireSymlink creates link pointing at target, skipping the test where symlinks are unavailable.
func requireSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}
}

// assertStillSymlink checks link is still a symlink to target.
func assertStillSymlink(t *testing.T, link, target string) {
	t.Helper()
	info, err := os.Lstat(link)
	require.NoError(t, err)
	require.NotZero(t, info.Mode()&os.ModeSymlink, "the cache path must stay a symlink")
	dest, err := os.Readlink(link)
	require.NoError(t, err)
	assert.Equal(t, target, dest)
}

// testMSALLock is a lock held the way msal-extensions (az) holds it: the lock file is created
// exclusively and then locked.
type testMSALLock struct {
	path string
	lock *flock.Flock
}

// acquireTestMSALLock creates and locks lockPath like msal-extensions' CrossPlatLock.
func acquireTestMSALLock(lockPath string) (*testMSALLock, error) {
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	lock := flock.New(lockPath)
	if _, err := lock.TryLock(); err != nil {
		return nil, err
	}
	return &testMSALLock{path: lockPath, lock: lock}, nil
}

// replaceTestMSALLock simulates another process replacing the lock file at lockPath: it creates
// and locks a new file beside it, then renames that file over lockPath. The rename unlinks the
// previous file from the path and installs the locked replacement atomically, so no other
// process can create a lock file at the path in between.
func replaceTestMSALLock(lockPath string) (*testMSALLock, error) {
	nextPath := lockPath + ".next"
	next, err := acquireTestMSALLock(nextPath)
	if err != nil {
		return nil, err
	}
	if err := os.Rename(nextPath, lockPath); err != nil {
		return nil, errors.Join(err, next.release())
	}
	return &testMSALLock{path: lockPath, lock: next.lock}, nil
}

// release unlocks and then deletes the lock file, as msal-extensions does.
func (l *testMSALLock) release() error {
	if err := l.lock.Unlock(); err != nil {
		return err
	}
	return os.Remove(l.path)
}

// TestMSALCache_ExportReadOnlyDirWritesInPlace verifies the in-place fallback for the shared
// cache: in a read-only directory the temporary file cannot be created, so a writable cache file
// is written in place, as older Atmos versions did.
func TestMSALCache_ExportReadOnlyDirWritesInPlace(t *testing.T) {
	skipIfCannotDenyDirWrite(t)
	cachePath := newRealmCacheFile(t, []byte(`{"Account":{"az-owned-account":{"username":"me"}}}`))
	dir := filepath.Dir(cachePath)
	require.NoError(t, os.WriteFile(AzureCLIMSALCacheLockPath(cachePath), nil, 0o600))
	require.NoError(t, os.WriteFile(cachePath+".lock", nil, 0o600))
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	c, err := NewMSALCache(cachePath, "")
	require.NoError(t, err)
	marshaler := &mockMarshaler{data: []byte(`{"AccessToken":{"atmos-token":{"secret":"new"}}}`)}
	require.NoError(t, c.Export(context.Background(), marshaler, msalcache.ExportHints{}))

	sections := readCacheSections(t, cachePath)
	assert.Contains(t, sections["Account"], "az-owned-account")
	assert.Contains(t, sections["AccessToken"], "atmos-token")
}

// shortMSALLockfileWait shortens the wait for another process's lock file for one test.
func shortMSALLockfileWait(t *testing.T, wait time.Duration) {
	t.Helper()
	previous := msalLockfileWait
	msalLockfileWait = wait
	t.Cleanup(func() { msalLockfileWait = previous })
}

// TestMSALLockfile_TimeoutFallsBackToTheLock verifies the msal-extensions fallback: when the lock
// file is still there after the wait, Atmos stops polling for it and waits on the lock itself,
// then removes the file once it is done.
func TestMSALLockfile_TimeoutFallsBackToTheLock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows FileLock is a no-op, so waiting on the lock cannot be observed")
	}
	shortMSALLockfileWait(t, 100*time.Millisecond)
	home, cachePath := newAzureDir(t)
	lockPath := AzureCLIMSALCacheLockPath(cachePath)
	holder, err := acquireTestMSALLock(lockPath)
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- updateMSALCache(newTestMSALCacheUpdate(home)) }()

	select {
	case err := <-done:
		t.Fatalf("update finished (err=%v) while the lock was held", err)
	case <-time.After(400 * time.Millisecond):
	}

	// Unlock but leave the file, as a holder that crashed between unlock and delete would.
	require.NoError(t, holder.lock.Unlock())
	require.NoError(t, <-done)
	assertNoMSALLockfile(t, cachePath)
}

// TestClaimMSALLockfile_StopsWaiting verifies the wait ends on cancellation, and that a lock file
// that cannot be probed counts as held (Atmos keeps waiting rather than racing a live holder)
// until the wait times out.
func TestClaimMSALLockfile_StopsWaiting(t *testing.T) {
	t.Run("canceled", func(t *testing.T) {
		_, cachePath := newAzureDir(t)
		holder, err := acquireTestMSALLock(AzureCLIMSALCacheLockPath(cachePath))
		require.NoError(t, err)
		t.Cleanup(func() { _ = holder.release() })

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		start := time.Now()
		claimMSALLockfile(ctx, AzureCLIMSALCacheLockPath(cachePath))
		assert.Less(t, time.Since(start), msalLockfileWait, "a canceled wait returns at once")
	})

	t.Run("unprobeable lock file", func(t *testing.T) {
		skipIfCannotDenyDirWrite(t)
		shortMSALLockfileWait(t, 300*time.Millisecond)
		_, cachePath := newAzureDir(t)
		// A lock file that cannot even be opened exists but cannot be probed.
		lockPath := AzureCLIMSALCacheLockPath(cachePath)
		require.NoError(t, os.WriteFile(lockPath, nil, 0o000))
		assert.True(t, msalLockfileHeld(lockPath))

		start := time.Now()
		claimMSALLockfile(context.Background(), lockPath)
		assert.GreaterOrEqual(t, time.Since(start), msalLockfileWait, "an unprobeable lock file is waited on until the timeout")
	})
}
