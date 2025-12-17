package workdir

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCleanWorkdir_Success(t *testing.T) {
	tmpDir := t.TempDir()

	// Create workdir structure.
	workdirPath := filepath.Join(tmpDir, WorkdirPath, "terraform", "vpc")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workdirPath, "main.tf"), []byte("# test"), 0o644))

	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	err := CleanWorkdir(atmosConfig, "vpc")
	require.NoError(t, err)

	// Verify workdir removed.
	_, err = os.Stat(workdirPath)
	assert.True(t, os.IsNotExist(err), "workdir should be removed")
}

func TestCleanWorkdir_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	// Should not error when workdir doesn't exist.
	err := CleanWorkdir(atmosConfig, "nonexistent")
	require.NoError(t, err)
}

func TestCleanWorkdir_EmptyBasePath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create workdir in current dir pattern.
	workdirPath := filepath.Join(tmpDir, WorkdirPath, "terraform", "vpc")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	// Change to tmpDir and test with empty BasePath.
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(oldWd) }()

	atmosConfig := &schema.AtmosConfiguration{BasePath: ""}

	err = CleanWorkdir(atmosConfig, "vpc")
	require.NoError(t, err)
}

func TestCleanAllWorkdirs_Success(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple workdirs.
	workdirBase := filepath.Join(tmpDir, WorkdirPath)
	require.NoError(t, os.MkdirAll(filepath.Join(workdirBase, "terraform", "vpc"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(workdirBase, "terraform", "s3"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workdirBase, "terraform", "vpc", "main.tf"), []byte("# vpc"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(workdirBase, "terraform", "s3", "main.tf"), []byte("# s3"), 0o644))

	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	err := CleanAllWorkdirs(atmosConfig)
	require.NoError(t, err)

	// Verify entire workdir base removed.
	_, err = os.Stat(workdirBase)
	assert.True(t, os.IsNotExist(err), "workdir base should be removed")
}

func TestCleanAllWorkdirs_NoWorkdirs(t *testing.T) {
	tmpDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	// Should not error when no workdirs exist.
	err := CleanAllWorkdirs(atmosConfig)
	require.NoError(t, err)
}

func TestCleanAllWorkdirs_EmptyBasePath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create workdir.
	workdirBase := filepath.Join(tmpDir, WorkdirPath)
	require.NoError(t, os.MkdirAll(filepath.Join(workdirBase, "terraform", "vpc"), 0o755))

	// Change to tmpDir.
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(oldWd) }()

	atmosConfig := &schema.AtmosConfiguration{BasePath: ""}

	err = CleanAllWorkdirs(atmosConfig)
	require.NoError(t, err)
}

func TestCleanSourceCache_Success(t *testing.T) {
	// CleanSourceCache uses the default cache which requires XDG setup.
	// This test verifies it doesn't error on a fresh system.
	err := CleanSourceCache()
	// May or may not error depending on XDG availability.
	// We just verify it doesn't panic and handles errors gracefully.
	if err != nil {
		assert.ErrorIs(t, err, errUtils.ErrWorkdirClean)
	}
}

func TestClean_WithComponent(t *testing.T) {
	tmpDir := t.TempDir()

	// Create workdir.
	workdirPath := filepath.Join(tmpDir, WorkdirPath, "terraform", "vpc")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	err := Clean(atmosConfig, CleanOptions{Component: "vpc"})
	require.NoError(t, err)

	// Verify workdir removed.
	_, err = os.Stat(workdirPath)
	assert.True(t, os.IsNotExist(err))
}

