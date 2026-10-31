package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adrg/xdg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithTestXDGHomeOverridesAtmosCache(t *testing.T) {
	// Register first so Reload runs after t.Setenv restores the surrounding environment.
	t.Cleanup(xdg.Reload)
	ambient := t.TempDir()
	standard := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", ambient)
	t.Setenv("XDG_CACHE_HOME", standard)
	ambientFile := filepath.Join(ambient, "atmos", "cache.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(ambientFile), 0o755))
	content := []byte("installation_id: developer-cache\nlast_checked: 123\n")
	require.NoError(t, os.WriteFile(ambientFile, content, 0o600))
	isolated := t.TempDir()
	func() {
		defer withTestXDGHome(t, isolated)()
		path, err := GetCacheFilePath()
		require.NoError(t, err)
		// Check isolation before writing, including when testing the broken helper.
		require.Equal(t, filepath.Join(isolated, "atmos", "cache.yaml"), path)
		require.NoError(t, SaveCache(CacheConfig{LastChecked: 456}))
		require.FileExists(t, path)
	}()
	assert.Equal(t, ambient, os.Getenv("ATMOS_XDG_CACHE_HOME"))
	assert.Equal(t, standard, os.Getenv("XDG_CACHE_HOME"))
	path, err := GetCacheFilePath()
	require.NoError(t, err)
	assert.Equal(t, ambientFile, path)
	after, err := os.ReadFile(ambientFile)
	require.NoError(t, err)
	assert.Equal(t, content, after, "cache tests must leave the inherited cache untouched")
}
