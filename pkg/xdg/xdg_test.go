package xdg

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetXDGCacheDir(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tempHome, ".cache"))

	dir, err := GetXDGCacheDir("test", 0o755)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tempHome, ".cache", "atmos", "test"), dir)

	// Verify directory was created.
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestGetXDGDataDir(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(tempHome, ".local", "share"))

	dir, err := GetXDGDataDir("keyring", 0o700)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tempHome, ".local", "share", "atmos", "keyring"), dir)

	// Verify directory was created with correct permissions.
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestGetXDGConfigDir(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tempHome, ".config"))

	dir, err := GetXDGConfigDir("settings", 0o755)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tempHome, ".config", "atmos", "settings"), dir)

	// Verify directory was created.
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestGetXDGCacheDir_AtmosOverride(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tempHome, ".cache"))
	t.Setenv("ATMOS_XDG_CACHE_HOME", filepath.Join(tempHome, "custom-cache"))

	dir, err := GetXDGCacheDir("test", 0o755)
	require.NoError(t, err)

	// Should use ATMOS_XDG_CACHE_HOME (takes precedence).
	assert.Equal(t, filepath.Join(tempHome, "custom-cache", "atmos", "test"), dir)
}

func TestGetXDGDataDir_AtmosOverride(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(tempHome, ".local", "share"))
	t.Setenv("ATMOS_XDG_DATA_HOME", filepath.Join(tempHome, "custom-data"))

	dir, err := GetXDGDataDir("keyring", 0o700)
	require.NoError(t, err)

	// Should use ATMOS_XDG_DATA_HOME (takes precedence).
	assert.Equal(t, filepath.Join(tempHome, "custom-data", "atmos", "keyring"), dir)
}

func TestGetXDGDir_EmptySubpath(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tempHome, ".cache"))

	dir, err := GetXDGCacheDir("", 0o755)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tempHome, ".cache", "atmos"), dir)
}

func TestGetXDGDir_NestedSubpath(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(tempHome, ".local", "share"))

	dir, err := GetXDGDataDir(filepath.Join("auth", "keyring"), 0o700)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tempHome, ".local", "share", "atmos", "auth", "keyring"), dir)

	// Verify nested directory was created.
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestGetXDGDir_MkdirError(t *testing.T) {
	// Create a file where we want to create a directory.
	// This will cause os.MkdirAll to fail.
	tempHome := t.TempDir()
	blockingFile := filepath.Join(tempHome, "atmos")

	// Create a regular file that blocks directory creation.
	err := os.WriteFile(blockingFile, []byte("blocking"), 0o644)
	require.NoError(t, err)

	t.Setenv("XDG_CACHE_HOME", tempHome)

	// Should fail because "atmos" exists as a file, not a directory.
	_, err = GetXDGCacheDir("test", 0o755)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create directory")
}

func TestGetXDGDir_DefaultFallback(t *testing.T) {
	// Unset all XDG environment variables to test default fallback.
	// The test should use the library default from github.com/adrg/xdg.
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("ATMOS_XDG_CACHE_HOME", "")

	// Should not error even without env vars - uses library default.
	dir, err := GetXDGCacheDir("test", 0o755)
	require.NoError(t, err)

	// Should contain "atmos/test" in the path.
	assert.Contains(t, dir, filepath.Join("atmos", "test"))

	// Verify directory was actually created.
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}
