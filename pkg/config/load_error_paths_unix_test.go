//go:build !windows

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// TestLoadAtmosDFromDirectory_StatPermissionError tests the stat error differentiation path.
// This test creates a directory that cannot be stat'd due to permission errors (Unix-only).
// On Windows, permission handling is different and this test is skipped.
func TestLoadAtmosDFromDirectory_StatPermissionError(t *testing.T) {
	// Skip if running as root (root can access everything).
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	tempDir := t.TempDir()

	// Create a parent directory.
	parentDir := filepath.Join(tempDir, "parent")
	err := os.MkdirAll(parentDir, 0o755)
	assert.NoError(t, err)

	// Create atmos.d inside parent.
	atmosDPath := filepath.Join(parentDir, "atmos.d")
	err = os.MkdirAll(atmosDPath, 0o755)
	assert.NoError(t, err)

	// Remove execute permission from parent directory.
	// This prevents stat() on children, causing a permission error (not NotExist).
	err = os.Chmod(parentDir, 0o000)
	assert.NoError(t, err)

	// Ensure we restore permissions on cleanup.
	t.Cleanup(func() {
		_ = os.Chmod(parentDir, 0o755)
	})

	v := viper.New()
	v.SetConfigType("yaml")

	// Call loadAtmosDFromDirectory - should hit the stat error path.
	// The error is logged at Debug level (not returned), so function should complete.
	loadAtmosDFromDirectory(parentDir, v)

	// Function should complete without panic even with permission errors.
}
