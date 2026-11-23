package azure

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	msalcache "github.com/AzureAD/microsoft-authentication-library-for-go/apps/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/filelock"
)

// newRealmCacheFile creates a cache path in a temp dir and optionally seeds it.
func newRealmCacheFile(t *testing.T, content []byte) string {
	t.Helper()

	cachePath := filepath.Join(t.TempDir(), "msal_token_cache.json")
	if content != nil {
		require.NoError(t, os.WriteFile(cachePath, content, 0o600))
	}
	return cachePath
}

// readCacheSections decodes the cache file into its top-level sections.
func readCacheSections(t *testing.T, cachePath string) map[string]map[string]any {
	t.Helper()

	data, err := os.ReadFile(cachePath)
	require.NoError(t, err)
	var sections map[string]map[string]any
	require.NoError(t, json.Unmarshal(data, &sections), "cache must stay valid JSON: %s", data)
	return sections
}

// TestMSALCache_UnparseableCacheIsNeverOverwritten verifies a damaged cache is left
// byte-for-byte unchanged: Replace keeps authentication working (no error, nothing
// loaded) and Export skips persisting instead of replacing the file. For the empty
// realm this file is the user's own `az login` cache.
func TestMSALCache_UnparseableCacheIsNeverOverwritten(t *testing.T) {
	corrupt := []byte(`{"Account": {"az-owned-account": {"username": "me@exa`)
	cachePath := newRealmCacheFile(t, corrupt)
	c, err := NewMSALCache(cachePath, "")
	require.NoError(t, err)

	unmarshaler := &mockUnmarshaler{}
	require.NoError(t, c.Replace(context.Background(), unmarshaler, msalcache.ReplaceHints{}))
	assert.False(t, unmarshaler.called, "an unparseable cache must not be handed to MSAL")

	marshaler := &mockMarshaler{data: []byte(`{"AccessToken":{"atmos-token":{"secret":"t"}}}`)}
	require.NoError(t, c.Export(context.Background(), marshaler, msalcache.ExportHints{}),
		"skipping the write must not fail authentication")

	data, err := os.ReadFile(cachePath)
	require.NoError(t, err)
	assert.Equal(t, string(corrupt), string(data), "the damaged cache must be left unchanged")
}

// TestMSALCache_ReplaceStripsBOM verifies a UTF-8 BOM (written by some Azure CLI
// builds) is removed before the cache is handed to MSAL, whose JSON decoder rejects it.
func TestMSALCache_ReplaceStripsBOM(t *testing.T) {
	payload := `{"Account":{"a":{"username":"me@example.com"}}}`
	cachePath := newRealmCacheFile(t, append([]byte{0xEF, 0xBB, 0xBF}, payload...))
	c, err := NewMSALCache(cachePath, "")
	require.NoError(t, err)

	unmarshaler := &mockUnmarshaler{}
	require.NoError(t, c.Replace(context.Background(), unmarshaler, msalcache.ReplaceHints{}))
	require.True(t, unmarshaler.called)
	assert.Equal(t, payload, string(unmarshaler.data))
}

// TestMSALCache_ExportKeepsEntriesWrittenSinceReplace verifies Export is a
// read-modify-write: entries another process (az, or another Atmos) wrote after
// this process loaded the cache survive, and only this process's changes are applied.
func TestMSALCache_ExportKeepsEntriesWrittenSinceReplace(t *testing.T) {
	cachePath := newRealmCacheFile(t, []byte(`{"AccessToken":{"shared":{"secret":"v1"}}}`))
	c, err := NewMSALCache(cachePath, "")
	require.NoError(t, err)
	require.NoError(t, c.Replace(context.Background(), &mockUnmarshaler{}, msalcache.ReplaceHints{}))

	// Another process updates the shared token and adds its own account after our Replace.
	require.NoError(t, os.WriteFile(cachePath,
		[]byte(`{"AccessToken":{"shared":{"secret":"v2"}},"Account":{"az-owned-account":{"username":"me"}}}`), 0o600))

	// MSAL still holds the v1 copy of "shared" (unchanged by us) and adds a token.
	marshaler := &mockMarshaler{data: []byte(`{"AccessToken":{"shared":{"secret":"v1"},"atmos-token":{"secret":"new"}}}`)}
	require.NoError(t, c.Export(context.Background(), marshaler, msalcache.ExportHints{}))

	sections := readCacheSections(t, cachePath)
	assert.Contains(t, sections["Account"], "az-owned-account", "an entry written by another process must survive")
	assert.Equal(t, map[string]any{"secret": "new"}, sections["AccessToken"]["atmos-token"], "our new entry must be written")
	assert.Equal(t, map[string]any{"secret": "v2"}, sections["AccessToken"]["shared"],
		"an entry we did not change must not overwrite a newer copy on disk")
}

