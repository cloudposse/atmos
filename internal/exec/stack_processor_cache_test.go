package exec

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestClearBaseComponentConfigCache tests that the cache clearing function works correctly.
func TestClearBaseComponentConfigCache(t *testing.T) {
	// First, populate the cache with a test entry.
	testConfig := &schema.BaseComponentConfig{
		FinalBaseComponentName: "test-component",
		BaseComponentVars:      map[string]any{"key": "value"},
	}
	cacheBaseComponentConfig("test:component:base", testConfig)

	// Verify it was cached.
	_, _, found := getCachedBaseComponentConfig("test:component:base")
	assert.True(t, found, "config should be cached before clearing")

	// Clear the cache.
	ClearBaseComponentConfigCache()

	// Verify it's gone.
	_, _, found = getCachedBaseComponentConfig("test:component:base")
	assert.False(t, found, "config should not be cached after clearing")
}

// TestClearJsonSchemaCache tests that the JSON schema cache clearing works correctly.
func TestClearJsonSchemaCache(t *testing.T) {
	// Clear the cache first to start fresh.
	ClearJsonSchemaCache()

	// Verify a non-existent entry is not found.
	_, found := getCachedCompiledSchema("/path/to/schema.json")
	assert.False(t, found, "schema should not be cached")

	// Clear again (should be safe even if empty).
	ClearJsonSchemaCache()
}

// TestClearFileContentCache tests that the file content cache clearing works correctly.
func TestClearFileContentCache(t *testing.T) {
	// Create a temp file to cache.
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(tmpFile, []byte("test: content"), 0o644)
	require.NoError(t, err)

	// Read it to populate the cache.
	content1, err := GetFileContent(tmpFile)
	require.NoError(t, err)
	assert.Equal(t, "test: content", content1)

	// Clear the cache.
	ClearFileContentCache()

	// Modify the file.
	err = os.WriteFile(tmpFile, []byte("modified: content"), 0o644)
	require.NoError(t, err)

	// Read again - should get new content since cache was cleared.
	content2, err := GetFileContent(tmpFile)
	require.NoError(t, err)
	assert.Equal(t, "modified: content", content2)
}

// TestGetFileContent tests file content reading and caching.
func TestGetFileContent(t *testing.T) {
	// Clear cache to start fresh.
	ClearFileContentCache()

	// Create a temp file.
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(tmpFile, []byte("test: content\nmore: data"), 0o644)
	require.NoError(t, err)

	// First read should read from disk.
	content1, err := GetFileContent(tmpFile)
	require.NoError(t, err)
	assert.Equal(t, "test: content\nmore: data", content1)

	// Modify the file on disk.
	err = os.WriteFile(tmpFile, []byte("changed: content"), 0o644)
	require.NoError(t, err)

	// Second read should return cached content (not the changed file).
	content2, err := GetFileContent(tmpFile)
	require.NoError(t, err)
	assert.Equal(t, "test: content\nmore: data", content2, "should return cached content")

	// Clean up.
	ClearFileContentCache()
}

// TestGetFileContentNonExistent tests reading a non-existent file.
func TestGetFileContentNonExistent(t *testing.T) {
	ClearFileContentCache()

	_, err := GetFileContent("/nonexistent/path/file.yaml")
	assert.Error(t, err, "should return error for non-existent file")
}

// TestGetFileContentWithoutCache tests uncached file reading.
func TestGetFileContentWithoutCache(t *testing.T) {
	// Create a temp file.
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(tmpFile, []byte("original: content"), 0o644)
	require.NoError(t, err)

	// First read.
	content1, err := GetFileContentWithoutCache(tmpFile)
	require.NoError(t, err)
	assert.Equal(t, "original: content", content1)

	// Modify the file.
	err = os.WriteFile(tmpFile, []byte("modified: content"), 0o644)
	require.NoError(t, err)

	// Second read should see the modification (no caching).
	content2, err := GetFileContentWithoutCache(tmpFile)
	require.NoError(t, err)
	assert.Equal(t, "modified: content", content2, "should always read fresh content")
}

// TestGetFileContentWithoutCacheNonExistent tests uncached reading of non-existent file.
func TestGetFileContentWithoutCacheNonExistent(t *testing.T) {
	_, err := GetFileContentWithoutCache("/nonexistent/path/file.yaml")
	assert.Error(t, err, "should return error for non-existent file")
}

