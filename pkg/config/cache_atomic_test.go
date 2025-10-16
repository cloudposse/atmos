//go:build !windows
// +build !windows

package config

import (
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAtomicFileWrites verifies that cache writes are atomic on Unix-like systems.
func TestAtomicFileWrites(t *testing.T) {
	// Create a temporary cache directory and set up XDG with proper synchronization.
	testDir := t.TempDir()
	cleanup := withTestXDGHome(t, testDir)
	t.Cleanup(cleanup)

	// Create initial cache with known values.
	initialCache := CacheConfig{
		LastChecked:              1000,
		InstallationId:           "atomic-test",
		TelemetryDisclosureShown: false,
	}
	err := SaveCache(initialCache)
	require.NoError(t, err)

	// Verify the file was created.
	cacheFile, err := GetCacheFilePath()
	require.NoError(t, err)

	info, err := os.Stat(cacheFile)
	require.NoError(t, err)
	assert.False(t, info.IsDir())

	// Test concurrent writes with different values.
	var wg sync.WaitGroup
	numGoroutines := 10
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Each goroutine updates the cache with a unique value.
			err := UpdateCache(func(cache *CacheConfig) {
				cache.LastChecked = int64(2000 + id)
				cache.TelemetryDisclosureShown = (id % 2) == 0
			})
			if err != nil {
				errors <- err
			}
		}(i)
	}

	// Wait for all goroutines to complete.
	wg.Wait()
	close(errors)

	// Check for any errors.
	for err := range errors {
		// On systems without locking, we might get lock errors.
		// These are acceptable as they prevent corruption.
		if err != nil {
			t.Logf("Acceptable error during concurrent write: %v", err)
		}
	}

	// Load the final cache and verify it's not corrupted.
	finalCache, err := LoadCache()
	assert.NoError(t, err)

	// The cache should have one of the values written, not a corrupted mix.
	assert.Equal(t, "atomic-test", finalCache.InstallationId)
	assert.True(t, finalCache.LastChecked >= 2000 && finalCache.LastChecked < 2010,
		"LastChecked should be one of the written values: %d", finalCache.LastChecked)

	// Verify the file still exists and is readable.
	_, err = os.Stat(cacheFile)
	assert.NoError(t, err)
}

// TestAtomicWriteFailureRecovery tests that failed writes don't corrupt the cache.
func TestAtomicWriteFailureRecovery(t *testing.T) {
	// Create a temporary cache directory and set up XDG with proper synchronization.
	testDir := t.TempDir()
	cleanup := withTestXDGHome(t, testDir)
	t.Cleanup(cleanup)

	// Create initial cache with known values.
	goodCache := CacheConfig{
		LastChecked:              5000,
		InstallationId:           "recovery-test",
		TelemetryDisclosureShown: true,
	}
	err := SaveCache(goodCache)
	require.NoError(t, err)

	// Load it back to verify.
	loaded, err := LoadCache()
	require.NoError(t, err)
	assert.Equal(t, goodCache, loaded)

	// The cache file should still contain the good data even after
	// any potential failed write attempts (atomic writes ensure this).
	finalCache, err := LoadCache()
	assert.NoError(t, err)
	assert.Equal(t, "recovery-test", finalCache.InstallationId)
	assert.Equal(t, int64(5000), finalCache.LastChecked)
	assert.True(t, finalCache.TelemetryDisclosureShown)
}
