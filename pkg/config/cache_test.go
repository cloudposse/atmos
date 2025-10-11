package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/adrg/xdg"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/stretchr/testify/assert"
)

func TestGetCacheFilePathWithXDGCacheHome(t *testing.T) {
	// Test with XDG_CACHE_HOME set.
	testDir := t.TempDir()
	cleanup := withTestXDGHome(t, testDir)
	defer cleanup()

	path, err := GetCacheFilePath()
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(testDir, "atmos", "cache.yaml"), path)
	assert.True(t, strings.HasPrefix(path, testDir))
}

func TestGetCacheFilePathWithoutXDGCacheHome(t *testing.T) {
	// Clear XDG_CACHE_HOME to test default behavior.
	cleanup := withTestXDGHome(t, "")
	defer cleanup()
	os.Unsetenv("XDG_CACHE_HOME")

	path, err := GetCacheFilePath()
	assert.NoError(t, err)

	// The XDG library will use the appropriate default for the OS.
	// On Unix-like systems: $HOME/.cache/atmos/cache.yaml.
	// On Windows: %LOCALAPPDATA%/atmos/cache.yaml.
	// On macOS: $HOME/Library/Caches/atmos/cache.yaml.
	expectedPath := filepath.Join(xdg.CacheHome, "atmos", "cache.yaml")
	assert.Equal(t, expectedPath, path)
}

func TestCacheSharedBetweenVersionAndTelemetry(t *testing.T) {
	// This test verifies that the cache structure supports both
	// version checking (LastChecked) and telemetry disclosure
	// (TelemetryDisclosureShown) in the same cache file.

	// Create a test cache directory.
	testDir := t.TempDir()
	cleanup := withTestXDGHome(t, testDir)
	defer cleanup()

	// Create a cache with both fields set.
	cache := CacheConfig{
		LastChecked:              1234567890,
		InstallationId:           "test-id",
		TelemetryDisclosureShown: true,
	}

	// Save the cache.
	err := SaveCache(cache)
	assert.NoError(t, err)

	// Load it back.
	loadedCache, err := LoadCache()
	assert.NoError(t, err)

	// Verify both fields are preserved.
	assert.Equal(t, int64(1234567890), loadedCache.LastChecked)
	assert.Equal(t, "test-id", loadedCache.InstallationId)
	assert.True(t, loadedCache.TelemetryDisclosureShown)
}

func TestLoadCacheNonExistent(t *testing.T) {
	// Test loading a cache when file doesn't exist.
	testDir := t.TempDir()
	cleanup := withTestXDGHome(t, testDir)
	defer cleanup()

	// Load non-existent cache should return empty config without error.
	cache, err := LoadCache()
	assert.NoError(t, err)
	assert.Equal(t, int64(0), cache.LastChecked)
	assert.Empty(t, cache.InstallationId)
	assert.False(t, cache.TelemetryDisclosureShown)
}

func TestSaveCacheCreatesDirectory(t *testing.T) {
	// Test that SaveCache creates the directory if it doesn't exist.
	testDir := t.TempDir()
	// Use a subdirectory that doesn't exist yet.
	cacheDir := filepath.Join(testDir, "subdir")
	cleanup := withTestXDGHome(t, cacheDir)
	defer cleanup()

	cache := CacheConfig{
		LastChecked:              9876543210,
		InstallationId:           "test-create-dir",
		TelemetryDisclosureShown: true,
	}

	// Save should create the directory.
	err := SaveCache(cache)
	assert.NoError(t, err)

	// Verify directory was created.
	expectedDir := filepath.Join(cacheDir, "atmos")
	assert.DirExists(t, expectedDir)

	// Verify cache file was created.
	expectedFile := filepath.Join(expectedDir, "cache.yaml")
	assert.FileExists(t, expectedFile)
}

