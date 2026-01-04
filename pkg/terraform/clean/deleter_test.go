package clean

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
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

func TestDeletePath_NormalizesPathSeparators(t *testing.T) {
	// Test that DeletePath normalizes path separators.
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "subdir", "file.txt")

	// Create subdirectory and file.
	require.NoError(t, os.MkdirAll(filepath.Dir(testFile), 0o755))
	require.NoError(t, os.WriteFile(testFile, []byte("content"), 0o644))

	// Delete using backslashes (Windows-style).
	err := DeletePath(testFile, "subdir\\file.txt")
	assert.NoError(t, err)

	// File should be deleted.
	_, err = os.Stat(testFile)
	assert.True(t, os.IsNotExist(err))
}

func TestDeleteFolders_DeletesMultipleFolders(t *testing.T) {
	tempDir := t.TempDir()

	// Create folder structure.
	folder1 := filepath.Join(tempDir, "folder1")
	folder2 := filepath.Join(tempDir, "folder2")
	require.NoError(t, os.MkdirAll(folder1, 0o755))
	require.NoError(t, os.MkdirAll(folder2, 0o755))

	// Create files in folders.
	file1 := filepath.Join(folder1, "file1.txt")
	file2 := filepath.Join(folder2, "file2.txt")
	require.NoError(t, os.WriteFile(file1, []byte("content1"), 0o644))
	require.NoError(t, os.WriteFile(file2, []byte("content2"), 0o644))

	// Build Directory structs.
	folders := []Directory{
		{
			Name:         "folder1",
			FullPath:     folder1,
			RelativePath: "folder1",
			Files: []ObjectInfo{
				{FullPath: file1, RelativePath: "folder1/file1.txt", Name: "file1.txt", IsDir: false},
			},
		},
		{
			Name:         "folder2",
			FullPath:     folder2,
			RelativePath: "folder2",
			Files: []ObjectInfo{
				{FullPath: file2, RelativePath: "folder2/file2.txt", Name: "file2.txt", IsDir: false},
			},
		},
	}

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
	}

	// Delete folders.
	deleteFolders(folders, "test", atmosConfig)

	// Verify files are deleted.
	assert.NoFileExists(t, file1)
	assert.NoFileExists(t, file2)

	// Verify empty folders are removed.
	assert.NoDirExists(t, folder1)
	assert.NoDirExists(t, folder2)
}

func TestDeleteFolders_DeletesDirectories(t *testing.T) {
	tempDir := t.TempDir()

	// Create a component folder with .terraform directory.
	componentDir := filepath.Join(tempDir, "component")
	terraformDir := filepath.Join(componentDir, ".terraform")
	require.NoError(t, os.MkdirAll(terraformDir, 0o755))

	// Create a file inside .terraform.
	require.NoError(t, os.WriteFile(filepath.Join(terraformDir, "providers"), []byte("content"), 0o644))

	// Build Directory struct where the file IS a directory.
	folders := []Directory{
		{
			Name:         "component",
			FullPath:     componentDir,
			RelativePath: "component",
			Files: []ObjectInfo{
				{FullPath: terraformDir, RelativePath: ".terraform", Name: ".terraform", IsDir: true},
			},
		},
	}

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
	}

	// Delete folders.
	deleteFolders(folders, "test", atmosConfig)

	// Verify .terraform directory is deleted.
	assert.NoDirExists(t, terraformDir)
}

func TestDeleteFolders_HandlesNonEmptyFolders(t *testing.T) {
	tempDir := t.TempDir()

	// Create folder with multiple files but only delete some.
	folder := filepath.Join(tempDir, "folder")
	require.NoError(t, os.MkdirAll(folder, 0o755))

	toDelete := filepath.Join(folder, "delete-me.txt")
	toKeep := filepath.Join(folder, "keep-me.txt")
	require.NoError(t, os.WriteFile(toDelete, []byte("delete"), 0o644))
	require.NoError(t, os.WriteFile(toKeep, []byte("keep"), 0o644))

	// Only delete one file.
	folders := []Directory{
		{
			Name:         "folder",
			FullPath:     folder,
			RelativePath: "folder",
			Files: []ObjectInfo{
				{FullPath: toDelete, RelativePath: "folder/delete-me.txt", Name: "delete-me.txt", IsDir: false},
			},
		},
	}

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
	}

	deleteFolders(folders, "test", atmosConfig)

	// File to delete should be gone.
	assert.NoFileExists(t, toDelete)
	// File to keep should still exist.
	assert.FileExists(t, toKeep)
	// Folder should still exist (not empty).
	assert.DirExists(t, folder)
}

