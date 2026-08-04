package azure

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	msalcache "github.com/AzureAD/microsoft-authentication-library-for-go/apps/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMSALCache(t *testing.T) {
	// Redirect HOME/USERPROFILE so the default-path case creates its cache
	// directory under a temp home instead of the real ~/.azure.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	tests := []struct {
		name          string
		cachePath     string
		expectDefault bool
	}{
		{
			name:          "custom path",
			cachePath:     "/tmp/test_msal_cache.json",
			expectDefault: false,
		},
		{
			name:          "default path",
			cachePath:     "",
			expectDefault: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache, err := NewMSALCache(tt.cachePath, "")
			require.NoError(t, err)
			require.NotNil(t, cache)

			msalCache, ok := cache.(*msalCache)
			require.True(t, ok, "Expected *msalCache type")

			if tt.expectDefault {
				homeDir, _ := os.UserHomeDir()
				expectedPath := filepath.Join(homeDir, ".azure", "msal_token_cache.json")
				assert.Equal(t, expectedPath, msalCache.cachePath)
			} else {
				assert.Equal(t, tt.cachePath, msalCache.cachePath)
			}
		})
	}
}

func TestMSALCache_ReplaceEmpty(t *testing.T) {
	// Create temporary cache file.
	tempDir := t.TempDir()
	cachePath := filepath.Join(tempDir, "msal_cache.json")

	cache, err := NewMSALCache(cachePath, "")
	require.NoError(t, err)

	ctx := context.Background()

	// Test Replace with non-existent cache (should not error).
	mockUnmarshaler := &mockUnmarshaler{}
	err = cache.Replace(ctx, mockUnmarshaler, msalcache.ReplaceHints{})
	assert.NoError(t, err, "Replace should succeed with non-existent cache")
	assert.False(t, mockUnmarshaler.called, "Unmarshal should not be called for non-existent cache")
}

func TestMSALCache_ReplaceExisting(t *testing.T) {
	// Create temporary cache file with test data.
	tempDir := t.TempDir()
	cachePath := filepath.Join(tempDir, "msal_cache.json")

	testData := []byte(`{"AccessToken": {}, "Account": {}}`)
	err := os.WriteFile(cachePath, testData, 0o600)
	require.NoError(t, err)

	cache, err := NewMSALCache(cachePath, "")
	require.NoError(t, err)

	ctx := context.Background()

	// Test Replace with existing cache.
	mockUnmarshaler := &mockUnmarshaler{}
	err = cache.Replace(ctx, mockUnmarshaler, msalcache.ReplaceHints{})
	assert.NoError(t, err)
	assert.True(t, mockUnmarshaler.called, "Unmarshal should be called for existing cache")
	assert.Equal(t, testData, mockUnmarshaler.data)
}

func TestMSALCache_Export(t *testing.T) {
	tempDir := t.TempDir()
	cachePath := filepath.Join(tempDir, "msal_cache.json")

	cache, err := NewMSALCache(cachePath, "")
	require.NoError(t, err)

	ctx := context.Background()

	// Test Export.
	testData := []byte(`{"AccessToken": {"key1": "value1"}, "RefreshToken": {"key2": "value2"}}`)
	mockMarshaler := &mockMarshaler{data: testData}
	err = cache.Export(ctx, mockMarshaler, msalcache.ExportHints{})
	require.NoError(t, err)

	// Verify file was written.
	writtenData, err := os.ReadFile(cachePath)
	require.NoError(t, err)
	assert.Equal(t, testData, writtenData)

	// Verify file permissions (Unix only - Windows uses different permission model).
	if runtime.GOOS != "windows" {
		info, err := os.Stat(cachePath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "Cache file should have 0600 permissions")
	}
}

func TestMSALCache_ReplaceWithCancellation(t *testing.T) {
	tempDir := t.TempDir()
	cachePath := filepath.Join(tempDir, "msal_cache.json")

	cache, err := NewMSALCache(cachePath, "")
	require.NoError(t, err)

	// Create cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mockUnmarshaler := &mockUnmarshaler{}
	err = cache.Replace(ctx, mockUnmarshaler, msalcache.ReplaceHints{})
	assert.Error(t, err, "Replace should fail with cancelled context")
	assert.ErrorIs(t, err, context.Canceled)
}

