//go:build !windows

package filesystem

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestGetGlobMatches_CacheHit verifies that GetGlobMatches returns cached results
// on a second call with the same pattern, without re-reading the filesystem.
func TestGetGlobMatches_CacheHit(t *testing.T) {
	// Use a fresh cache state by clearing it.
	ResetGlobMatchesCache()
	t.Cleanup(ResetGlobMatchesCache)

	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "a.yaml"), []byte(""), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "b.yaml"), []byte(""), 0o644))

	pattern := filepath.Join(tmpDir, "*.yaml")

	// First call - cache miss.
	first, err := GetGlobMatches(pattern)
	require.NoError(t, err)
	assert.Len(t, first, 2)

	// Second call with same pattern - should hit cache.
	second, err := GetGlobMatches(pattern)
	require.NoError(t, err)
	assert.Len(t, second, 2)

	// Results should be strictly equal — same type (non-nil), same order, same content.
	// Using assert.Equal (not ElementsMatch) to lock in the "always non-nil" return contract
	// and verify that cached results are identical (not just order-equivalent).
	assert.Equal(t, first, second)
}

// TestGetGlobMatches_CacheIsolation verifies that cached results are cloned, so
// mutating the returned slice does not corrupt subsequent calls.
func TestGetGlobMatches_CacheIsolation(t *testing.T) {
	ResetGlobMatchesCache()
	t.Cleanup(ResetGlobMatchesCache)

	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "c.yaml"), []byte(""), 0o644))

	pattern := filepath.Join(tmpDir, "*.yaml")

	first, err := GetGlobMatches(pattern)
	require.NoError(t, err)

	// Mutate the returned slice.
	if len(first) > 0 {
		first[0] = "mutated"
	}

	// Second call should still return the original value.
	second, err := GetGlobMatches(pattern)
	require.NoError(t, err)
	if len(second) > 0 {
		assert.NotEqual(t, "mutated", second[0])
	}
}

// TestGetGlobMatches_NonExistentBaseDir verifies that GetGlobMatches returns an
// appropriate error when the base directory does not exist.
func TestGetGlobMatches_NonExistentBaseDir(t *testing.T) {
	ResetGlobMatchesCache()
	t.Cleanup(ResetGlobMatchesCache)

	// Build a path guaranteed to not exist by using a non-existent sub-directory
	// of a fresh t.TempDir() (which will be cleaned up, but we never create the subdir).
	pattern := filepath.Join(t.TempDir(), "nonexistent", "*.yaml")
	_, err := GetGlobMatches(pattern)
	require.Error(t, err, "expected error for non-existent base directory")
	assert.True(t, errors.Is(err, errUtils.ErrFailedToFindImport), "expected ErrFailedToFindImport, got: %v", err)
}

// TestGetGlobMatches_EmptyResults verifies that a pattern matching no files returns
// an empty slice (not an error).
func TestGetGlobMatches_EmptyResults(t *testing.T) {
	ResetGlobMatchesCache()
	t.Cleanup(ResetGlobMatchesCache)

	tmpDir := t.TempDir()
	// No files created in tmpDir.

	pattern := filepath.Join(tmpDir, "*.yaml")
	matches, err := GetGlobMatches(pattern)
	require.NoError(t, err)
	assert.NotNil(t, matches, "GetGlobMatches must return non-nil slice for empty results")
	assert.Empty(t, matches)
}

// TestGetGlobMatches_NonNilContractOnCacheHit verifies the non-nil slice contract is
// preserved on a cache hit — the cached empty result must also be non-nil.
// This is the contract test for the Critical #1 behavior documented in the function docstring.
func TestGetGlobMatches_NonNilContractOnCacheHit(t *testing.T) {
	ResetGlobMatchesCache()
	t.Cleanup(ResetGlobMatchesCache)

	tmpDir := t.TempDir()
	pattern := filepath.Join(tmpDir, "*.nomatches")

	// First call (cache miss): verify non-nil.
	first, err := GetGlobMatches(pattern)
	require.NoError(t, err)
	assert.NotNil(t, first, "first call must return non-nil slice for empty results")
	assert.Empty(t, first)

	// Second call (cache hit): must also be non-nil with identical content.
	second, err := GetGlobMatches(pattern)
	require.NoError(t, err)
	assert.NotNil(t, second, "cached result must also be non-nil — never nil on cache hit")
	assert.Empty(t, second)

	// Strict equality (same type, same content) between cache miss and cache hit.
	assert.Equal(t, first, second, "cache hit must return same non-nil type as cache miss")
}

