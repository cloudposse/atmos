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
