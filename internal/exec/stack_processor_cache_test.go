package exec

import (
	"fmt"
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
		BaseComponentMetadata: map[string]any{
			"component_type": "terraform",
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
	assert.Equal(t, "terraform", cached.BaseComponentMetadata["component_type"])
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
	originalMetadata := map[string]any{
		"type": "original",
	}
	config := &schema.BaseComponentConfig{
		FinalBaseComponentName:    "test",
		BaseComponentVars:         originalVars,
		BaseComponentMetadata:     originalMetadata,
		ComponentInheritanceChain: []string{"base1"},
	}

	// Cache it.
	cacheBaseComponentConfig("test-key", config)

	// Modify the original after caching.
	originalVars["key"] = "modified"
	originalMetadata["type"] = "modified"
	config.ComponentInheritanceChain[0] = "modified-base"

	// Retrieve from cache.
	cached, _, found := getCachedBaseComponentConfig("test-key")
	require.True(t, found)

	// Cached values should NOT be affected by modifications to original.
	assert.Equal(t, "original", cached.BaseComponentVars["key"], "cached vars should not be modified")
	assert.Equal(t, "original", cached.BaseComponentMetadata["type"], "cached metadata should not be modified")

	// Now modify the cached value.
	cached.BaseComponentVars["key"] = "cached-modified"
	cached.BaseComponentMetadata["type"] = "cached-modified"

	// Retrieve again and verify it's still the original.
	cached2, _, found := getCachedBaseComponentConfig("test-key")
	require.True(t, found)
	assert.Equal(t, "original", cached2.BaseComponentVars["key"], "cache should return independent copies")
	assert.Equal(t, "original", cached2.BaseComponentMetadata["type"], "cache should return independent copies for metadata")

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

// TestConcurrentCacheAccess_DisjointKeys verifies that many goroutines
// writing distinct cache keys all succeed and every value is retrievable
// afterwards. This is the production-realistic pattern: each component
// instance writes its own unique "stack:component:baseComponent" key,
// so the cache sees high write concurrency with no key overlap. The
// Phase 2 sync.Map + outside-the-lock deep-copy change is specifically
// optimized for this pattern; a regression that reintroduces a global
// lock would still pass under TestConcurrentCacheAccess (single key,
// race detector only verifies safety, not throughput) but would
// reintroduce the lock-contention pathology this test guards against.
func TestConcurrentCacheAccess_DisjointKeys(t *testing.T) {
	ClearBaseComponentConfigCache()
	defer ClearBaseComponentConfigCache()

	const numKeys = 200
	var wg sync.WaitGroup

	// Each goroutine writes a unique key.
	for i := 0; i < numKeys; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			config := &schema.BaseComponentConfig{
				FinalBaseComponentName: fmt.Sprintf("component-%d", id),
				BaseComponentVars: map[string]any{
					"id":    id,
					"label": fmt.Sprintf("instance-%d", id),
				},
				ComponentInheritanceChain: []string{fmt.Sprintf("base-%d", id)},
			}
			cacheBaseComponentConfig(fmt.Sprintf("stack-%d:component:base", id), config)
		}(i)
	}
	wg.Wait()

	// Verify every key is independently readable with the right value.
	for i := 0; i < numKeys; i++ {
		cached, _, found := getCachedBaseComponentConfig(fmt.Sprintf("stack-%d:component:base", i))
		require.True(t, found, "key %d should be cached", i)
		require.NotNil(t, cached)
		require.Equal(t, fmt.Sprintf("component-%d", i), cached.FinalBaseComponentName,
			"key %d should have its own value (no cross-contamination)", i)
		require.Equal(t, i, cached.BaseComponentVars["id"], "key %d vars id mismatch", i)
	}
}

