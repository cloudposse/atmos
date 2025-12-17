package clean

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeletePath_HandlesNonExistentFiles(t *testing.T) {
	// Test that DeletePath handles non-existent files gracefully.
	tempDir := t.TempDir()
	nonExistentPath := filepath.Join(tempDir, "non-existent-file.txt")

	// Should return an error for non-existent file.
	err := DeletePath(nonExistentPath, "test/non-existent-file.txt")
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestDeletePath_DeletesExistingFile(t *testing.T) {
	// Test that DeletePath successfully deletes an existing file.
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test-file.txt")

	// Create the test file.
	require.NoError(t, os.WriteFile(testFile, []byte("test content"), 0o644))

	// Verify file exists.
	_, err := os.Stat(testFile)
	require.NoError(t, err)

	// Delete the file.
	err = DeletePath(testFile, "test/test-file.txt")
	assert.NoError(t, err)

	// Verify file no longer exists.
	_, err = os.Stat(testFile)
	assert.True(t, os.IsNotExist(err))
}

func TestDeletePath_DeletesDirectory(t *testing.T) {
	// Test that DeletePath successfully deletes a directory.
	tempDir := t.TempDir()
	testDir := filepath.Join(tempDir, "test-dir")

	// Create the test directory with a file inside.
	require.NoError(t, os.MkdirAll(testDir, 0o755))
	testFile := filepath.Join(testDir, "file.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("content"), 0o644))

	// Verify directory exists.
	info, err := os.Stat(testDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// Delete the directory.
	err = DeletePath(testDir, "test/test-dir")
	assert.NoError(t, err)

	// Verify directory no longer exists.
	_, err = os.Stat(testDir)
	assert.True(t, os.IsNotExist(err))
}

func TestDeletePath_RefusesSymlinks(t *testing.T) {
	// Test that DeletePath refuses to delete symbolic links.
	tempDir := t.TempDir()
	targetFile := filepath.Join(tempDir, "target.txt")
	symlinkPath := filepath.Join(tempDir, "symlink.txt")

	// Create target file.
	require.NoError(t, os.WriteFile(targetFile, []byte("target content"), 0o644))

	// Create symbolic link.
	err := os.Symlink(targetFile, symlinkPath)
	if err != nil {
		t.Skipf("Skipping symlink test: %v", err)
	}

	// Try to delete the symlink - should be refused.
	err = DeletePath(symlinkPath, "test/symlink.txt")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to delete symbolic link")

	// Verify both symlink and target still exist.
	_, err = os.Stat(symlinkPath)
	assert.NoError(t, err)
	_, err = os.Stat(targetFile)
	assert.NoError(t, err)
}