// TestMSALCache_ExportAfterFailedMSALLoadKeepsDiskEntries verifies that when MSAL
// cannot load a JSON cache (so its in-memory cache starts empty), Export adds the
// new entries without dropping the ones on disk.
func TestMSALCache_ExportAfterFailedMSALLoadKeepsDiskEntries(t *testing.T) {
	cachePath := newRealmCacheFile(t, []byte(`{"Account":{"az-owned-account":{"username":"me"}}}`))
	c, err := NewMSALCache(cachePath, "")
	require.NoError(t, err)

	failing := &errorUnmarshaler{err: assert.AnError}
	require.NoError(t, c.Replace(context.Background(), failing, msalcache.ReplaceHints{}))
	require.True(t, failing.called)

	marshaler := &mockMarshaler{data: []byte(`{"AccessToken":{"atmos-token":{"secret":"new"}}}`)}
	require.NoError(t, c.Export(context.Background(), marshaler, msalcache.ExportHints{}))

	sections := readCacheSections(t, cachePath)
	assert.Contains(t, sections["Account"], "az-owned-account")
	assert.Contains(t, sections["AccessToken"], "atmos-token")
}

// TestMSALCache_LocksMSALExtensionsAndLegacyLockFiles verifies Replace and Export
// wait for both the msal-extensions lock file (`<cache>.lockfile`, used by az and
// by Atmos's Azure CLI cache update) and the legacy `<cache>.lock` used by older
// Atmos releases.
func TestMSALCache_LocksMSALExtensionsAndLegacyLockFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows FileLock is a no-op, so lock exclusion cannot be observed")
	}

	for _, suffix := range []string{".lockfile", ".lock"} {
		t.Run(suffix, func(t *testing.T) {
			cachePath := newRealmCacheFile(t, []byte(`{}`))
			c, err := NewMSALCache(cachePath, "")
			require.NoError(t, err)

			locked := make(chan struct{})
			release := make(chan struct{})
			holderDone := make(chan error, 1)
			go func() {
				holderDone <- filelock.New(cachePath+suffix).WithExclusive(context.Background(), func() error {
					close(locked)
					<-release
					return os.WriteFile(cachePath, []byte(`{"Account":{"az-owned-account":{"username":"me"}}}`), 0o600)
				})
			}()
			<-locked

			exportDone := make(chan error, 1)
			go func() {
				marshaler := &mockMarshaler{data: []byte(`{"AccessToken":{"atmos-token":{"secret":"new"}}}`)}
				exportDone <- c.Export(context.Background(), marshaler, msalcache.ExportHints{})
			}()

			select {
			case err := <-exportDone:
				t.Fatalf("Export finished (err=%v) while %s was held", err, suffix)
			case <-time.After(200 * time.Millisecond):
			}

			close(release)
			require.NoError(t, <-holderDone)
			require.NoError(t, <-exportDone)

			sections := readCacheSections(t, cachePath)
			assert.Contains(t, sections["Account"], "az-owned-account", "the write made while holding the lock must survive")
			assert.Contains(t, sections["AccessToken"], "atmos-token")
		})
	}
}