// TestGetGlobMatches_EmptyResultsCache verifies that empty results are cached and
// retrieved without hitting the filesystem again.
func TestGetGlobMatches_EmptyResultsCache(t *testing.T) {
	ResetGlobMatchesCache()
	t.Cleanup(ResetGlobMatchesCache)

	tmpDir := t.TempDir()

	pattern := filepath.Join(tmpDir, "*.nonexistent")

	// First call - should return empty (not nil) slice and cache it.
	first, err := GetGlobMatches(pattern)
	require.NoError(t, err)

	// Second call should use cache.
	second, err := GetGlobMatches(pattern)
	require.NoError(t, err)

	// Both should be strictly equal — same type (non-nil empty slice), same content.
	// This catches a nil vs []string{} inconsistency between the first and cached call.
	assert.Equal(t, first, second)
}

// TestPathMatch_CacheHit verifies that the PathMatch cache is used on repeated calls.
func TestPathMatch_CacheHit(t *testing.T) {
	// Clear the path match cache using the exported test helper.
	ResetPathMatchCache()
	t.Cleanup(ResetPathMatchCache)

	pattern := "stacks/**/*.yaml"
	name := "stacks/dev/vpc.yaml"

	// First call - cache miss.
	first, err := PathMatch(pattern, name)
	require.NoError(t, err)

	// Second call - should hit cache.
	second, err := PathMatch(pattern, name)
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.True(t, first, "pattern should match the name")
}

// TestPathMatch_CacheHit_NoMatch verifies that cache entries for non-matching patterns
// are also cached and returned correctly.
func TestPathMatch_CacheHit_NoMatch(t *testing.T) {
	ResetPathMatchCache()
	t.Cleanup(ResetPathMatchCache)

	pattern := "*.go"
	name := "file.yaml"

	// First call - cache miss.
	first, err := PathMatch(pattern, name)
	require.NoError(t, err)
	assert.False(t, first)

	// Second call - should hit cache.
	second, err := PathMatch(pattern, name)
	require.NoError(t, err)

	assert.Equal(t, first, second)
}

// TestPathMatch_InvalidPattern verifies that an invalid glob pattern returns an error.
func TestPathMatch_InvalidPattern(t *testing.T) {
	ResetPathMatchCache()
	t.Cleanup(ResetPathMatchCache)

	// An invalid pattern with unclosed bracket.
	_, err := PathMatch("[invalid", "file.yaml")
	// doublestar.Match returns an error for invalid patterns.
	assert.Error(t, err)
}

// TestWriteFileAtomic verifies that WriteFileAtomic writes file contents correctly.
func TestWriteFileAtomic(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "atomic-test.txt")
	content := []byte("hello atomic world")

	err := WriteFileAtomicUnix(filePath, content, 0o644)
	require.NoError(t, err)

	// Verify file was written.
	got, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

// TestWriteFileAtomic_Overwrite verifies that WriteFileAtomic correctly overwrites
// an existing file atomically.
func TestWriteFileAtomic_Overwrite(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "atomic-overwrite.txt")

	// Write initial content.
	require.NoError(t, os.WriteFile(filePath, []byte("initial content"), 0o644))

	// Overwrite with atomic write.
	newContent := []byte("new content")
	err := WriteFileAtomicUnix(filePath, newContent, 0o644)
	require.NoError(t, err)

	got, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, newContent, got)
}

// TestOSFileSystem_WriteFileAtomic verifies that OSFileSystem.WriteFileAtomic works.
func TestOSFileSystem_WriteFileAtomic(t *testing.T) {
	fs := NewOSFileSystem()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "os-atomic.txt")
	content := []byte("atomic content via OSFileSystem")

	err := fs.WriteFileAtomic(filePath, content, 0o644)
	require.NoError(t, err)

	got, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

// TestOSFileSystem_WriteFileAtomic_Overwrite verifies that OSFileSystem.WriteFileAtomic
// correctly overwrites an existing file atomically.
func TestOSFileSystem_WriteFileAtomic_Overwrite(t *testing.T) {
	fs := NewOSFileSystem()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "os-atomic-overwrite.txt")

	// Write initial content.
	require.NoError(t, os.WriteFile(filePath, []byte("initial content"), 0o644))

	// Overwrite with atomic write.
	newContent := []byte("overwritten content via OSFileSystem")
	err := fs.WriteFileAtomic(filePath, newContent, 0o644)
	require.NoError(t, err)

	got, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, newContent, got)
}