func TestMSALCache_ExportWithCancellation(t *testing.T) {
	tempDir := t.TempDir()
	cachePath := filepath.Join(tempDir, "msal_cache.json")

	cache, err := NewMSALCache(cachePath, "")
	require.NoError(t, err)

	// Create cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mockMarshaler := &mockMarshaler{data: []byte("{}")}
	err = cache.Export(ctx, mockMarshaler, msalcache.ExportHints{})
	assert.Error(t, err, "Export should fail with cancelled context")
	assert.ErrorIs(t, err, context.Canceled)
}

func TestMSALCache_GetCachePath(t *testing.T) {
	cachePath := "/tmp/test_cache.json"
	cache, err := NewMSALCache(cachePath, "")
	require.NoError(t, err)

	msalCache, ok := cache.(*msalCache)
	require.True(t, ok)

	assert.Equal(t, cachePath, msalCache.GetCachePath())
}

// requireLockDenied makes tempDir read-only so that the ".lock" sibling file
// withFileLock creates next to the cache file cannot be opened, forcing a
// fast, deterministic lock-acquisition failure without waiting on a real
// contended lock. Restores permissions on cleanup so t.TempDir() can remove it.
func requireLockDenied(t *testing.T, tempDir string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("file locking is best-effort/noop on Windows; lock-failure path is not reachable")
	}
	if os.Geteuid() == 0 {
		t.Skip("Skipping test when running as root: root bypasses directory permission checks")
	}

	require.NoError(t, os.Chmod(tempDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(tempDir, 0o755) })
}

func TestMSALCache_ReplacePropagatesLockError(t *testing.T) {
	tempDir := t.TempDir()
	cachePath := filepath.Join(tempDir, "msal_cache.json")

	cache, err := NewMSALCache(cachePath, "")
	require.NoError(t, err)

	requireLockDenied(t, tempDir)

	mockUnmarshaler := &mockUnmarshaler{}
	err = cache.Replace(context.Background(), mockUnmarshaler, msalcache.ReplaceHints{})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFileLockTimeout)
	assert.False(t, mockUnmarshaler.called, "Unmarshal must not run when the lock cannot be acquired")
}

func TestMSALCache_ExportPropagatesLockError(t *testing.T) {
	tempDir := t.TempDir()
	cachePath := filepath.Join(tempDir, "msal_cache.json")

	cache, err := NewMSALCache(cachePath, "")
	require.NoError(t, err)

	requireLockDenied(t, tempDir)

	mockMarshaler := &mockMarshaler{data: []byte(`{"AccessToken":{}}`)}
	err = cache.Export(context.Background(), mockMarshaler, msalcache.ExportHints{})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFileLockTimeout)

	// The cache file itself must not have been written.
	_, statErr := os.Stat(cachePath)
	assert.True(t, os.IsNotExist(statErr), "cache file should not exist when the lock cannot be acquired")
}

func TestNewMSALCache_HomeDirError(t *testing.T) {
	// Clear both the Unix and Windows home-directory env vars so
	// os.UserHomeDir() fails, exercising NewMSALCache's error-wrapping branch.
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")

	cache, err := NewMSALCache("", "")

	require.Error(t, err)
	assert.Nil(t, cache)
	assert.Contains(t, err.Error(), "failed to get user home directory")
}

func TestNewMSALCache_RealmIsolatesDefaultPath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	cache, err := NewMSALCache("", "my-realm")
	require.NoError(t, err)

	msalCache, ok := cache.(*msalCache)
	require.True(t, ok)

	expectedPath := filepath.Join(homeDir, ".azure", "atmos", "my-realm", "msal_token_cache.json")
	assert.Equal(t, expectedPath, msalCache.cachePath, "realm must isolate the default cache path from the shared one")
}

func TestNewMSALCache_MkdirAllError(t *testing.T) {
	tempDir := t.TempDir()

	// Create a regular file where a directory segment is expected, so
	// MkdirAll(cacheDir) fails to create the cache directory.
	blocker := filepath.Join(tempDir, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("not a directory"), 0o600))

	cachePath := filepath.Join(blocker, "sub", "msal_cache.json")

	cache, err := NewMSALCache(cachePath, "")

	require.Error(t, err)
	assert.Nil(t, cache)
	assert.Contains(t, err.Error(), "failed to create cache directory")
}

