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
// Because validatedIsFiles only stats each path, a file with no read permission
// passes validation but fails the subsequent read (Unix-only; skipped as root).
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

// newLockedGitRoot creates a git-root temp dir containing an unreadable "locked"
// subdirectory (chmod 000) and points TEST_GIT_ROOT at it. A bare base_path of
// "locked/child" then makes resolveAbsolutePath's git-root os.Stat fail with a
// permission error (not NotExist), exercising its stat-error branch. Unix-only.
func newLockedGitRoot(t *testing.T) string {
	t.Helper()
	gitRoot := t.TempDir()
	t.Setenv("TEST_GIT_ROOT", gitRoot)
	locked := filepath.Join(gitRoot, "locked")
	require.NoError(t, os.MkdirAll(locked, 0o755))
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	return gitRoot
}

// TestMergeImports_ResolveAbsolutePathError covers the resolveAbsolutePath error branch
// in mergeImports (base_path resolution hits a permission-denied stat under git root).
func TestMergeImports_ResolveAbsolutePathError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}
	setupTestAdapters()
	gitRoot := newLockedGitRoot(t)

	v := viper.New()
	v.SetConfigType("yaml")
	v.Set("base_path", "locked/child")

	_, err := mergeImports(v, gitRoot, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrStatFile)
}

// TestMergeConfigFileWithImports_ResolveAbsolutePathError covers the resolveAbsolutePath
// error branch in mergeConfigFileWithImports (a config file that declares a bare
// base_path resolving under an unreadable git-root directory, plus an import).
func TestMergeConfigFileWithImports_ResolveAbsolutePathError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}
	setupTestAdapters()
	gitRoot := newLockedGitRoot(t)

	cfg := filepath.Join(gitRoot, "atmos.yaml")
	require.NoError(t, os.WriteFile(cfg, []byte("base_path: locked/child\nimport:\n  - x.yaml\n"), 0o644))

	v := viper.New()
	v.SetConfigType("yaml")

	err := mergeConfigFileWithImports(cfg, v)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrStatFile)
}