func TestConcurrentCacheAccess(t *testing.T) {
	// Test concurrent reads and writes to the cache.
	if runtime.GOOS == "windows" {
		t.Skip("Skipping concurrent cache test on Windows: file locking is disabled")
	}

	testDir := t.TempDir()
	cleanup := withTestXDGHome(t, testDir)
	defer cleanup()

	// Create initial cache.
	initialCache := CacheConfig{
		LastChecked:              1000,
		InstallationId:           "concurrent-test",
		TelemetryDisclosureShown: false,
	}
	err := SaveCache(initialCache)
	assert.NoError(t, err)

	// Run concurrent operations.
	done := make(chan bool, 3)
	errsChannel := make(chan error, 3)

	// Writer 1: Update LastChecked.
	go func() {
		err := UpdateCache(func(cache *CacheConfig) {
			cache.LastChecked = 2000
		})
		if err != nil {
			errsChannel <- err
		}
		done <- true
	}()

	// Writer 2: Update TelemetryDisclosureShown.
	go func() {
		err := UpdateCache(func(cache *CacheConfig) {
			cache.TelemetryDisclosureShown = true
		})
		if err != nil {
			errsChannel <- err
		}
		done <- true
	}()

	// Reader: Read multiple times.
	go func() {
		for i := 0; i < 5; i++ {
			_, err := LoadCache()
			if err != nil {
				errsChannel <- err
				done <- true
				return
			}
		}
		done <- true
	}()

	// Wait for all goroutines.
	for i := 0; i < 3; i++ {
		<-done
	}

	// Check for errors.
	close(errsChannel)
	for err := range errsChannel {
		assert.NoError(t, err)
	}

	// Final cache should have both updates.
	finalCache, err := LoadCache()
	assert.NoError(t, err)
	assert.Equal(t, "concurrent-test", finalCache.InstallationId)
	// With proper locking, both updates should be applied.
	assert.Equal(t, int64(2000), finalCache.LastChecked)
	assert.True(t, finalCache.TelemetryDisclosureShown)
}

func TestShouldCheckForUpdates(t *testing.T) {
	now := int64(1000000)

	tests := []struct {
		name        string
		lastChecked int64
		frequency   string
		expected    bool
	}{
		{"Daily check - due", now - 86401, "daily", true},
		{"Daily check - not due", now - 86399, "daily", false},
		{"Hourly check - due", now - 3601, "hourly", true},
		{"Hourly check - not due", now - 3599, "hourly", false},
		{"Weekly check - due", now - 604801, "weekly", true},
		{"Weekly check - not due", now - 604799, "weekly", false},
		{"Custom seconds - due", now - 301, "300", true},
		{"Custom seconds - not due", now - 299, "300", false},
		{"Custom duration - due", now - 121, "2m", true},
		{"Custom duration - not due", now - 119, "2m", false},
		{"Invalid frequency defaults to daily", now - 86401, "invalid", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Use the test helper with a fixed "now" time
			result := shouldCheckForUpdatesAt(tc.lastChecked, tc.frequency, now)
			if result != tc.expected {
				t.Errorf("shouldCheckForUpdatesAt(%d, %q, %d) = %v; want %v",
					tc.lastChecked, tc.frequency, now, result, tc.expected)
			}
		})
	}
}

func TestShouldCheckForUpdatesPublicAPI(t *testing.T) {
	// Test the public ShouldCheckForUpdates function.
	// This uses the current time, so we need to be careful with our assertions.

	// Check that very old timestamp triggers update check.
	veryOldTimestamp := int64(0)
	assert.True(t, ShouldCheckForUpdates(veryOldTimestamp, "daily"))
	assert.True(t, ShouldCheckForUpdates(veryOldTimestamp, "hourly"))
	assert.True(t, ShouldCheckForUpdates(veryOldTimestamp, "60")) // 60 seconds

	// Check that very recent timestamp doesn't trigger update.
	// Use current time minus 1 second.
	veryRecentTimestamp := time.Now().Unix() - 1
	assert.False(t, ShouldCheckForUpdates(veryRecentTimestamp, "daily"))
	assert.False(t, ShouldCheckForUpdates(veryRecentTimestamp, "hourly"))
	assert.False(t, ShouldCheckForUpdates(veryRecentTimestamp, "60")) // 60 seconds
}

func TestParseFrequency(t *testing.T) {
	tests := []struct {
		name      string
		frequency string
		expected  int64
		hasError  bool
	}{
		// Integer seconds
		{"Integer seconds", "300", 300, false},
		{"Integer seconds with spaces", "  300  ", 300, false},
		{"Zero seconds", "0", 0, true},
		{"Negative seconds", "-100", 0, true},

		// Duration with suffix
		{"Seconds suffix", "30s", 30, false},
		{"Minutes suffix", "5m", 300, false},
		{"Hours suffix", "2h", 7200, false},
		{"Days suffix", "1d", 86400, false},
		{"Invalid suffix", "10x", 0, true},
		{"Invalid number with suffix", "abcm", 0, true},
		{"Zero with suffix", "0m", 0, true},
		{"Negative with suffix", "-5m", 0, true},

		// Predefined keywords
		{"Minute keyword", "minute", 60, false},
		{"Hourly keyword", "hourly", 3600, false},
		{"Daily keyword", "daily", 86400, false},
		{"Weekly keyword", "weekly", 604800, false},
		{"Monthly keyword", "monthly", 2592000, false},
		{"Yearly keyword", "yearly", 31536000, false},

		// Invalid inputs
		{"Invalid keyword", "invalid", 0, true},
		{"Empty string", "", 0, true},
		{"Random text", "abc123", 0, true},
		{"Single character", "d", 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseFrequency(tc.frequency)
			if tc.hasError {
				assert.Error(t, err, "Expected error for frequency %q", tc.frequency)
			} else {
				assert.NoError(t, err, "Unexpected error for frequency %q", tc.frequency)
				assert.Equal(t, tc.expected, result, "Incorrect value for frequency %q", tc.frequency)
			}
		})
	}
}