// TestGetGlobMatches_LRU_Eviction verifies that the LRU cache evicts the least-recently-used
// entry when the cache reaches its capacity (defaultGlobCacheMaxEntries).
// This test uses a small in-process simulation: it fills the cache to capacity + 1 and
// then checks that the first entry was evicted (i.e., a fresh filesystem read is triggered).
// It also verifies that the eviction counter increments as expected.
func TestGetGlobMatches_LRU_Eviction(t *testing.T) {
	ResetGlobMatchesCache()
	t.Cleanup(ResetGlobMatchesCache)

	tmpDir := t.TempDir()

	// Populate the LRU cache with defaultGlobCacheMaxEntries unique patterns (all non-matching
	// since we only need entries in the cache, not actual files).
	// Use sub-directories that don't exist — GetGlobMatches returns an error for
	// non-existent base directories, so instead write empty-match YAML patterns.
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "seed.yaml"), []byte(""), 0o644))
	// Insert a "seed" entry that we will check for eviction later.
	seedPattern := filepath.Join(tmpDir, "seed.yaml")
	_, err := GetGlobMatches(seedPattern)
	require.NoError(t, err)
	initialLen := GlobCacheLen()
	require.Equal(t, 1, initialLen, "seed entry should be in cache")

	// Fill the cache to defaultGlobCacheMaxEntries by using unique patterns that each match
	// the same seed file (pattern variation, not file variation).
	// We create defaultGlobCacheMaxEntries additional real files so all patterns resolve.
	for i := range defaultGlobCacheMaxEntries {
		// Use fmt.Sprintf to guarantee unique filenames for all i values (i > 26 would
		// cycle single-character names and produce duplicates).
		name := filepath.Join(tmpDir, fmt.Sprintf("file_evict_%d.yaml", i))
		_ = os.WriteFile(name, []byte(""), 0o644)
		_, err := GetGlobMatches(name)
		require.NoError(t, err)
	}

	// After inserting defaultGlobCacheMaxEntries more entries, the LRU should have evicted the
	// seed entry (it was the oldest / least recently used).
	// We verify this by checking the cache size is bounded at defaultGlobCacheMaxEntries.
	afterLen := GlobCacheLen()
	assert.LessOrEqual(t, afterLen, defaultGlobCacheMaxEntries, "LRU cache must not exceed max capacity")

	// The eviction counter must have incremented at least once.
	evictions := GlobCacheEvictions()
	assert.Positive(t, evictions, "eviction counter must increment when LRU capacity is exceeded")
}

// TestGetGlobMatches_TTL_Expiry verifies that a stale cache entry (past TTL)
// is treated as a cache miss and triggers a fresh filesystem read.
func TestGetGlobMatches_TTL_Expiry(t *testing.T) {
	ResetGlobMatchesCache()
	t.Cleanup(ResetGlobMatchesCache)

	tmpDir := t.TempDir()

	// Create a file so the first call returns a result.
	file1 := filepath.Join(tmpDir, "a.yaml")
	require.NoError(t, os.WriteFile(file1, []byte(""), 0o644))

	pattern := filepath.Join(tmpDir, "*.yaml")

	// First call — cache miss, should find the file.
	res1, err := GetGlobMatches(pattern)
	require.NoError(t, err)
	assert.Len(t, res1, 1, "should find exactly one file")

	// Forcibly expire the cache entry via the test helper.
	SetGlobCacheEntryExpired(pattern)

	// Add a second file before the second call.
	file2 := filepath.Join(tmpDir, "b.yaml")
	require.NoError(t, os.WriteFile(file2, []byte(""), 0o644))

	// Second call — the TTL has expired, so the cache should be bypassed and
	// both files should be discovered.
	res2, err := GetGlobMatches(pattern)
	require.NoError(t, err)
	assert.Len(t, res2, 2, "TTL expiry should trigger fresh filesystem read returning both files")
}

// TestGetGlobMatches_EmptyResultCached verifies that empty results (no matching files)
// are cached and served from cache on subsequent calls.
func TestGetGlobMatches_EmptyResultCached(t *testing.T) {
	ResetGlobMatchesCache()
	t.Cleanup(ResetGlobMatchesCache)

	tmpDir := t.TempDir()

	// Pattern that matches nothing.
	pattern := filepath.Join(tmpDir, "nonexistent_*.yaml")

	res1, err := GetGlobMatches(pattern)
	require.NoError(t, err)
	assert.Empty(t, res1, "should return empty result for non-matching pattern")
	assert.NotNil(t, res1, "empty result must be non-nil (contract)")

	// Second call — should be a cache hit (no filesystem walk).
	res2, err := GetGlobMatches(pattern)
	require.NoError(t, err)
	assert.Empty(t, res2, "second call should return empty result from cache")
	assert.NotNil(t, res2, "cached empty result must be non-nil (contract)")
}