func TestExecuteCleanDeletion_DeletesFoldersAndTFDataDir(t *testing.T) {
	tempDir := t.TempDir()

	// Create component folder.
	componentDir := filepath.Join(tempDir, "component")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	// Create a file to delete.
	lockFile := filepath.Join(componentDir, ".terraform.lock.hcl")
	require.NoError(t, os.WriteFile(lockFile, []byte("lock"), 0o644))

	folders := []Directory{
		{
			Name:         "component",
			FullPath:     componentDir,
			RelativePath: "component",
			Files: []ObjectInfo{
				{FullPath: lockFile, RelativePath: "component/.terraform.lock.hcl", Name: ".terraform.lock.hcl", IsDir: false},
			},
		},
	}

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
	}

	// Execute with no TF_DATA_DIR folders.
	executeCleanDeletion(folders, []Directory{}, "component", atmosConfig)

	// Verify file is deleted.
	assert.NoFileExists(t, lockFile)
}

func TestHandleTFDataDir_NoEnvVar(t *testing.T) {
	tempDir := t.TempDir()

	// Ensure TF_DATA_DIR is not set (t.Setenv with empty string effectively unsets).
	t.Setenv(EnvTFDataDir, "")

	// Should do nothing and not panic.
	handleTFDataDir(tempDir, "test")
}

func TestHandleTFDataDir_InvalidDir(t *testing.T) {
	tempDir := t.TempDir()

	// Set TF_DATA_DIR to an invalid value.
	t.Setenv(EnvTFDataDir, "/")

	// Should fail validation and not delete anything.
	handleTFDataDir(tempDir, "test")
}

func TestHandleTFDataDir_NonExistentDir(t *testing.T) {
	tempDir := t.TempDir()

	// Set TF_DATA_DIR to a non-existent directory.
	t.Setenv(EnvTFDataDir, ".custom-terraform")

	// Should not panic when directory doesn't exist.
	handleTFDataDir(tempDir, "test")
}

func TestHandleTFDataDir_DeletesExistingDir(t *testing.T) {
	tempDir := t.TempDir()

	// Create the TF_DATA_DIR directory.
	tfDataDirName := ".custom-terraform"
	tfDataDirPath := filepath.Join(tempDir, tfDataDirName)
	require.NoError(t, os.MkdirAll(tfDataDirPath, 0o755))

	// Create a file inside.
	require.NoError(t, os.WriteFile(filepath.Join(tfDataDirPath, "provider"), []byte("data"), 0o644))

	// Set TF_DATA_DIR.
	t.Setenv(EnvTFDataDir, tfDataDirName)

	// Should delete the directory.
	handleTFDataDir(tempDir, "test")

	// Verify directory is deleted.
	assert.NoDirExists(t, tfDataDirPath)
}

// TestDeleteFolders_WithMixedFiles tests deleting folders with both files and directories.
func TestDeleteFolders_WithMixedFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create component folder.
	componentDir := filepath.Join(tempDir, "component")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	// Create a regular file.
	lockFile := filepath.Join(componentDir, ".terraform.lock.hcl")
	require.NoError(t, os.WriteFile(lockFile, []byte("lock"), 0o644))

	// Create a directory.
	tfDir := filepath.Join(componentDir, ".terraform")
	require.NoError(t, os.MkdirAll(tfDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tfDir, "providers.json"), []byte("{}"), 0o644))

	folders := []Directory{
		{
			Name:         "component",
			FullPath:     componentDir,
			RelativePath: "component",
			Files: []ObjectInfo{
				{FullPath: lockFile, RelativePath: ".terraform.lock.hcl", Name: ".terraform.lock.hcl", IsDir: false},
				{FullPath: tfDir, RelativePath: ".terraform", Name: ".terraform", IsDir: true},
			},
		},
	}

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
	}

	deleteFolders(folders, "component", atmosConfig)

	// Both file and directory should be deleted.
	assert.NoFileExists(t, lockFile)
	assert.NoDirExists(t, tfDir)
}

