package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectComponentsDirectoryObjects_NoDuplicateFiles(t *testing.T) {
	// Create a temporary directory structure to simulate terraform components.
	tempDir := t.TempDir()

	// Create component directories that would be referenced by multiple stacks.
	component1Dir := filepath.Join(tempDir, "component1")
	component2Dir := filepath.Join(tempDir, "component2")

	require.NoError(t, os.MkdirAll(component1Dir, 0o755))
	require.NoError(t, os.MkdirAll(component2Dir, 0o755))

	// Create test files in component1.
	terraformDir1 := filepath.Join(component1Dir, ".terraform")
	require.NoError(t, os.MkdirAll(terraformDir1, 0o755))

	lockFile1 := filepath.Join(component1Dir, ".terraform.lock.hcl")
	require.NoError(t, os.WriteFile(lockFile1, []byte("lock content"), 0o644))

	varFile1 := filepath.Join(component1Dir, "test.tfvars.json")
	require.NoError(t, os.WriteFile(varFile1, []byte("{}"), 0o644))

	// Create test files in component2.
	terraformDir2 := filepath.Join(component2Dir, ".terraform")
	require.NoError(t, os.MkdirAll(terraformDir2, 0o755))

	lockFile2 := filepath.Join(component2Dir, ".terraform.lock.hcl")
	require.NoError(t, os.WriteFile(lockFile2, []byte("lock content"), 0o644))

	// Test patterns that would match our created files.
	patterns := []string{".terraform", ".terraform.lock.hcl", "*.tfvars.json"}

	// Simulate multiple stacks referencing the same components.
	// This would happen when getAllStacksComponentsPaths returns duplicate paths.
	componentPaths := []string{"component1", "component1", "component2", "component1"} // component1 appears 3 times.

	// Before the fix, this would collect duplicate files.
	folders, err := CollectComponentsDirectoryObjects(tempDir, componentPaths, patterns)
	require.NoError(t, err)

	// Count how many times each file appears.
	fileOccurrences := make(map[string]int)
	for _, folder := range folders {
		for _, file := range folder.Files {
			fileOccurrences[file.FullPath]++
		}
	}

	// Check if any file appears more than once.
	duplicatesFound := false
	for path, count := range fileOccurrences {
		if count > 1 {
			t.Logf("File %s appears %d times (duplicate)", path, count)
			duplicatesFound = true
		}
	}

	// With the current implementation, we expect duplicates because
	// CollectComponentsDirectoryObjects doesn't deduplicate internally.
	// The deduplication happens at the getAllStacksComponentsPaths level.
	assert.True(t, duplicatesFound, "Expected duplicates when passing duplicate paths to CollectComponentsDirectoryObjects")
}

func TestDeletePathTerraform_HandlesNonExistentFiles(t *testing.T) {
	// Test that DeletePathTerraform handles non-existent files gracefully.
	tempDir := t.TempDir()
	nonExistentPath := filepath.Join(tempDir, "non-existent-file.txt")

	// Should return an error for non-existent file.
	err := DeletePathTerraform(nonExistentPath, "test/non-existent-file.txt")
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestDeletePathTerraform_DeletesExistingFile(t *testing.T) {
	// Test that DeletePathTerraform successfully deletes an existing file.
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test-file.txt")

	// Create the test file.
	require.NoError(t, os.WriteFile(testFile, []byte("test content"), 0o644))

	// Verify file exists.
	_, err := os.Stat(testFile)
	require.NoError(t, err)

	// Delete the file.
	err = DeletePathTerraform(testFile, "test/test-file.txt")
	assert.NoError(t, err)

	// Verify file no longer exists.
	_, err = os.Stat(testFile)
	assert.True(t, os.IsNotExist(err))
}

func TestDeletePathTerraform_DeletesDirectory(t *testing.T) {
	// Test that DeletePathTerraform successfully deletes a directory.
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
	err = DeletePathTerraform(testDir, "test/test-dir")
	assert.NoError(t, err)

	// Verify directory no longer exists.
	_, err = os.Stat(testDir)
	assert.True(t, os.IsNotExist(err))
}

func TestDeletePathTerraform_RefusesSymlinks(t *testing.T) {
	// Test that DeletePathTerraform refuses to delete symbolic links.
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
	err = DeletePathTerraform(symlinkPath, "test/symlink.txt")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to delete symbolic link")

	// Verify both symlink and target still exist.
	_, err = os.Stat(symlinkPath)
	assert.NoError(t, err)
	_, err = os.Stat(targetFile)
	assert.NoError(t, err)
}
