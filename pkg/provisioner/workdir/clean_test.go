package workdir

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCleanWorkdir_Success(t *testing.T) {
	tmpDir := t.TempDir()

	// Create workdir structure using stack-component naming.
	workdirPath := filepath.Join(tmpDir, WorkdirPath, "terraform", "dev-vpc")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workdirPath, "main.tf"), []byte("# test"), 0o644))

	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	err := CleanWorkdir(atmosConfig, "vpc", "dev")
	require.NoError(t, err)

	// Verify workdir removed.
	_, err = os.Stat(workdirPath)
	assert.True(t, os.IsNotExist(err), "workdir should be removed")
}

func TestCleanWorkdir_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	// Should not error when workdir doesn't exist.
	err := CleanWorkdir(atmosConfig, "nonexistent", "dev")
	require.NoError(t, err)
}

func TestCleanWorkdir_EmptyBasePath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create workdir in current dir pattern using stack-component naming.
	workdirPath := filepath.Join(tmpDir, WorkdirPath, "terraform", "dev-vpc")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	// Change to tmpDir and test with empty BasePath.
	t.Chdir(tmpDir)

	atmosConfig := &schema.AtmosConfiguration{BasePath: ""}

	err := CleanWorkdir(atmosConfig, "vpc", "dev")
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

	// Create workdir using stack-component naming.
	workdirPath := filepath.Join(tmpDir, WorkdirPath, "terraform", "dev-vpc")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	err := Clean(atmosConfig, CleanOptions{Component: "vpc", Stack: "dev"})
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

	// Create specific workdirs using stack-component naming.
	vpcPath := filepath.Join(tmpDir, WorkdirPath, "terraform", "dev-vpc")
	s3Path := filepath.Join(tmpDir, WorkdirPath, "terraform", "dev-s3")
	require.NoError(t, os.MkdirAll(vpcPath, 0o755))
	require.NoError(t, os.MkdirAll(s3Path, 0o755))

	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	// All=true should clean all, not just component.
	err := Clean(atmosConfig, CleanOptions{
		Component: "vpc",
		Stack:     "dev",
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
	if runtime.GOOS == "windows" {
		t.Skip("Skipping permission test on Windows - different permission model")
	}
	if os.Getenv("CI") != "" {
		t.Skip("Skipping permission test in CI")
	}

	tmpDir := t.TempDir()
	workdirPath := filepath.Join(tmpDir, WorkdirPath, "terraform", "dev-vpc")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	// Make parent non-writable to cause RemoveAll to fail.
	parentPath := filepath.Join(tmpDir, WorkdirPath, "terraform")
	require.NoError(t, os.Chmod(parentPath, 0o555))
	defer func() { _ = os.Chmod(parentPath, 0o755) }()

	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	err := CleanWorkdir(atmosConfig, "vpc", "dev")
	require.Error(t, err, "expected permission error to occur")
	assert.ErrorIs(t, err, errUtils.ErrWorkdirClean)
}

func TestCleanAllWorkdirs_ErrorType(t *testing.T) {
	// Similar permission test.
	if runtime.GOOS == "windows" {
		t.Skip("Skipping permission test on Windows - different permission model")
	}
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
	if runtime.GOOS == "windows" {
		t.Skip("Skipping permission test on Windows - different permission model")
	}
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
	if runtime.GOOS == "windows" {
		t.Skip("Skipping permission test on Windows - different permission model")
	}
	if os.Getenv("CI") != "" {
		t.Skip("Skipping permission test in CI")
	}

	tmpDir := t.TempDir()
	workdirPath := filepath.Join(tmpDir, WorkdirPath, "terraform", "dev-vpc")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	// Make parent non-writable to cause removal to fail.
	parentPath := filepath.Join(tmpDir, WorkdirPath, "terraform")
	require.NoError(t, os.Chmod(parentPath, 0o555))
	defer func() { _ = os.Chmod(parentPath, 0o755) }()

	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	err := Clean(atmosConfig, CleanOptions{Component: "vpc", Stack: "dev"})
	require.Error(t, err, "expected permission error to occur during cleanup")
	// When errors occur, they should be accumulated.
	assert.ErrorIs(t, err, errUtils.ErrWorkdirClean)
}

// Test expired workdir cleanup.

func TestCleanExpiredWorkdirs_NoWorkdirs(t *testing.T) {
	tmpDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	err := CleanExpiredWorkdirs(atmosConfig, "7d", false)
	require.NoError(t, err)
}

func TestCleanExpiredWorkdirs_InvalidTTL(t *testing.T) {
	tmpDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	err := CleanExpiredWorkdirs(atmosConfig, "invalid", false)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWorkdirClean)
}

