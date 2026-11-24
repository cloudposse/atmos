package azure

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filelock"
)

// azOwnedMSALCache is a cache the user's own `az login` wrote; every Atmos update must keep it.
func azOwnedMSALCache(t *testing.T) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]interface{}{
		"Account": map[string]interface{}{
			"az-owned-account": map[string]interface{}{"username": "me@example.com"},
		},
		"RefreshToken": map[string]interface{}{
			"az-owned-refresh-token": map[string]interface{}{"secret": "keep-me"},
		},
	})
	require.NoError(t, err)
	return data
}

func newTestMSALCacheUpdate(home string) *msalCacheUpdate {
	return &msalCacheUpdate{
		Home:        home,
		AccessToken: createTestJWT(map[string]interface{}{"oid": "user-oid-123", "upn": "user@example.com"}),
		Expiration:  time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
		UserOID:     "user-oid-123",
		TenantID:    "tenant-123",
	}
}

func readMSALCacheSections(t *testing.T, path string) map[string]map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var sections map[string]map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &sections))
	return sections
}

func newAzureDir(t *testing.T) (home, cachePath string) {
	t.Helper()
	home = t.TempDir()
	azureDir := filepath.Join(home, ".azure")
	require.NoError(t, os.MkdirAll(azureDir, 0o700))
	return home, filepath.Join(azureDir, "msal_token_cache.json")
}

// TestUpdateMSALCache_PreservesBOMPrefixedCache verifies a BOM-prefixed az cache is
// parsed and extended rather than rejected or replaced.
func TestUpdateMSALCache_PreservesBOMPrefixedCache(t *testing.T) {
	home, cachePath := newAzureDir(t)
	bom := []byte{BomMarker, BomSecondByte, BomThirdByte}
	require.NoError(t, os.WriteFile(cachePath, append(bom, azOwnedMSALCache(t)...), 0o600))

	require.NoError(t, updateMSALCache(newTestMSALCacheUpdate(home)))

	sections := readMSALCacheSections(t, cachePath)
	assert.Contains(t, sections["Account"], "az-owned-account")
	assert.Contains(t, sections["RefreshToken"], "az-owned-refresh-token")
	assert.Len(t, sections["Account"], 2, "the Atmos account is added next to the az-owned one")
	assert.NotEmpty(t, sections[FieldAccessToken])
}

// TestUpdateMSALCache_ReadsUnderMSALExtensionsLock verifies the read happens under
// the msal-extensions lock file: a write az makes while it holds that lock must
// not be lost by the Atmos update.
func TestUpdateMSALCache_ReadsUnderMSALExtensionsLock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows FileLock is a no-op, so lock exclusion cannot be observed")
	}
	home, cachePath := newAzureDir(t)
	require.NoError(t, os.WriteFile(cachePath, []byte(`{}`), 0o600))

	locked := make(chan struct{})
	release := make(chan struct{})
	holderDone := make(chan error, 1)
	go func() {
		holderDone <- filelock.New(cachePath+".lockfile").WithExclusive(context.Background(), func() error {
			close(locked)
			<-release
			return os.WriteFile(cachePath, azOwnedMSALCache(t), 0o600)
		})
	}()
	<-locked

	updateDone := make(chan error, 1)
	go func() { updateDone <- updateMSALCache(newTestMSALCacheUpdate(home)) }()

	select {
	case err := <-updateDone:
		t.Fatalf("update finished (err=%v) while the msal-extensions lock was held", err)
	case <-time.After(200 * time.Millisecond):
	}

	close(release)
	require.NoError(t, <-holderDone)
	require.NoError(t, <-updateDone)

	sections := readMSALCacheSections(t, cachePath)
	assert.Contains(t, sections["Account"], "az-owned-account", "the write az made while holding the lock must survive")
	assert.NotEmpty(t, sections[FieldAccessToken])
}

// TestUpdateMSALCache_LeavesUnparseableCacheUntouched verifies a damaged cache is
// reported and left byte-for-byte unchanged.
func TestUpdateMSALCache_LeavesUnparseableCacheUntouched(t *testing.T) {
	home, cachePath := newAzureDir(t)
	corrupt := []byte(`{"Account": {"az-owned-account": {"username": "me@exa`)
	require.NoError(t, os.WriteFile(cachePath, corrupt, 0o600))

	err := updateMSALCache(newTestMSALCacheUpdate(home))
	require.Error(t, err)

	data, readErr := os.ReadFile(cachePath)
	require.NoError(t, readErr)
	assert.Equal(t, string(corrupt), string(data))
}

// errMutate is a sentinel returned by a test mutate callback.
var errMutate = errors.New("mutate failed")

