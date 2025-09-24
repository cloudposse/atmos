package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adrg/xdg"
	"github.com/stretchr/testify/assert"
)

func TestGetCacheFilePathWithXDGCacheHome(t *testing.T) {
	// Save original XDG_CACHE_HOME.
	originalXDG := os.Getenv("XDG_CACHE_HOME")
	defer os.Setenv("XDG_CACHE_HOME", originalXDG)

	// Test with XDG_CACHE_HOME set.
	testDir := t.TempDir()
	os.Setenv("XDG_CACHE_HOME", testDir)

	// Reload XDG to pick up the environment change.
	xdg.Reload()

	path, err := GetCacheFilePath()
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(testDir, "atmos", "cache.yaml"), path)
	assert.True(t, strings.HasPrefix(path, testDir))
}

func TestGetCacheFilePathWithoutXDGCacheHome(t *testing.T) {
	// Save original XDG_CACHE_HOME.
	originalXDG := os.Getenv("XDG_CACHE_HOME")
	defer os.Setenv("XDG_CACHE_HOME", originalXDG)

	// Clear XDG_CACHE_HOME to test default behavior.
	os.Unsetenv("XDG_CACHE_HOME")

	// Reload XDG to pick up the environment change.
	xdg.Reload()

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
	originalXDG := os.Getenv("XDG_CACHE_HOME")
	defer os.Setenv("XDG_CACHE_HOME", originalXDG)
	os.Setenv("XDG_CACHE_HOME", testDir)

	// Reload XDG to pick up the environment change.
	xdg.Reload()

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
	originalXDG := os.Getenv("XDG_CACHE_HOME")
	defer os.Setenv("XDG_CACHE_HOME", originalXDG)
	os.Setenv("XDG_CACHE_HOME", testDir)

	// Reload XDG to pick up the environment change.
	xdg.Reload()

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
	originalXDG := os.Getenv("XDG_CACHE_HOME")
	defer os.Setenv("XDG_CACHE_HOME", originalXDG)
	os.Setenv("XDG_CACHE_HOME", cacheDir)

	// Reload XDG to pick up the environment change.
	xdg.Reload()

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
	testDir := t.TempDir()
	originalXDG := os.Getenv("XDG_CACHE_HOME")
	defer os.Setenv("XDG_CACHE_HOME", originalXDG)
	os.Setenv("XDG_CACHE_HOME", testDir)

	// Reload XDG to pick up the environment change..
	xdg.Reload()

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
	// Both updates should be applied when using UpdateCache.
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