func TestCleanExpiredWorkdirs_DryRun(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a workdir with old metadata.
	workdirPath := filepath.Join(tmpDir, WorkdirPath, "terraform", "dev-vpc")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	// Write metadata with old LastAccessed time (30 days ago).
	oldTime := time.Now().Add(-30 * 24 * time.Hour)
	metadata := &WorkdirMetadata{
		Component:    "vpc",
		Stack:        "dev",
		SourceType:   SourceTypeLocal,
		Source:       "components/terraform/vpc",
		CreatedAt:    oldTime,
		UpdatedAt:    oldTime,
		LastAccessed: oldTime,
	}
	require.NoError(t, WriteMetadata(workdirPath, metadata))

	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	// Dry run should not remove the workdir.
	err := CleanExpiredWorkdirs(atmosConfig, "7d", true)
	require.NoError(t, err)

	// Verify workdir still exists.
	_, err = os.Stat(workdirPath)
	assert.False(t, os.IsNotExist(err), "workdir should still exist after dry run")
}

func TestCleanExpiredWorkdirs_ActualClean(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a workdir with old metadata.
	workdirPath := filepath.Join(tmpDir, WorkdirPath, "terraform", "dev-vpc")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	// Write metadata with old LastAccessed time (30 days ago).
	oldTime := time.Now().Add(-30 * 24 * time.Hour)
	metadata := &WorkdirMetadata{
		Component:    "vpc",
		Stack:        "dev",
		SourceType:   SourceTypeLocal,
		Source:       "components/terraform/vpc",
		CreatedAt:    oldTime,
		UpdatedAt:    oldTime,
		LastAccessed: oldTime,
	}
	require.NoError(t, WriteMetadata(workdirPath, metadata))

	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	// Actual clean should remove the expired workdir.
	err := CleanExpiredWorkdirs(atmosConfig, "7d", false)
	require.NoError(t, err)

	// Verify workdir is removed.
	_, err = os.Stat(workdirPath)
	assert.True(t, os.IsNotExist(err), "workdir should be removed")
}

func TestCleanExpiredWorkdirs_NotExpired(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a workdir with recent metadata.
	workdirPath := filepath.Join(tmpDir, WorkdirPath, "terraform", "dev-vpc")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	// Write metadata with recent LastAccessed time (1 hour ago).
	recentTime := time.Now().Add(-1 * time.Hour)
	metadata := &WorkdirMetadata{
		Component:    "vpc",
		Stack:        "dev",
		SourceType:   SourceTypeLocal,
		Source:       "components/terraform/vpc",
		CreatedAt:    recentTime,
		UpdatedAt:    recentTime,
		LastAccessed: recentTime,
	}
	require.NoError(t, WriteMetadata(workdirPath, metadata))

	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	// Should not clean recent workdirs.
	err := CleanExpiredWorkdirs(atmosConfig, "7d", false)
	require.NoError(t, err)

	// Verify workdir still exists.
	_, err = os.Stat(workdirPath)
	assert.False(t, os.IsNotExist(err), "recent workdir should not be removed")
}

func TestClean_ExpiredWithoutTTL(t *testing.T) {
	tmpDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	// Expired=true without TTL should error.
	err := Clean(atmosConfig, CleanOptions{Expired: true})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWorkdirClean)
}

func TestClean_ExpiredWithTTL(t *testing.T) {
	tmpDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	// No workdirs, should succeed without error.
	err := Clean(atmosConfig, CleanOptions{Expired: true, TTL: "7d"})
	require.NoError(t, err)
}

// Test formatDuration.

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"less than minute", 30 * time.Second, "< 1m"},
		{"minutes only", 45 * time.Minute, "45m"},
		{"hours only", 3 * time.Hour, "3h"},
		{"hours and minutes", 3*time.Hour + 30*time.Minute, "3h 30m"},
		{"days only", 7 * 24 * time.Hour, "7d"},
		{"days and hours", 7*24*time.Hour + 5*time.Hour, "7d 5h"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := formatDuration(tc.duration)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// Test getLastAccessedTime fallbacks.

func TestGetLastAccessedTime_WithMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	workdirPath := filepath.Join(tmpDir, "test-workdir")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	expectedTime := time.Now().Add(-24 * time.Hour).Truncate(time.Second)
	metadata := &WorkdirMetadata{
		Component:    "test",
		Stack:        "dev",
		SourceType:   SourceTypeLocal,
		Source:       "test",
		CreatedAt:    expectedTime.Add(-48 * time.Hour),
		UpdatedAt:    expectedTime.Add(-12 * time.Hour),
		LastAccessed: expectedTime,
	}
	require.NoError(t, WriteMetadata(workdirPath, metadata))

	entries, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	result := getLastAccessedTime(workdirPath, entries[0])
	// LastAccessed should be used.
	assert.True(t, expectedTime.Equal(result), "LastAccessed time should match")
}

