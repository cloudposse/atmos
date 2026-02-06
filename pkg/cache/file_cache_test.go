package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filesystem"
)

// mockFileLock is a simple mock for FileLock used in error path tests.
type mockFileLock struct {
	withLockErr  error
	withRLockErr error
}

func (m *mockFileLock) WithLock(fn func() error) error {
	if m.withLockErr != nil {
		return m.withLockErr
	}
	return fn()
}

func (m *mockFileLock) WithRLock(fn func() error) error {
	if m.withRLockErr != nil {
		return m.withRLockErr
	}
	return fn()
}

// newTestCache creates a FileCache for testing with the given temp directory.
func newTestCache(t *testing.T) *FileCache {
	t.Helper()
	tempDir := t.TempDir()
	lockBasePath := filepath.Join(tempDir, "cache")
	return &FileCache{
		baseDir:      tempDir,
		lockFilePath: lockBasePath + ".lock",
		lock:         NewFileLock(lockBasePath),
		fs:           filesystem.NewOSFileSystem(),
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
		assert.Regexp(t, `^[a-f0-9]+(\.(yaml|yml|json|toml|hcl|tf)(\.tmpl)?|\.tmpl)?$`, f1)
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
		// Keys with compound template extensions should preserve them.
		{"https://example.com/stack.yaml.tmpl", ".yaml.tmpl"},
		{"https://example.com/stack.yml.tmpl", ".yml.tmpl"},
		{"https://example.com/stack.json.tmpl", ".json.tmpl"},
		// Keys with bare .tmpl extension should preserve it.
		{"https://example.com/template.tmpl", ".tmpl"},
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

func TestNewFileCache_WithCustomBaseDir(t *testing.T) {
	tempDir := t.TempDir()
	customDir := filepath.Join(tempDir, "custom-cache")

	cache, err := NewFileCache("test-subpath", WithBaseDir(customDir))
	require.NoError(t, err)
	require.NotNil(t, cache)

	// Verify the custom directory was used and created.
	assert.DirExists(t, customDir)
	assert.Equal(t, customDir, cache.BaseDir())
}

func TestNewFileCache_WithCustomFileSystem(t *testing.T) {
	tempDir := t.TempDir()
	customFS := filesystem.NewOSFileSystem()

	cache, err := NewFileCache("test-subpath", WithBaseDir(tempDir), WithFileSystem(customFS))
	require.NoError(t, err)
	require.NotNil(t, cache)

	// Test that the custom filesystem is used by setting and getting a value.
	err = cache.Set("test-key", []byte("test-content"))
	require.NoError(t, err)

	content, exists, err := cache.Get("test-key")
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, []byte("test-content"), content)
}

func TestFileCache_BaseDir(t *testing.T) {
	tempDir := t.TempDir()
	cache, err := NewFileCache("test-subpath", WithBaseDir(tempDir))
	require.NoError(t, err)

	assert.Equal(t, tempDir, cache.BaseDir())
}

func TestExtractExtension_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected string
	}{
		{"empty string", "", ""},
		{"no extension", "https://example.com/config", ""},
		{"invalid extension", "https://example.com/config.txt", ""},
		{"fragment stripped", "https://example.com/config.yaml#section", ".yaml"},
		{"query and fragment", "https://example.com/config.yaml?ref=v1#section", ".yaml"},
		{"multiple dots", "https://example.com/config.backup.yaml", ".yaml"},
		{"compound yaml.tmpl", "https://example.com/stack.yaml.tmpl", ".yaml.tmpl"},
		{"compound yml.tmpl", "https://example.com/stack.yml.tmpl", ".yml.tmpl"},
		{"bare tmpl", "https://example.com/template.tmpl", ".tmpl"},
		{"txt.tmpl not compound", "https://example.com/readme.txt.tmpl", ".tmpl"},
		{"uppercase extension", "https://example.com/config.YAML", ".YAML"},
		{"only dots", "...", ""},
		{"hidden file", ".gitignore", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractExtension(tt.uri)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFileCache_Clear_EmptyDirectory(t *testing.T) {
	tempDir := t.TempDir()
	cache, err := NewFileCache("test-subpath", WithBaseDir(tempDir))
	require.NoError(t, err)

	// Clear an empty cache should not error.
	err = cache.Clear()
	require.NoError(t, err)
}

func TestFileCache_Clear_DirectoryDeletedAfterCreation(t *testing.T) {
	tempDir := t.TempDir()
	cacheDir := filepath.Join(tempDir, "cache-to-delete")

	// Create a cache with a custom directory.
	cache, err := NewFileCache("test", WithBaseDir(cacheDir))
	require.NoError(t, err)

	// Add a file to the cache.
	err = cache.Set("test-key", []byte("test-content"))
	require.NoError(t, err)

	// Delete the directory.
	err = os.RemoveAll(cacheDir)
	require.NoError(t, err)

	// Clear should handle the missing directory gracefully.
	err = cache.Clear()
	require.NoError(t, err)
}

func TestFileCache_Clear_PreservesLockFile(t *testing.T) {
	cache := newTestCache(t)

	// Add some cache entries.
	require.NoError(t, cache.Set("key1", []byte("content1")))
	require.NoError(t, cache.Set("key2", []byte("content2")))

	// Create the lock file explicitly so we can verify it persists.
	err := os.WriteFile(cache.lockFilePath, []byte("lock"), DefaultFilePerm)
	require.NoError(t, err)

	// Clear the cache.
	err = cache.Clear()
	require.NoError(t, err)

	// Verify cached entries are gone.
	_, exists, err := cache.Get("key1")
	require.NoError(t, err)
	assert.False(t, exists)

	// Verify the lock file was preserved.
	_, err = os.Stat(cache.lockFilePath)
	assert.NoError(t, err, "lock file should be preserved after Clear()")

	// Verify cache is still functional after Clear().
	require.NoError(t, cache.Set("key3", []byte("content3")))
	content, exists, err := cache.Get("key3")
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, []byte("content3"), content)
}

func TestFileCache_Get_LockError(t *testing.T) {
	tempDir := t.TempDir()
	lockErr := fmt.Errorf("lock acquisition failed")
	cache := &FileCache{
		baseDir:      tempDir,
		lockFilePath: filepath.Join(tempDir, "cache.lock"),
		lock:         &mockFileLock{withRLockErr: lockErr},
		fs:           filesystem.NewOSFileSystem(),
	}

	_, _, err := cache.Get("test-key")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCacheRead)
}

func TestFileCache_GetOrFetch_FetchError(t *testing.T) {
	cache := newTestCache(t)

	fetchErr := fmt.Errorf("download failed")
	_, err := cache.GetOrFetch("test-key", func() ([]byte, error) {
		return nil, fetchErr
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCacheFetch)
}

func TestFileCache_GetOrFetch_SetFailsStillReturnsContent(t *testing.T) {
	// Use a read-only directory so writes fail, but reads succeed.
	tempDir := t.TempDir()
	lockBasePath := filepath.Join(tempDir, "cache")
	cache := &FileCache{
		baseDir:      tempDir,
		lockFilePath: lockBasePath + ".lock",
		lock:         &mockFileLock{withLockErr: fmt.Errorf("lock error on write")},
		fs:           filesystem.NewOSFileSystem(),
	}

	// GetOrFetch: the fetch succeeds but Set fails. Content should still be returned.
	expectedContent := []byte("fetched content")
	got, err := cache.GetOrFetch("test-key", func() ([]byte, error) {
		return expectedContent, nil
	})
	require.NoError(t, err)
	assert.Equal(t, expectedContent, got, "should return fetched content even when Set fails")
}
