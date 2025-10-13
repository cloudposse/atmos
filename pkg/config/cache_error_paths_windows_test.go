//go:build windows
// +build windows

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestLoadCache_WindowsReadError tests Windows read path at cache.go:64-74.
func TestLoadCache_WindowsReadError(t *testing.T) {
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

	// On Windows, errors are ignored and empty config is returned.
	cfg, err := LoadCache()
	assert.NoError(t, err)
	// Should return empty config despite invalid YAML.
	assert.Equal(t, int64(0), cfg.LastChecked)
}