func TestMSALCache_ReplaceReadErrorNotNotExist(t *testing.T) {
	tempDir := t.TempDir()
	// Point the cache path at a directory instead of a file: os.ReadFile
	// fails with an error other than os.IsNotExist, exercising the
	// "failed to read MSAL cache" branch (distinct from the not-exist path
	// already covered by TestMSALCache_ReplaceEmpty).
	cachePath := filepath.Join(tempDir, "cache-is-a-dir")
	require.NoError(t, os.Mkdir(cachePath, 0o755))

	cache, err := NewMSALCache(cachePath, "")
	require.NoError(t, err)

	mockUnmarshaler := &mockUnmarshaler{}
	err = cache.Replace(context.Background(), mockUnmarshaler, msalcache.ReplaceHints{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read MSAL cache")
	assert.False(t, mockUnmarshaler.called)
}

func TestMSALCache_ReplaceUnmarshalErrorStartsFresh(t *testing.T) {
	tempDir := t.TempDir()
	cachePath := filepath.Join(tempDir, "msal_cache.json")
	require.NoError(t, os.WriteFile(cachePath, []byte("some cached bytes"), 0o600))

	cache, err := NewMSALCache(cachePath, "")
	require.NoError(t, err)

	failingUnmarshaler := &errorUnmarshaler{err: assert.AnError}
	err = cache.Replace(context.Background(), failingUnmarshaler, msalcache.ReplaceHints{})

	// A corrupted on-disk cache must not fail auth: Replace should log and
	// start fresh rather than propagate the unmarshal error.
	require.NoError(t, err)
	assert.True(t, failingUnmarshaler.called, "Unmarshal should have been attempted")
}

func TestMSALCache_ExportMarshalError(t *testing.T) {
	tempDir := t.TempDir()
	cachePath := filepath.Join(tempDir, "msal_cache.json")

	cache, err := NewMSALCache(cachePath, "")
	require.NoError(t, err)

	failingMarshaler := &errorMarshaler{err: assert.AnError}
	err = cache.Export(context.Background(), failingMarshaler, msalcache.ExportHints{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to marshal MSAL cache")

	_, statErr := os.Stat(cachePath)
	assert.True(t, os.IsNotExist(statErr), "cache file should not be written when marshaling fails")
}

func TestMSALCache_ExportWriteErrorIsADirectory(t *testing.T) {
	tempDir := t.TempDir()
	// Point the cache path at a directory instead of a file: os.WriteFile
	// fails, exercising the "failed to write MSAL cache" branch. The lock
	// (a sibling ".lock" file) is still acquired successfully since only
	// cachePath itself, not its parent, is a directory.
	cachePath := filepath.Join(tempDir, "cache-is-a-dir")
	require.NoError(t, os.Mkdir(cachePath, 0o755))

	cache, err := NewMSALCache(cachePath, "")
	require.NoError(t, err)

	mockMarshaler := &mockMarshaler{data: []byte(`{"AccessToken":{}}`)}
	err = cache.Export(context.Background(), mockMarshaler, msalcache.ExportHints{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write MSAL cache")
}

// Mock types for testing.

type mockUnmarshaler struct {
	called bool
	data   []byte
}

func (m *mockUnmarshaler) Unmarshal(data []byte) error {
	m.called = true
	m.data = data
	return nil
}

type mockMarshaler struct {
	data []byte
}

func (m *mockMarshaler) Marshal() ([]byte, error) {
	return m.data, nil
}

// errorUnmarshaler always fails, simulating a corrupted on-disk cache.
type errorUnmarshaler struct {
	called bool
	err    error
}

func (m *errorUnmarshaler) Unmarshal(data []byte) error {
	m.called = true
	return m.err
}

// errorMarshaler always fails, simulating an MSAL internal marshaling error.
type errorMarshaler struct {
	err error
}

func (m *errorMarshaler) Marshal() ([]byte, error) {
	return nil, m.err
}