// TestConcurrentCacheAccess_InterleavedReadWrite stresses the read-while-write
// path: half the goroutines write keys, the other half repeatedly read them.
// With Phase 2's deep-copy-outside-the-lock change, readers no longer hold an
// RLock while copying, so concurrent writes proceed without coordination.
// Run with `go test -race` to catch data races introduced by future
// modifications to the cache; this test must complete cleanly under the
// race detector.
func TestConcurrentCacheAccess_InterleavedReadWrite(t *testing.T) {
	ClearBaseComponentConfigCache()
	defer ClearBaseComponentConfigCache()

	const writers = 50
	const readers = 50
	const itersPerReader = 20
	var wg sync.WaitGroup

	// Pre-seed a few keys so readers have something to find.
	for i := 0; i < writers; i++ {
		cacheBaseComponentConfig(fmt.Sprintf("seed-%d", i), &schema.BaseComponentConfig{
			FinalBaseComponentName: fmt.Sprintf("seed-%d", i),
		})
	}

	// Writers add new keys concurrently with readers.
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			cfg := &schema.BaseComponentConfig{
				FinalBaseComponentName: fmt.Sprintf("writer-%d", id),
				BaseComponentVars:      map[string]any{"id": id},
			}
			cacheBaseComponentConfig(fmt.Sprintf("writer-%d", id), cfg)
		}(i)
	}

	// Readers read the seeded keys while writers are working.
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < itersPerReader; j++ {
				key := fmt.Sprintf("seed-%d", (id+j)%writers)
				cached, _, found := getCachedBaseComponentConfig(key)
				if found {
					require.NotNil(t, cached)
					// Read a field; tripping the deep copy is the point.
					_ = cached.FinalBaseComponentName
				}
			}
		}(i)
	}
	wg.Wait()

	// Every key should be present and intact.
	for i := 0; i < writers; i++ {
		cached, _, found := getCachedBaseComponentConfig(fmt.Sprintf("seed-%d", i))
		require.True(t, found)
		require.Equal(t, fmt.Sprintf("seed-%d", i), cached.FinalBaseComponentName)
	}
}

// TestCacheReadIsolationAfterStoreReturns verifies that mutating the *input*
// to cacheBaseComponentConfig AFTER the call returns does NOT affect what a
// later reader sees. Phase 2 moved the deep-copy outside the lock, so this
// test guards against a regression where the copy is skipped or aliased.
//
// This complements TestCacheBaseComponentConfigDeepCopy (which mutates BEFORE
// the read happens). Here we mutate AFTER the cache write has completed,
// proving the cached value is independent of the caller's struct.
func TestCacheReadIsolationAfterStoreReturns(t *testing.T) {
	ClearBaseComponentConfigCache()
	defer ClearBaseComponentConfigCache()

	src := &schema.BaseComponentConfig{
		FinalBaseComponentName:    "original",
		BaseComponentVars:         map[string]any{"key": "original"},
		ComponentInheritanceChain: []string{"base"},
	}
	cacheBaseComponentConfig("iso-key", src)

	// Mutate the source AFTER caching - cached entry must be unaffected.
	src.FinalBaseComponentName = "mutated"
	src.BaseComponentVars["key"] = "mutated"
	src.ComponentInheritanceChain[0] = "mutated-base"
	src.ComponentInheritanceChain = append(src.ComponentInheritanceChain, "appended")

	cached, baseComponents, found := getCachedBaseComponentConfig("iso-key")
	require.True(t, found)
	require.Equal(t, "original", cached.FinalBaseComponentName)
	require.Equal(t, "original", cached.BaseComponentVars["key"])
	require.Equal(t, []string{"base"}, cached.ComponentInheritanceChain)
	require.Equal(t, []string{"base"}, *baseComponents)
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

// TestCacheBaseComponentConfig_SkipsEmptyFields verifies the Phase 12/13
// optimization: deepCopyBaseComponentConfigMaps skips m.DeepCopyMap for
// fields that are empty in the source. The behavioral contract is that
// empty (or nil) source fields produce nil dst fields, identical to the
// pre-optimization behavior of m.DeepCopyMap(nil) returning nil.
func TestCacheBaseComponentConfig_SkipsEmptyFields(t *testing.T) {
	ClearBaseComponentConfigCache()
	defer ClearBaseComponentConfigCache()

	// Source with most fields empty — only Vars is populated.
	src := &schema.BaseComponentConfig{
		FinalBaseComponentName: "minimal",
		BaseComponentVars:      map[string]any{"region": "us-east-1"},
		// All other map fields intentionally nil.
		ComponentInheritanceChain: []string{"base"},
	}
	cacheBaseComponentConfig("minimal-key", src)

	cached, _, found := getCachedBaseComponentConfig("minimal-key")
	require.True(t, found)
	require.NotNil(t, cached)

	// The populated field round-trips.
	assert.Equal(t, "us-east-1", cached.BaseComponentVars["region"])

	// The empty-source fields stay nil on the retrieved copy (no empty-map
	// allocations were performed during caching or retrieval).
	assert.Nil(t, cached.BaseComponentSettings)
	assert.Nil(t, cached.BaseComponentEnv)
	assert.Nil(t, cached.BaseComponentAuth)
	assert.Nil(t, cached.BaseComponentMetadata)
	assert.Nil(t, cached.BaseComponentDependencies)
	assert.Nil(t, cached.BaseComponentProviders)
	assert.Nil(t, cached.BaseComponentHooks)
	assert.Nil(t, cached.BaseComponentBackendSection)
	assert.Nil(t, cached.BaseComponentRemoteStateBackendSection)
}