func TestGetLastAccessedTime_FallbackToUpdatedAt(t *testing.T) {
	tmpDir := t.TempDir()
	workdirPath := filepath.Join(tmpDir, "test-workdir")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	expectedTime := time.Now().Add(-24 * time.Hour).Truncate(time.Second)
	metadata := &WorkdirMetadata{
		Component:  "test",
		Stack:      "dev",
		SourceType: SourceTypeLocal,
		Source:     "test",
		CreatedAt:  expectedTime.Add(-48 * time.Hour),
		UpdatedAt:  expectedTime,
		// LastAccessed is zero.
	}
	require.NoError(t, WriteMetadata(workdirPath, metadata))

	entries, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	result := getLastAccessedTime(workdirPath, entries[0])
	// UpdatedAt should be used when LastAccessed is zero.
	assert.True(t, expectedTime.Equal(result), "UpdatedAt time should match when LastAccessed is zero")
}

func TestGetLastAccessedTime_FallbackToCreatedAt(t *testing.T) {
	tmpDir := t.TempDir()
	workdirPath := filepath.Join(tmpDir, "test-workdir")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	expectedTime := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	metadata := &WorkdirMetadata{
		Component:  "test",
		Stack:      "dev",
		SourceType: SourceTypeLocal,
		Source:     "test",
		CreatedAt:  expectedTime,
		// UpdatedAt and LastAccessed are zero.
	}
	require.NoError(t, WriteMetadata(workdirPath, metadata))

	entries, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	result := getLastAccessedTime(workdirPath, entries[0])
	// CreatedAt should be used when others are zero.
	assert.True(t, expectedTime.Equal(result), "CreatedAt time should match when others are zero")
}

func TestGetLastAccessedTime_NoMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	workdirPath := filepath.Join(tmpDir, "test-workdir")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	// No metadata written.

	entries, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	result := getLastAccessedTime(workdirPath, entries[0])
	// Should fall back to directory modification time.
	assert.False(t, result.IsZero(), "should return file modification time")
}

// Test checkWorkdirExpiry.

func TestCheckWorkdirExpiry_NotADirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file, not a directory.
	filePath := filepath.Join(tmpDir, "testfile")
	require.NoError(t, os.WriteFile(filePath, []byte("test"), 0o644))

	entries, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	result := checkWorkdirExpiry(tmpDir, entries[0], time.Now())
	assert.Nil(t, result, "should return nil for non-directory entries")
}

func TestCheckWorkdirExpiry_NotExpired(t *testing.T) {
	tmpDir := t.TempDir()
	workdirPath := filepath.Join(tmpDir, "recent-workdir")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	// Write recent metadata.
	recentTime := time.Now().Add(-1 * time.Hour)
	metadata := &WorkdirMetadata{
		Component:    "vpc",
		Stack:        "dev",
		LastAccessed: recentTime,
	}
	require.NoError(t, WriteMetadata(workdirPath, metadata))

	entries, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	// Cutoff is 7 days ago, workdir is only 1 hour old.
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	result := checkWorkdirExpiry(tmpDir, entries[0], cutoff)
	assert.Nil(t, result, "should return nil for non-expired workdirs")
}

func TestCheckWorkdirExpiry_Expired(t *testing.T) {
	tmpDir := t.TempDir()
	workdirPath := filepath.Join(tmpDir, "old-workdir")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	// Write old metadata.
	oldTime := time.Now().Add(-30 * 24 * time.Hour)
	metadata := &WorkdirMetadata{
		Component:    "vpc",
		Stack:        "dev",
		LastAccessed: oldTime,
	}
	require.NoError(t, WriteMetadata(workdirPath, metadata))

	entries, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	// Cutoff is 7 days ago, workdir is 30 days old.
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	result := checkWorkdirExpiry(tmpDir, entries[0], cutoff)
	require.NotNil(t, result, "should return info for expired workdirs")
	assert.Equal(t, "old-workdir", result.Name)
	assert.Equal(t, workdirPath, result.Path)
}

// Test getModTimeFromEntry.