// TestExecuteCleanDeletion_WithTFDataDirFolders tests deletion with TF_DATA_DIR folders.
func TestExecuteCleanDeletion_WithTFDataDirFolders(t *testing.T) {
	tempDir := t.TempDir()

	// Create component folder.
	componentDir := filepath.Join(tempDir, "component")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	// Create a file to delete in regular folders.
	lockFile := filepath.Join(componentDir, ".terraform.lock.hcl")
	require.NoError(t, os.WriteFile(lockFile, []byte("lock"), 0o644))

	// Create a TF_DATA_DIR folder.
	tfDataDir := filepath.Join(componentDir, ".custom-terraform")
	require.NoError(t, os.MkdirAll(tfDataDir, 0o755))

	folders := []Directory{
		{
			Name:         "component",
			FullPath:     componentDir,
			RelativePath: "component",
			Files: []ObjectInfo{
				{FullPath: lockFile, RelativePath: ".terraform.lock.hcl", Name: ".terraform.lock.hcl", IsDir: false},
			},
		},
	}

	// Set TF_DATA_DIR.
	t.Setenv(EnvTFDataDir, ".custom-terraform")

	tfDataDirFolders := []Directory{
		{
			Name:     ".custom-terraform",
			FullPath: componentDir,
		},
	}

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
	}

	executeCleanDeletion(folders, tfDataDirFolders, "component", atmosConfig)

	// Regular file should be deleted.
	assert.NoFileExists(t, lockFile)

	// TF_DATA_DIR should also be deleted.
	assert.NoDirExists(t, tfDataDir)
}

// TestDeleteFolders_HandlesRelativePathError tests handling when relative path fails.
func TestDeleteFolders_HandlesRelativePathError(t *testing.T) {
	tempDir := t.TempDir()

	// Create folder and file.
	folder := filepath.Join(tempDir, "folder")
	require.NoError(t, os.MkdirAll(folder, 0o755))
	testFile := filepath.Join(folder, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("content"), 0o644))

	folders := []Directory{
		{
			Name:         "folder",
			FullPath:     folder,
			RelativePath: "folder",
			Files: []ObjectInfo{
				{FullPath: testFile, RelativePath: "folder/test.txt", Name: "test.txt", IsDir: false},
			},
		},
	}

	// Use an invalid base path to trigger relative path fallback.
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: "/nonexistent/path",
	}

	// Should not panic and should still attempt deletion.
	deleteFolders(folders, "test", atmosConfig)

	// File should be deleted regardless of relative path error.
	assert.NoFileExists(t, testFile)
}

// TestExecuteCleanDeletion_MultipleTFDataDirFolders tests deletion with multiple TF_DATA_DIR folders.
func TestExecuteCleanDeletion_MultipleTFDataDirFolders(t *testing.T) {
	tempDir := t.TempDir()

	// Create multiple component folders.
	comp1Dir := filepath.Join(tempDir, "component1")
	comp2Dir := filepath.Join(tempDir, "component2")
	require.NoError(t, os.MkdirAll(comp1Dir, 0o755))
	require.NoError(t, os.MkdirAll(comp2Dir, 0o755))

	// Create TF_DATA_DIR in each component.
	tfDataDir1 := filepath.Join(comp1Dir, ".tf-data")
	tfDataDir2 := filepath.Join(comp2Dir, ".tf-data")
	require.NoError(t, os.MkdirAll(tfDataDir1, 0o755))
	require.NoError(t, os.MkdirAll(tfDataDir2, 0o755))

	t.Setenv(EnvTFDataDir, ".tf-data")

	tfDataDirFolders := []Directory{
		{Name: ".tf-data", FullPath: comp1Dir},
		{Name: ".tf-data", FullPath: comp2Dir},
	}

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
	}

	executeCleanDeletion([]Directory{}, tfDataDirFolders, "test", atmosConfig)

	// Both TF_DATA_DIR folders should be deleted.
	assert.NoDirExists(t, tfDataDir1)
	assert.NoDirExists(t, tfDataDir2)
}
