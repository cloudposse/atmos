package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	msalcache "github.com/AzureAD/microsoft-authentication-library-for-go/apps/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestMSALCache_ConcurrentExportsKeepEveryEntry verifies that several Atmos
// processes (simulated by separate cache instances on one file) logging in at the
// same time each keep their tokens: every Export re-reads the file under the lock.
func TestMSALCache_ConcurrentExportsKeepEveryEntry(t *testing.T) {
	cachePath := newRealmCacheFile(t, []byte(`{}`))

	const writers = 8
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := NewMSALCache(cachePath, "")
			if err != nil {
				errs <- err
				return
			}
			if err := c.Replace(context.Background(), &mockUnmarshaler{}, msalcache.ReplaceHints{}); err != nil {
				errs <- err
				return
			}
			data := fmt.Sprintf(`{"AccessToken":{"token-%d":{"secret":"s%d"}}}`, i, i)
			errs <- c.Export(context.Background(), &mockMarshaler{data: []byte(data)}, msalcache.ExportHints{})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	sections := readCacheSections(t, cachePath)
	require.Len(t, sections["AccessToken"], writers)
	assert.Equal(t, map[string]any{"secret": "s0"}, sections["AccessToken"]["token-0"])
	assert.Equal(t, map[string]any{"secret": fmt.Sprintf("s%d", writers-1)}, sections["AccessToken"][fmt.Sprintf("token-%d", writers-1)])
}

// TestMSALCache_ExportPreservesNumbersAndTopLevelFields verifies the
// read-modify-write keeps numbers verbatim and carries top-level values that are
// not sections, writing a changed one and keeping an unchanged one as on disk.
func TestMSALCache_ExportPreservesNumbersAndTopLevelFields(t *testing.T) {
	cachePath := newRealmCacheFile(t,
		[]byte(`{"Version":1,"Owner":"az","Account":{"a":{"cached_at":12345678901234567890}}}`))
	c, err := NewMSALCache(cachePath, "")
	require.NoError(t, err)
	require.NoError(t, c.Replace(context.Background(), &mockUnmarshaler{}, msalcache.ReplaceHints{}))

	// Another process changes Owner after our load; MSAL changes Version.
	require.NoError(t, os.WriteFile(cachePath,
		[]byte(`{"Version":1,"Owner":"other","Account":{"a":{"cached_at":12345678901234567890}}}`), 0o600))
	marshaler := &mockMarshaler{data: []byte(`{"Version":2,"Owner":"az","Account":{"a":{"cached_at":12345678901234567890}}}`)}
	require.NoError(t, c.Export(context.Background(), marshaler, msalcache.ExportHints{}))

	data, err := os.ReadFile(cachePath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "12345678901234567890", "large numbers must round-trip verbatim")
	var top map[string]any
	require.NoError(t, json.Unmarshal(data, &top))
	assert.Equal(t, float64(2), top["Version"], "a value MSAL changed is written")
	assert.Equal(t, "other", top["Owner"], "a value MSAL did not change keeps the newer copy on disk")
}

// TestDecodeMSALCacheState covers the accepted and rejected cache shapes.
func TestDecodeMSALCacheState(t *testing.T) {
	for _, input := range []string{"", "null", "{}"} {
		state, err := decodeMSALCacheState([]byte(input))
		require.NoError(t, err, input)
		assert.Empty(t, state, input)
	}
	for _, input := range []string{"[]", `"x"`, "{", `{} {}`, `{"a":1} trailing`} {
		_, err := decodeMSALCacheState([]byte(input))
		require.Error(t, err, input)
	}
}

// TestReadMSALCacheState covers the missing, BOM-prefixed and unparseable cases.
func TestReadMSALCacheState(t *testing.T) {
	dir := t.TempDir()

	state, err := readMSALCacheState(filepath.Join(dir, "missing.json"))
	require.NoError(t, err)
	assert.Empty(t, state)

	bomPath := filepath.Join(dir, "bom.json")
	require.NoError(t, os.WriteFile(bomPath, append([]byte{0xEF, 0xBB, 0xBF}, ` {"Account":{}} `...), 0o600))
	state, err = readMSALCacheState(bomPath)
	require.NoError(t, err)
	assert.Contains(t, state, "Account")

	badPath := filepath.Join(dir, "bad.json")
	require.NoError(t, os.WriteFile(badPath, []byte("not json"), 0o600))
	_, err = readMSALCacheState(badPath)
	require.ErrorIs(t, err, errUtils.ErrAzureMSALCacheUnparseable)
}

// TestMSALCache_ExportOursUnparseable verifies a marshaler output that is not a JSON
// object is reported and nothing is written.
func TestMSALCache_ExportOursUnparseable(t *testing.T) {
	cachePath := newRealmCacheFile(t, nil)
	c, err := NewMSALCache(cachePath, "")
	require.NoError(t, err)

	err = c.Export(context.Background(), &mockMarshaler{data: []byte(`[]`)}, msalcache.ExportHints{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to marshal MSAL cache")
	_, statErr := os.Stat(cachePath)
	assert.True(t, os.IsNotExist(statErr))
}
