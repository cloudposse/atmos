package cache

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/filesystem"
)

// newTestCache creates a FileCache for testing with the given temp directory.
func newTestCache(t *testing.T) *FileCache {
	t.Helper()
	tempDir := t.TempDir()
	return &FileCache{
		baseDir: tempDir,
		lock:    NewFileLock(filepath.Join(tempDir, "cache")),
		fs:      filesystem.NewOSFileSystem(),
	}
}

func TestFileCache_BasicOperations(t *testing.T) {
	cache := newTestCache(t)
	tempDir := cache.baseDir

	// Test Set and Get.
	key := "test-key"
	content := []byte("test content")

	err := cache.Set(key, content)
	require.NoError(t, err)

	// Verify file exists.
	filename := keyToFilename(key)
	path := filepath.Join(tempDir, filename)
	_, err = os.Stat(path)
	require.NoError(t, err)

	// Get the content back.
	got, exists, err := cache.Get(key)
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, content, got)
}

func TestFileCache_Get_NotFound(t *testing.T) {
	cache := newTestCache(t)

	got, exists, err := cache.Get("nonexistent-key")
	require.NoError(t, err)
	assert.False(t, exists)
	assert.Nil(t, got)
}

func TestFileCache_GetPath(t *testing.T) {
	cache := newTestCache(t)

	key := "test-key"
	content := []byte("test content")

	// Before setting, path should not exist.
	path, exists := cache.GetPath(key)
	assert.False(t, exists)
	assert.NotEmpty(t, path)

	// Set the content.
	err := cache.Set(key, content)
	require.NoError(t, err)

	// Now path should exist.
	path, exists = cache.GetPath(key)
	assert.True(t, exists)
	assert.NotEmpty(t, path)

	// Verify we can read from the path.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

func TestFileCache_GetOrFetch_Cached(t *testing.T) {
	cache := newTestCache(t)

	key := "test-key"
	content := []byte("cached content")

	// Pre-populate cache.
	err := cache.Set(key, content)
	require.NoError(t, err)

	// GetOrFetch should return cached content without calling fetch.
	fetchCalled := false
	got, err := cache.GetOrFetch(key, func() ([]byte, error) {
		fetchCalled = true
		return []byte("fetched content"), nil
	})
	require.NoError(t, err)
	assert.False(t, fetchCalled)
	assert.Equal(t, content, got)
}

func TestFileCache_GetOrFetch_Fetched(t *testing.T) {
	cache := newTestCache(t)

	key := "test-key"
	fetchedContent := []byte("fetched content")

	// GetOrFetch should call fetch and cache the result.
	fetchCalled := false
	got, err := cache.GetOrFetch(key, func() ([]byte, error) {
		fetchCalled = true
		return fetchedContent, nil
	})
	require.NoError(t, err)
	assert.True(t, fetchCalled)
	assert.Equal(t, fetchedContent, got)

	// Subsequent calls should return cached content.
	fetchCalled = false
	got, err = cache.GetOrFetch(key, func() ([]byte, error) {
		fetchCalled = true
		return []byte("should not be called"), nil
	})
	require.NoError(t, err)
	assert.False(t, fetchCalled)
	assert.Equal(t, fetchedContent, got)
}

func TestFileCache_Clear(t *testing.T) {
	cache := newTestCache(t)

	// Create some cache entries.
	require.NoError(t, cache.Set("key1", []byte("content1")))
	require.NoError(t, cache.Set("key2", []byte("content2")))

	// Clear the cache.
	err := cache.Clear()
	require.NoError(t, err)

	// Verify entries are gone.
	_, exists, err := cache.Get("key1")
	require.NoError(t, err)
	assert.False(t, exists)

	_, exists, err = cache.Get("key2")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestFileCache_ConcurrentAccess(t *testing.T) {
	cache := newTestCache(t)

	// Concurrent writes to different keys.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := keyToFilename(string(rune(n)))
			content := []byte{byte(n)}
			err := cache.Set(key, content)
			assert.NoError(t, err)
		}(i)
	}
	wg.Wait()
}

func TestKeyToFilename(t *testing.T) {
	tests := []struct {
		key1 string
		key2 string
		same bool
	}{
		{"https://example.com/config.yaml", "https://example.com/config.yaml", true},
		{"https://example.com/a.yaml", "https://example.com/b.yaml", false},
		{"", "", true},
	}

	for _, tt := range tests {
		f1 := keyToFilename(tt.key1)
		f2 := keyToFilename(tt.key2)
		if tt.same {
			assert.Equal(t, f1, f2)
		} else {
			assert.NotEqual(t, f1, f2)
		}
		// Verify filename is valid (hex hash with optional extension).
		assert.Regexp(t, `^[a-f0-9]+(\.(yaml|yml|json|toml|hcl|tf))?$`, f1)
	}
}

func TestKeyToFilename_PreservesExtension(t *testing.T) {
	tests := []struct {
		key      string
		expected string
	}{
		// Keys with valid extensions should preserve them.
		{"https://example.com/config.yaml", ".yaml"},
		{"https://example.com/config.yml", ".yml"},
		{"https://example.com/config.json", ".json"},
		{"https://example.com/config.toml", ".toml"},
		{"https://example.com/config.hcl", ".hcl"},
		{"https://example.com/config.tf", ".tf"},
		// Keys with query strings should extract extension from path.
		{"github.com/org/repo//path/config.yaml?ref=v1.0", ".yaml"},
		// Keys without valid extensions should not have extension.
		{"https://example.com/config.txt", ""},
		{"https://example.com/config", ""},
		{"simple-key", ""},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			filename := keyToFilename(tt.key)
			if tt.expected == "" {
				assert.Regexp(t, `^[a-f0-9]+$`, filename)
			} else {
				assert.Contains(t, filename, tt.expected)
			}
		})
	}
}