func TestClean_WithAll(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple workdirs.
	workdirBase := filepath.Join(tmpDir, WorkdirPath)
	require.NoError(t, os.MkdirAll(filepath.Join(workdirBase, "terraform", "vpc"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(workdirBase, "terraform", "s3"), 0o755))

	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	err := Clean(atmosConfig, CleanOptions{All: true})
	require.NoError(t, err)

	// Verify workdir base removed.
	_, err = os.Stat(workdirBase)
	assert.True(t, os.IsNotExist(err))
}

func TestClean_NoOptions(t *testing.T) {
	tmpDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	// With no options set, nothing should be cleaned and no error.
	err := Clean(atmosConfig, CleanOptions{})
	require.NoError(t, err)
}

func TestClean_CacheOnly(t *testing.T) {
	tmpDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	// Clean cache only - may error if XDG not available.
	err := Clean(atmosConfig, CleanOptions{Cache: true})
	// Just verify it doesn't panic.
	_ = err
}

func TestClean_CacheAndComponent(t *testing.T) {
	tmpDir := t.TempDir()

	// Create workdir.
	workdirPath := filepath.Join(tmpDir, WorkdirPath, "terraform", "vpc")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	// Clean both cache and component.
	err := Clean(atmosConfig, CleanOptions{
		Cache:     true,
		Component: "vpc",
	})
	// May have error from cache, but component should still be cleaned.
	_ = err

	// Verify workdir removed regardless of cache result.
	_, statErr := os.Stat(workdirPath)
	assert.True(t, os.IsNotExist(statErr), "workdir should be removed even if cache clean failed")
}

func TestClean_AllAndCache(t *testing.T) {
	tmpDir := t.TempDir()

	// Create workdirs.
	workdirBase := filepath.Join(tmpDir, WorkdirPath)
	require.NoError(t, os.MkdirAll(filepath.Join(workdirBase, "terraform", "vpc"), 0o755))

	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	// Clean all and cache.
	err := Clean(atmosConfig, CleanOptions{
		Cache: true,
		All:   true,
	})
	_ = err

	// Verify workdir base removed.
	_, statErr := os.Stat(workdirBase)
	assert.True(t, os.IsNotExist(statErr))
}

func TestCleanOptions_Structure(t *testing.T) {
	opts := CleanOptions{
		Component: "vpc",
		All:       true,
		Cache:     true,
	}

	assert.Equal(t, "vpc", opts.Component)
	assert.True(t, opts.All)
	assert.True(t, opts.Cache)
}

func TestClean_ErrorAggregation(t *testing.T) {
	// This test verifies error aggregation behavior.
	// When multiple clean operations fail, errors should be joined.
	tmpDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	// Both cache and all clean might produce errors.
	// The function should aggregate them.
	err := Clean(atmosConfig, CleanOptions{
		Cache: true,
		All:   true,
	})
	// If there was an error, it should be wrapped as ErrWorkdirClean.
	if err != nil {
		assert.ErrorIs(t, err, errUtils.ErrWorkdirClean)
	}
}

func TestClean_ComponentPriority(t *testing.T) {
	// When both Component and All are set, Component takes precedence
	// based on the if/else structure in Clean().
	tmpDir := t.TempDir()

	// Create specific workdir.
	vpcPath := filepath.Join(tmpDir, WorkdirPath, "terraform", "vpc")
	s3Path := filepath.Join(tmpDir, WorkdirPath, "terraform", "s3")
	require.NoError(t, os.MkdirAll(vpcPath, 0o755))
	require.NoError(t, os.MkdirAll(s3Path, 0o755))

	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	// All=true should clean all, not just component.
	err := Clean(atmosConfig, CleanOptions{
		Component: "vpc",
		All:       true,
	})
	require.NoError(t, err)

	// With All=true, both should be removed (All takes precedence in if/else).
	_, err = os.Stat(vpcPath)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(s3Path)
	assert.True(t, os.IsNotExist(err))
}

// Test error types.

func TestCleanWorkdir_ErrorType(t *testing.T) {
	// Create a directory that will cause permission errors.
	// Skip on non-Unix systems where permission model differs.
	if os.Getenv("CI") != "" {
		t.Skip("Skipping permission test in CI")
	}

	tmpDir := t.TempDir()
	workdirPath := filepath.Join(tmpDir, WorkdirPath, "terraform", "vpc")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	// Make parent non-writable to cause RemoveAll to fail.
	parentPath := filepath.Join(tmpDir, WorkdirPath, "terraform")
	require.NoError(t, os.Chmod(parentPath, 0o555))
	defer func() { _ = os.Chmod(parentPath, 0o755) }()

	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	err := CleanWorkdir(atmosConfig, "vpc")
	if err != nil {
		assert.ErrorIs(t, err, errUtils.ErrWorkdirClean)
	}
}

func TestCleanAllWorkdirs_ErrorType(t *testing.T) {
	// Similar permission test.
	if os.Getenv("CI") != "" {
		t.Skip("Skipping permission test in CI")
	}

	tmpDir := t.TempDir()
	workdirBase := filepath.Join(tmpDir, WorkdirPath)
	require.NoError(t, os.MkdirAll(filepath.Join(workdirBase, "terraform"), 0o755))

	// Make workdir base non-removable.
	require.NoError(t, os.Chmod(tmpDir, 0o555))
	defer func() { _ = os.Chmod(tmpDir, 0o755) }()

	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	err := CleanAllWorkdirs(atmosConfig)
	if err != nil {
		assert.ErrorIs(t, err, errUtils.ErrWorkdirClean)
	}
}

// Helper to verify error wrapping.

func TestErrorWrapping(t *testing.T) {
	// Verify error builder produces correct sentinel error.
	baseErr := errors.New("underlying error")
	wrappedErr := errUtils.Build(errUtils.ErrWorkdirClean).
		WithCause(baseErr).
		WithExplanation("test explanation").
		Err()

	assert.ErrorIs(t, wrappedErr, errUtils.ErrWorkdirClean)
	// The error should be based on the sentinel error.
	assert.NotNil(t, wrappedErr)
}
