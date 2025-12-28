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
	t.Chdir(tmpDir)

	atmosConfig := &schema.AtmosConfiguration{BasePath: ""}

	err := CleanWorkdir(atmosConfig, "vpc")
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
	t.Chdir(tmpDir)

	atmosConfig := &schema.AtmosConfiguration{BasePath: ""}

	err := CleanAllWorkdirs(atmosConfig)
	require.NoError(t, err)
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

func TestClean_AllTakesPrecedence(t *testing.T) {
	// When both Component and All are set, All takes precedence
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
	require.Error(t, err, "expected permission error to occur")
	assert.ErrorIs(t, err, errUtils.ErrWorkdirClean)
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
	require.Error(t, err, "expected permission error to occur")
	assert.ErrorIs(t, err, errUtils.ErrWorkdirClean)
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
}

// Test Clean error accumulation - the missing coverage in Clean function.

func TestClean_ErrorAccumulation_AllFails(t *testing.T) {
	// Test the error accumulation path by making CleanAllWorkdirs fail.
	// This requires creating a situation where removal fails.
	if os.Getenv("CI") != "" {
		t.Skip("Skipping permission test in CI")
	}

	tmpDir := t.TempDir()
	workdirBase := filepath.Join(tmpDir, WorkdirPath)
	require.NoError(t, os.MkdirAll(filepath.Join(workdirBase, "terraform"), 0o755))

	// Make parent non-writable to cause removal to fail.
	require.NoError(t, os.Chmod(tmpDir, 0o555))
	defer func() { _ = os.Chmod(tmpDir, 0o755) }()

	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	err := Clean(atmosConfig, CleanOptions{All: true})
	require.Error(t, err, "expected permission error to occur during cleanup")
	// When errors occur, they should be accumulated.
	assert.ErrorIs(t, err, errUtils.ErrWorkdirClean)
}

func TestClean_ErrorAccumulation_ComponentFails(t *testing.T) {
	// Test error accumulation when component cleanup fails.
	if os.Getenv("CI") != "" {
		t.Skip("Skipping permission test in CI")
	}

	tmpDir := t.TempDir()
	workdirPath := filepath.Join(tmpDir, WorkdirPath, "terraform", "vpc")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	// Make parent non-writable to cause removal to fail.
	parentPath := filepath.Join(tmpDir, WorkdirPath, "terraform")
	require.NoError(t, os.Chmod(parentPath, 0o555))
	defer func() { _ = os.Chmod(parentPath, 0o755) }()

	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	err := Clean(atmosConfig, CleanOptions{Component: "vpc"})
	require.Error(t, err, "expected permission error to occur during cleanup")
	// When errors occur, they should be accumulated.
	assert.ErrorIs(t, err, errUtils.ErrWorkdirClean)
}