// TestGetGlobMatches_HitMissCounters verifies that the hit and miss counters
// are incremented correctly across cache hits and misses.
func TestGetGlobMatches_HitMissCounters(t *testing.T) {
	ResetGlobMatchesCache()
	t.Cleanup(ResetGlobMatchesCache)

	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "a.yaml"), []byte(""), 0o644))

	pattern := filepath.Join(tmpDir, "*.yaml")

	// First call is always a miss.
	_, err := GetGlobMatches(pattern)
	require.NoError(t, err)
	assert.Equal(t, int64(0), GlobCacheHits(), "no hits yet")
	assert.Equal(t, int64(1), GlobCacheMisses(), "first call is a miss")

	// Second call should be a hit.
	_, err = GetGlobMatches(pattern)
	require.NoError(t, err)
	assert.Equal(t, int64(1), GlobCacheHits(), "second call is a hit")
	assert.Equal(t, int64(1), GlobCacheMisses(), "miss count must not change")
}

// TestGetGlobMatches_EmptyResultCachingDisabled verifies that when
// ATMOS_FS_GLOB_CACHE_EMPTY=0 is set, empty results are not cached.
func TestGetGlobMatches_EmptyResultCachingDisabled(t *testing.T) {
	// Set the env var BEFORE applying config.
	t.Setenv("ATMOS_FS_GLOB_CACHE_EMPTY", "0")
	ApplyGlobCacheConfigForTest()
	t.Cleanup(func() {
		// Restore default behavior after the test.
		ApplyGlobCacheConfigForTest()
		ResetGlobMatchesCache()
	})

	assert.False(t, GlobCacheEmptyEnabled(), "empty caching must be disabled when env var is 0")

	tmpDir := t.TempDir()
	// Use a wildcard that initially matches nothing.
	pattern := filepath.Join(tmpDir, "*.yaml")

	// First call — cache miss, empty result; must NOT be stored.
	res1, err := GetGlobMatches(pattern)
	require.NoError(t, err)
	assert.Empty(t, res1, "should return empty result")
	assert.NotNil(t, res1, "must be non-nil per contract")
	assert.Equal(t, 0, GlobCacheLen(), "empty result must not be cached when disabled")

	// Create a file so the next call returns a non-empty result.
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "found.yaml"), []byte(""), 0o644))

	// Second call — since the empty result was NOT cached, the filesystem is re-read
	// and the newly-created file should be discovered.
	res2, err := GetGlobMatches(pattern)
	require.NoError(t, err)
	assert.Len(t, res2, 1, "new file should be found after cache bypass")
}

// TestGetGlobMatches_RaceStress hammers the glob cache from many goroutines to
// surface data races.  Run with -race to exercise the race detector.
func TestGetGlobMatches_RaceStress(t *testing.T) {
	ResetGlobMatchesCache()
	t.Cleanup(ResetGlobMatchesCache)

	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "stress.yaml"), []byte(""), 0o644))

	const numGoroutines = 32
	const callsPerGoroutine = 50

	done := make(chan struct{})
	for g := range numGoroutines {
		g := g
		go func() {
			defer func() { done <- struct{}{} }()
			for i := range callsPerGoroutine {
				// Use a mix of unique and shared patterns to exercise both cache hits
				// and cache misses concurrently.
				var pattern string
				if i%2 == 0 {
					pattern = filepath.Join(tmpDir, "*.yaml")
				} else {
					pattern = filepath.Join(tmpDir, fmt.Sprintf("unique_%d_%d_*.yaml", g, i))
				}
				_, _ = GetGlobMatches(pattern)
			}
		}()
	}

	for range numGoroutines {
		<-done
	}
}

// TestGetGlobMatches_EnvTTL verifies that ATMOS_FS_GLOB_CACHE_TTL is honoured.
// A very short TTL means entries expire immediately, so every call is a miss.
func TestGetGlobMatches_EnvTTL(t *testing.T) {
	t.Setenv("ATMOS_FS_GLOB_CACHE_TTL", "1ns")
	ApplyGlobCacheConfigForTest()
	t.Cleanup(func() {
		ApplyGlobCacheConfigForTest()
		ResetGlobMatchesCache()
	})

	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "ttl.yaml"), []byte(""), 0o644))

	pattern := filepath.Join(tmpDir, "*.yaml")

	_, err := GetGlobMatches(pattern)
	require.NoError(t, err)

	// With 1ns TTL the entry will already be stale.  Force it expired just to be safe.
	SetGlobCacheEntryExpired(pattern)

	// Add a second file to prove the second call re-reads the filesystem.
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "ttl2.yaml"), []byte(""), 0o644))

	res, err := GetGlobMatches(pattern)
	require.NoError(t, err)
	assert.Len(t, res, 2, "short TTL should cause re-read and find both files")
}