// TestUpdateAzureCLIMSALCache covers the shared read-modify-write helper directly.
func TestUpdateAzureCLIMSALCache(t *testing.T) {
	t.Run("missing file starts an empty cache and is created", func(t *testing.T) {
		cachePath := filepath.Join(t.TempDir(), ".azure", "msal_token_cache.json")
		var seen map[string]interface{}
		require.NoError(t, UpdateAzureCLIMSALCache(cachePath, func(c map[string]interface{}) error {
			seen = c
			c["Account"] = map[string]interface{}{"k": "v"}
			return nil
		}))
		require.NotNil(t, seen)
		assert.Equal(t, map[string]map[string]interface{}{"Account": {"k": "v"}}, readMSALCacheSections(t, cachePath))
	})

	for name, content := range map[string]string{
		"empty file":      "",
		"whitespace only": " \n\t",
		"BOM only":        string([]byte{BomMarker, BomSecondByte, BomThirdByte}),
		"JSON null":       "null",
	} {
		t.Run(name+" is treated as an empty cache", func(t *testing.T) {
			_, cachePath := newAzureDir(t)
			require.NoError(t, os.WriteFile(cachePath, []byte(content), 0o600))
			var seen map[string]interface{}
			require.NoError(t, UpdateAzureCLIMSALCache(cachePath, func(c map[string]interface{}) error {
				seen = c
				return nil
			}))
			require.NotNil(t, seen, "mutate must receive a non-nil map")
			assert.Empty(t, seen)
		})
	}

	t.Run("unparseable cache is reported and not passed to mutate", func(t *testing.T) {
		_, cachePath := newAzureDir(t)
		require.NoError(t, os.WriteFile(cachePath, []byte("not json"), 0o600))
		called := false
		err := UpdateAzureCLIMSALCache(cachePath, func(map[string]interface{}) error {
			called = true
			return nil
		})
		require.ErrorIs(t, err, errUtils.ErrAzureCLIMSALCacheUnparseable)
		assert.False(t, called)
	})

	t.Run("mutate error aborts without writing", func(t *testing.T) {
		_, cachePath := newAzureDir(t)
		original := azOwnedMSALCache(t)
		require.NoError(t, os.WriteFile(cachePath, original, 0o600))
		err := UpdateAzureCLIMSALCache(cachePath, func(c map[string]interface{}) error {
			c["Account"] = nil
			return errMutate
		})
		require.ErrorIs(t, err, errMutate)
		data, readErr := os.ReadFile(cachePath)
		require.NoError(t, readErr)
		assert.Equal(t, string(original), string(data))
	})

	t.Run("read failure other than not-exist is reported", func(t *testing.T) {
		_, cachePath := newAzureDir(t)
		// A directory at the cache path makes the read fail on every platform.
		require.NoError(t, os.MkdirAll(cachePath, 0o700))
		err := UpdateAzureCLIMSALCache(cachePath, func(map[string]interface{}) error { return nil })
		require.ErrorIs(t, err, errUtils.ErrAzureCLIMSALCacheUpdate)
	})

	t.Run("directory creation failure is reported", func(t *testing.T) {
		home := t.TempDir()
		// ".azure" exists as a regular file, so MkdirAll fails.
		require.NoError(t, os.WriteFile(filepath.Join(home, ".azure"), []byte("not a dir"), 0o600))
		err := UpdateAzureCLIMSALCache(filepath.Join(home, ".azure", "msal_token_cache.json"), func(map[string]interface{}) error { return nil })
		require.ErrorIs(t, err, errUtils.ErrAzureCLIMSALCacheUpdate)
	})

	t.Run("marshal failure is reported without writing", func(t *testing.T) {
		_, cachePath := newAzureDir(t)
		err := UpdateAzureCLIMSALCache(cachePath, func(c map[string]interface{}) error {
			c["bad"] = make(chan int)
			return nil
		})
		require.ErrorIs(t, err, errUtils.ErrAzureCLIMSALCacheUpdate)
		_, statErr := os.Stat(cachePath)
		assert.True(t, os.IsNotExist(statErr))
	})

	t.Run("write failure is reported", func(t *testing.T) {
		skipIfCannotDenyDirWrite(t)
		_, cachePath := newAzureDir(t)
		azureDir := filepath.Dir(cachePath)
		// Pre-create the lock files (msal-extensions and legacy Atmos) so locking
		// succeeds, then deny writes so the cache write fails.
		require.NoError(t, os.WriteFile(AzureCLIMSALCacheLockPath(cachePath), nil, 0o600))
		require.NoError(t, os.WriteFile(cachePath+".lock", nil, 0o600))
		require.NoError(t, os.Chmod(azureDir, 0o500))
		t.Cleanup(func() { _ = os.Chmod(azureDir, 0o700) })
		err := UpdateAzureCLIMSALCache(cachePath, func(map[string]interface{}) error { return nil })
		require.ErrorIs(t, err, errUtils.ErrAzureCLIMSALCacheUpdate)
	})

	t.Run("lock failure is reported as a lock timeout", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows FileLock is a no-op and never reports a lock failure")
		}
		skipIfCannotDenyDirWrite(t)
		_, cachePath := newAzureDir(t)
		azureDir := filepath.Dir(cachePath)
		// No lock file exists and the directory is read-only, so creating it fails.
		require.NoError(t, os.Chmod(azureDir, 0o500))
		t.Cleanup(func() { _ = os.Chmod(azureDir, 0o700) })
		err := UpdateAzureCLIMSALCache(cachePath, func(map[string]interface{}) error { return nil })
		require.ErrorIs(t, err, ErrFileLockTimeout)
	})
}

func TestAzureCLIMSALCacheLockPath(t *testing.T) {
	path := filepath.Join("home", ".azure", "msal_token_cache.json")
	assert.Equal(t, path+".lockfile", AzureCLIMSALCacheLockPath(path))
}
