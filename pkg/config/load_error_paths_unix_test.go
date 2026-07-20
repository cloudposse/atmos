//go:build !windows

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
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

// TestMergeFiles_ReadFileError covers the os.ReadFile failure branch in mergeFiles.
// validatedIsFiles only stats each path, so a file with no read permission passes
// validation but fails the subsequent read (Unix-only; skipped as root).
func TestMergeFiles_ReadFileError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	dir := t.TempDir()
	cfg := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(cfg, []byte("base_path: .\n"), 0o000))
	t.Cleanup(func() {
		_ = os.Chmod(cfg, 0o644)
	})

	v := viper.New()
	v.SetConfigType("yaml")

	err := mergeFiles(v, []string{cfg})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrReadConfig)
}