// TestCacheBaseComponentConfig tests caching of base component configurations.
func TestCacheBaseComponentConfig(t *testing.T) {
	ClearBaseComponentConfigCache()

	// Create a config with all fields populated.
	config := &schema.BaseComponentConfig{
		FinalBaseComponentName:              "final-base",
		BaseComponentCommand:                "terraform",
		BaseComponentBackendType:            "s3",
		BaseComponentRemoteStateBackendType: "s3",
		BaseComponentVars: map[string]any{
			"var1": "value1",
			"var2": map[string]any{"nested": "value"},
		},
		BaseComponentSettings: map[string]any{
			"setting1": true,
		},
		BaseComponentEnv: map[string]any{
			"ENV_VAR": "value",
		},
		BaseComponentAuth: map[string]any{
			"auth_type": "aws",
		},
		BaseComponentProviders: map[string]any{
			"aws": map[string]any{"region": "us-east-1"},
		},
		BaseComponentHooks: map[string]any{
			"pre_plan": []any{"echo hello"},
		},
		BaseComponentBackendSection: map[string]any{
			"bucket": "my-bucket",
		},
		BaseComponentRemoteStateBackendSection: map[string]any{
			"bucket": "state-bucket",
		},
		ComponentInheritanceChain: []string{"base1", "base2"},
	}

	// Cache the config.
	cacheKey := "stack:component:base"
	cacheBaseComponentConfig(cacheKey, config)

	// Retrieve and verify.
	cached, baseComponents, found := getCachedBaseComponentConfig(cacheKey)
	require.True(t, found, "config should be found in cache")
	require.NotNil(t, cached)
	require.NotNil(t, baseComponents)

	// Verify all fields.
	assert.Equal(t, "final-base", cached.FinalBaseComponentName)
	assert.Equal(t, "terraform", cached.BaseComponentCommand)
	assert.Equal(t, "s3", cached.BaseComponentBackendType)
	assert.Equal(t, "s3", cached.BaseComponentRemoteStateBackendType)
	assert.Equal(t, "value1", cached.BaseComponentVars["var1"])
	assert.Equal(t, true, cached.BaseComponentSettings["setting1"])
	assert.Equal(t, "value", cached.BaseComponentEnv["ENV_VAR"])
	assert.Equal(t, "aws", cached.BaseComponentAuth["auth_type"])
	assert.Equal(t, "my-bucket", cached.BaseComponentBackendSection["bucket"])
	assert.Equal(t, "state-bucket", cached.BaseComponentRemoteStateBackendSection["bucket"])
	assert.Equal(t, []string{"base1", "base2"}, cached.ComponentInheritanceChain)
	assert.Equal(t, []string{"base1", "base2"}, *baseComponents)

	// Clean up.
	ClearBaseComponentConfigCache()
}

// TestCacheBaseComponentConfigDeepCopy tests that cached configs are deep copied.
func TestCacheBaseComponentConfigDeepCopy(t *testing.T) {
	ClearBaseComponentConfigCache()

	// Create a config with mutable nested data.
	originalVars := map[string]any{
		"key": "original",
	}
	config := &schema.BaseComponentConfig{
		FinalBaseComponentName:    "test",
		BaseComponentVars:         originalVars,
		ComponentInheritanceChain: []string{"base1"},
	}

	// Cache it.
	cacheBaseComponentConfig("test-key", config)

	// Modify the original after caching.
	originalVars["key"] = "modified"
	config.ComponentInheritanceChain[0] = "modified-base"

	// Retrieve from cache.
	cached, _, found := getCachedBaseComponentConfig("test-key")
	require.True(t, found)

	// Cached values should NOT be affected by modifications to original.
	assert.Equal(t, "original", cached.BaseComponentVars["key"], "cached vars should not be modified")

	// Now modify the cached value.
	cached.BaseComponentVars["key"] = "cached-modified"

	// Retrieve again and verify it's still the original.
	cached2, _, found := getCachedBaseComponentConfig("test-key")
	require.True(t, found)
	assert.Equal(t, "original", cached2.BaseComponentVars["key"], "cache should return independent copies")

	// Clean up.
	ClearBaseComponentConfigCache()
}

// TestGetCachedBaseComponentConfigNotFound tests cache miss behavior.
func TestGetCachedBaseComponentConfigNotFound(t *testing.T) {
	ClearBaseComponentConfigCache()

	cached, baseComponents, found := getCachedBaseComponentConfig("nonexistent-key")
	assert.False(t, found)
	assert.Nil(t, cached)
	assert.Nil(t, baseComponents)
}

// TestConcurrentCacheAccess tests thread-safety of cache operations.
func TestConcurrentCacheAccess(t *testing.T) {
	ClearBaseComponentConfigCache()
	ClearFileContentCache()

	// Create a temp file for file content cache testing.
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "concurrent.yaml")
	err := os.WriteFile(tmpFile, []byte("concurrent: test"), 0o644)
	require.NoError(t, err)

	var wg sync.WaitGroup
	numGoroutines := 50

	// Test concurrent base component config cache access.
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			config := &schema.BaseComponentConfig{
				FinalBaseComponentName: "component",
				BaseComponentVars:      map[string]any{"id": id},
			}
			cacheKey := "stack:component:base"

			// Cache and retrieve.
			cacheBaseComponentConfig(cacheKey, config)
			getCachedBaseComponentConfig(cacheKey)
		}(i)
	}

	// Test concurrent file content cache access.
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = GetFileContent(tmpFile)
		}()
	}

	wg.Wait()

	// Clean up.
	ClearBaseComponentConfigCache()
	ClearFileContentCache()
}

// TestCacheCompiledSchemaBasic tests JSON schema caching mechanics.
func TestCacheCompiledSchemaBasic(t *testing.T) {
	ClearJsonSchemaCache()

	// Verify not found initially.
	_, found := getCachedCompiledSchema("/test/schema.json")
	assert.False(t, found)

	// Note: We can't easily test with real compiled schemas without actual schema files,
	// but we can verify the cache mechanism works with nil values.
	cacheCompiledSchema("/test/schema.json", nil)

	// Should be found now (even if nil).
	cached, found := getCachedCompiledSchema("/test/schema.json")
	assert.True(t, found)
	assert.Nil(t, cached)

	// Clean up.
	ClearJsonSchemaCache()
}
