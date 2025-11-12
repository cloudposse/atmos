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
			cache, err := NewMSALCache(tt.cachePath)
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

	cache, err := NewMSALCache(cachePath)
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

	cache, err := NewMSALCache(cachePath)
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

	cache, err := NewMSALCache(cachePath)
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

	cache, err := NewMSALCache(cachePath)
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

	cache, err := NewMSALCache(cachePath)
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
	cache, err := NewMSALCache(cachePath)
	require.NoError(t, err)

	msalCache, ok := cache.(*msalCache)
	require.True(t, ok)

	assert.Equal(t, cachePath, msalCache.GetCachePath())
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
