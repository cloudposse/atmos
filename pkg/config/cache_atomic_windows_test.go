//go:build windows
// +build windows

package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAtomicFileWrites_Windows tests basic cache operations on Windows.
// Full atomic write testing with concurrent goroutines is disabled on Windows
// due to different file locking behavior, but we verify basic cache functionality.
func TestAtomicFileWrites_Windows(t *testing.T) {
	// Create a temporary cache directory and set up XDG with proper synchronization.
	testDir := t.TempDir()
	cleanup := withTestXDGHome(t, testDir)
	t.Cleanup(cleanup)

	// Create initial cache with known values.
	initialCache := CacheConfig{
		LastChecked:              1000,
		InstallationId:           "atomic-test-windows",
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

	// Load the cache and verify it matches what we saved.
	loaded, err := LoadCache()
	assert.NoError(t, err)
	assert.Equal(t, initialCache, loaded)
}
