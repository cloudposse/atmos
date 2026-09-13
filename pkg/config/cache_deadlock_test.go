//go:build !windows

package config

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/cache"
)

// TestCacheFileLockDeadlock tests that file locking doesn't cause deadlocks.
func TestCacheFileLockDeadlock(t *testing.T) {
	// Create a temporary cache directory and set up XDG with proper synchronization.
	testDir := t.TempDir()
	cleanup := withTestXDGHome(t, testDir)
	t.Cleanup(cleanup)

	// Create initial cache.
	initialCache := CacheConfig{
		LastChecked:    1000,
		InstallationId: "deadlock-test",
	}
	err := SaveCache(initialCache)
	require.NoError(t, err)

	// Test concurrent operations with timeout.
	done := make(chan bool)
	timeout := time.After(5 * time.Second)

	go func() {
		// Simulate multiple goroutines trying to access cache.
		var wg sync.WaitGroup
		successCount := 0
		var mu sync.Mutex

		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				// Try to load cache.
				// LoadCache uses TryRLock so it may return empty config if locked.
				_, err := LoadCache()
				assert.NoError(t, err, "LoadCache failed for goroutine %d", id)

				// Try to update cache.
				// UpdateCache may fail if lock can't be acquired within retry limit.
				// This is expected behavior to prevent deadlocks.
				err = UpdateCache(func(cache *CacheConfig) {
					cache.LastChecked = int64(2000 + id)
				})

				if err == nil {
					mu.Lock()
					successCount++
					mu.Unlock()
				} else {
					// Lock contention is expected when multiple goroutines compete.
					// Verify that the error is our sentinel error.
					assert.True(t, errors.Is(err, errUtils.ErrCacheLocked),
						"Expected ErrCacheLocked sentinel error for goroutine %d, got: %v", id, err)
					assert.Contains(t, err.Error(), "cache file is locked by another process",
						"Unexpected error message for goroutine %d: %v", id, err)
				}
			}(i)
		}
		wg.Wait()

		// At least one goroutine should succeed.
		assert.Greater(t, successCount, 0, "At least one goroutine should successfully update the cache")
		t.Logf("Successfully updated cache in %d out of 5 goroutines", successCount)

		done <- true
	}()

	select {
	case <-done:
		t.Log("All concurrent operations completed successfully")
	case <-timeout:
		t.Fatal("Test timed out - possible deadlock in cache file locking")
	}
}

// TestCacheFileLockTimeoutBehavior tests the timeout behavior of file locking.
func TestCacheFileLockTimeoutBehavior(t *testing.T) {
	testDir := t.TempDir()
	cleanup := withTestXDGHome(t, testDir)
	t.Cleanup(cleanup)

	cacheFile, err := GetCacheFilePath()
	require.NoError(t, err)

	// Create a test that simulates what happens during a lock timeout.
	// This should complete within a reasonable time (5 seconds max for lock acquisition).
	start := time.Now()
	lock := cache.NewFileLock(cacheFile)
	err = lock.WithLock(func() error {
		// Simulate some work.
		time.Sleep(100 * time.Millisecond)
		return nil
	})
	elapsed := time.Since(start)

	assert.NoError(t, err)
	assert.Less(t, elapsed, 6*time.Second, "Lock acquisition took too long: %v", elapsed)
}

// TestLoadCacheNonBlockingWithLockedFile tests that LoadCache doesn't block indefinitely
// when the file is locked.
func TestLoadCacheNonBlockingWithLockedFile(t *testing.T) {
	testDir := t.TempDir()
	cleanup := withTestXDGHome(t, testDir)
	t.Cleanup(cleanup)

	// Create initial cache.
	initialCache := CacheConfig{
		LastChecked:    1000,
		InstallationId: "non-blocking-test",
	}
	err := SaveCache(initialCache)
	require.NoError(t, err)

	cacheFile, err := GetCacheFilePath()
	require.NoError(t, err)

	var loaded CacheConfig
	var loadErr error
	loadedCh := make(chan struct{})
	lock := cache.NewFileLock(cacheFile)
	err = lock.WithLock(func() error {
		go func() {
			defer close(loadedCh)
			loaded, loadErr = LoadCache()
		}()

		// Keep the write lock until the read completes. This tests that the
		// best-effort read proceeds under contention, independently of CI speed.
		select {
		case <-loadedCh:
		case <-time.After(5 * time.Second):
			t.Error("LoadCache did not finish while the write lock was held")
		}
		return nil
	})
	require.NoError(t, err)

	// Join the reader after releasing the lock, including on the timeout path,
	// so it cannot outlive the test's temporary cache directory.
	select {
	case <-loadedCh:
		require.NoError(t, loadErr)
		assert.Equal(t, initialCache, loaded)
	case <-time.After(5 * time.Second):
		t.Fatal("LoadCache did not finish after the write lock was released")
	}
}
