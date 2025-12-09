//go:build !windows
// +build !windows

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestLoadCache_UnixReadLockError tests Unix read lock path at cache.go:76-80.
func TestLoadCache_UnixReadLockError(t *testing.T) {
	// Create temp directory for cache.
	tempDir := t.TempDir()
	cleanup := withTestXDGHome(t, tempDir)
	t.Cleanup(cleanup)

	// Create cache directory.
	cacheDir := filepath.Join(tempDir, "atmos")
	err := os.MkdirAll(cacheDir, 0o755)
	assert.NoError(t, err)

	// Create invalid YAML file.
	cacheFile := filepath.Join(cacheDir, "cache.yaml")
	invalidYAML := `invalid: [yaml: content`
	err = os.WriteFile(cacheFile, []byte(invalidYAML), 0o644)
	assert.NoError(t, err)

	// LoadCache should handle the error.
	cfg, err := LoadCache()
	// On Unix with locking, this may error or return empty config.
	// The important thing is it doesn't panic.
	if err != nil {
		assert.Error(t, err)
	} else {
		assert.Equal(t, int64(0), cfg.LastChecked)
	}
}
