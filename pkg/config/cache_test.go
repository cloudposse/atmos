package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetCacheFilePathWithXDGCacheHome(t *testing.T) {
	// Save original XDG_CACHE_HOME
	originalXDG := os.Getenv("XDG_CACHE_HOME")
	defer os.Setenv("XDG_CACHE_HOME", originalXDG)

	// Test with XDG_CACHE_HOME set
	testDir := t.TempDir()
	os.Setenv("XDG_CACHE_HOME", testDir)

	path, err := GetCacheFilePath()
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(testDir, "atmos", "cache.yaml"), path)
	assert.True(t, strings.HasPrefix(path, testDir))
}

func TestGetCacheFilePathWithoutXDGCacheHome(t *testing.T) {
	// Save original XDG_CACHE_HOME
	originalXDG := os.Getenv("XDG_CACHE_HOME")
	defer os.Setenv("XDG_CACHE_HOME", originalXDG)

	// Clear XDG_CACHE_HOME to test default behavior
	os.Unsetenv("XDG_CACHE_HOME")

	path, err := GetCacheFilePath()
	assert.NoError(t, err)

	// Should use $HOME/.cache/atmos/cache.yaml by default
	homeDir, _ := os.UserHomeDir()
	expectedPath := filepath.Join(homeDir, ".cache", "atmos", "cache.yaml")
	assert.Equal(t, expectedPath, path)
}

func TestCacheSharedBetweenVersionAndTelemetry(t *testing.T) {
	// This test verifies that the cache structure supports both
	// version checking (LastChecked) and telemetry disclosure
	// (TelemetryDisclosureShown) in the same cache file

	// Create a test cache directory
	testDir := t.TempDir()
	originalXDG := os.Getenv("XDG_CACHE_HOME")
	defer os.Setenv("XDG_CACHE_HOME", originalXDG)
	os.Setenv("XDG_CACHE_HOME", testDir)

	// Create a cache with both fields set
	cache := CacheConfig{
		LastChecked:              1234567890,
		InstallationId:           "test-id",
		TelemetryDisclosureShown: true,
	}

	// Save the cache
	err := SaveCache(cache)
	assert.NoError(t, err)

	// Load it back
	loadedCache, err := LoadCache()
	assert.NoError(t, err)

	// Verify both fields are preserved
	assert.Equal(t, int64(1234567890), loadedCache.LastChecked)
	assert.Equal(t, "test-id", loadedCache.InstallationId)
	assert.True(t, loadedCache.TelemetryDisclosureShown)
}