func TestGetModTimeFromEntry(t *testing.T) {
	tmpDir := t.TempDir()
	workdirPath := filepath.Join(tmpDir, "test-workdir")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	entries, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	result := getModTimeFromEntry(entries[0])
	assert.False(t, result.IsZero(), "should return non-zero mod time")
	// ModTime should be recent (within last minute).
	assert.True(t, result.After(time.Now().Add(-1*time.Minute)))
}

// Test findExpiredWorkdirs.

func TestFindExpiredWorkdirs_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()

	// No workdir base exists.
	result, err := findExpiredWorkdirs(tmpDir, 7*24*time.Hour)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestFindExpiredWorkdirs_MixedWorkdirs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create terraform workdirs base.
	workdirBase := filepath.Join(tmpDir, WorkdirPath, "terraform")
	require.NoError(t, os.MkdirAll(workdirBase, 0o755))

	// Create an old workdir.
	oldWorkdir := filepath.Join(workdirBase, "old-vpc")
	require.NoError(t, os.MkdirAll(oldWorkdir, 0o755))
	oldTime := time.Now().Add(-30 * 24 * time.Hour)
	require.NoError(t, WriteMetadata(oldWorkdir, &WorkdirMetadata{
		Component:    "vpc",
		Stack:        "old",
		LastAccessed: oldTime,
	}))

	// Create a recent workdir.
	recentWorkdir := filepath.Join(workdirBase, "recent-s3")
	require.NoError(t, os.MkdirAll(recentWorkdir, 0o755))
	recentTime := time.Now().Add(-1 * time.Hour)
	require.NoError(t, WriteMetadata(recentWorkdir, &WorkdirMetadata{
		Component:    "s3",
		Stack:        "recent",
		LastAccessed: recentTime,
	}))

	// Find expired with 7 day TTL.
	result, err := findExpiredWorkdirs(tmpDir, 7*24*time.Hour)
	require.NoError(t, err)

	// Only the old workdir should be returned.
	require.Len(t, result, 1)
	assert.Equal(t, "old-vpc", result[0].Name)
}

// Test CleanExpiredWorkdirs with removal errors.

func TestCleanExpiredWorkdirs_EmptyBasePath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create workdir.
	workdirBase := filepath.Join(tmpDir, WorkdirPath, "terraform", "old-vpc")
	require.NoError(t, os.MkdirAll(workdirBase, 0o755))

	oldTime := time.Now().Add(-30 * 24 * time.Hour)
	require.NoError(t, WriteMetadata(workdirBase, &WorkdirMetadata{
		Component:    "vpc",
		Stack:        "old",
		LastAccessed: oldTime,
	}))

	// Change to tmpDir.
	t.Chdir(tmpDir)

	atmosConfig := &schema.AtmosConfiguration{BasePath: ""}

	err := CleanExpiredWorkdirs(atmosConfig, "7d", false)
	require.NoError(t, err)

	// Workdir should be removed.
	_, err = os.Stat(workdirBase)
	assert.True(t, os.IsNotExist(err))
}

// Test Clean with Expired option.

func TestClean_ExpiredTakesPrecedence(t *testing.T) {
	tmpDir := t.TempDir()

	// Create an old workdir.
	workdirPath := filepath.Join(tmpDir, WorkdirPath, "terraform", "old-vpc")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	oldTime := time.Now().Add(-30 * 24 * time.Hour)
	require.NoError(t, WriteMetadata(workdirPath, &WorkdirMetadata{
		Component:    "vpc",
		Stack:        "old",
		LastAccessed: oldTime,
	}))

	atmosConfig := &schema.AtmosConfiguration{BasePath: tmpDir}

	// When Expired is true, it takes precedence over Component/Stack.
	err := Clean(atmosConfig, CleanOptions{
		Component: "different",
		Stack:     "stack",
		Expired:   true,
		TTL:       "7d",
	})
	require.NoError(t, err)

	// The old workdir should be removed (expired takes precedence).
	_, err = os.Stat(workdirPath)
	assert.True(t, os.IsNotExist(err))
}

// Test formatDuration edge cases.

func TestFormatDuration_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"zero duration", 0, "< 1m"},
		{"1 second", 1 * time.Second, "< 1m"},
		{"59 seconds", 59 * time.Second, "< 1m"},
		{"1 minute exactly", 1 * time.Minute, "1m"},
		{"1 hour exactly", 1 * time.Hour, "1h"},
		{"1 day exactly", 24 * time.Hour, "1d"},
		{"1 day 1 hour", 25 * time.Hour, "1d 1h"},
		{"negative duration", -1 * time.Hour, "< 1m"}, // Edge case.
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := formatDuration(tc.duration)
			assert.Equal(t, tc.expected, result)
		})
	}
}