func TestLoadCacheWithCorruptedFile(t *testing.T) {
	// Test loading a cache when the file is corrupted.
	testDir := t.TempDir()
	cleanup := withTestXDGHome(t, testDir)
	defer cleanup()

	// Create a corrupted cache file.
	cacheDir := filepath.Join(testDir, "atmos")
	err := os.MkdirAll(cacheDir, 0o755)
	assert.NoError(t, err)

	cacheFile := filepath.Join(cacheDir, "cache.yaml")
	err = os.WriteFile(cacheFile, []byte("not valid yaml: {[}"), 0o644)
	assert.NoError(t, err)

	// Load should return an error for corrupted file.
	// On Windows, errors are ignored so it returns empty config.
	cfg, err := LoadCache()
	if runtime.GOOS == "windows" {
		// On Windows, cache read errors are ignored.
		assert.NoError(t, err)
		// Should return empty/default config.
		assert.Equal(t, CacheConfig{}, cfg)
	} else {
		// On Unix, should return an error.
		assert.Error(t, err)
		if err != nil {
			assert.Contains(t, err.Error(), "cache read failed")
		}
	}
}

func TestGetCacheFilePathWithDirectoryCreationError(t *testing.T) {
	// Test GetCacheFilePath when directory creation fails.
	// This is harder to test without mocking, but we can test with read-only parent.
	if runtime.GOOS == "windows" {
		t.Skip("Skipping read-only directory test on Windows: permission model differs")
	}
	if os.Geteuid() == 0 {
		t.Skip("Skipping test when running as root")
	}

	testDir := t.TempDir()

	// Create a read-only directory.
	readOnlyDir := filepath.Join(testDir, "readonly")
	err := os.MkdirAll(readOnlyDir, 0o555) // Read and execute only
	assert.NoError(t, err)

	// Set XDG_CACHE_HOME to a subdirectory of the read-only directory.
	cleanup := withTestXDGHome(t, filepath.Join(readOnlyDir, "subdir"))
	defer cleanup()

	// GetCacheFilePath should return an error when it can't create the directory.
	_, err = GetCacheFilePath()
	assert.Error(t, err)
	if err != nil {
		assert.ErrorIs(t, err, errUtils.ErrCacheDir)
	}
}

func TestUpdateCacheWithNonExistentFile(t *testing.T) {
	// Test UpdateCache when the cache file doesn't exist yet.
	testDir := t.TempDir()
	cleanup := withTestXDGHome(t, testDir)
	defer cleanup()

	// Update non-existent cache should create it.
	err := UpdateCache(func(cache *CacheConfig) {
		cache.LastChecked = 5000
		cache.InstallationId = "new-install"
		cache.TelemetryDisclosureShown = true
	})
	assert.NoError(t, err)

	// Verify the cache was created with the correct values.
	loadedCache, err := LoadCache()
	assert.NoError(t, err)
	assert.Equal(t, int64(5000), loadedCache.LastChecked)
	assert.Equal(t, "new-install", loadedCache.InstallationId)
	assert.True(t, loadedCache.TelemetryDisclosureShown)
}

func TestWithCacheFileLockTimeout(t *testing.T) {
	// Test that file lock acquisition times out appropriately.
	// This test simulates a scenario where the lock is held for a long time.
	testDir := t.TempDir()
	cleanup := withTestXDGHome(t, testDir)
	defer cleanup()

	// Create initial cache.
	initialCache := CacheConfig{
		LastChecked:    1000,
		InstallationId: "lock-test",
	}
	err := SaveCache(initialCache)
	assert.NoError(t, err)

	cacheFile, err := GetCacheFilePath()
	assert.NoError(t, err)

	// We can't easily test the timeout without modifying the withCacheFileLock function,
	// but we can test that multiple operations work correctly.
	done := make(chan bool, 2)
	errors := make(chan error, 2)

	// Start two concurrent operations.
	for i := 0; i < 2; i++ {
		go func() {
			err := withCacheFileLock(cacheFile, func() error {
				// Simulate some work.
				time.Sleep(10 * time.Millisecond)
				return nil
			})
			if err != nil {
				errors <- err
			}
			done <- true
		}()
	}

	// Wait for both to complete.
	for i := 0; i < 2; i++ {
		<-done
	}

	close(errors)
	for err := range errors {
		assert.NoError(t, err)
	}
}
