package testhelpers

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCopyFile tests copying a single file with permission preservation.
func TestCopyFile(t *testing.T) {
	// Create temporary directories for test.
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create a source file with specific permissions.
	srcFile := filepath.Join(srcDir, "test.txt")
	content := []byte("test content")
	err := os.WriteFile(srcFile, content, 0o644)
	require.NoError(t, err)

	// Copy the file.
	dstFile := filepath.Join(dstDir, "test.txt")
	err = copyFile(srcFile, dstFile)
	require.NoError(t, err)

	// Verify file exists.
	assert.FileExists(t, dstFile)

	// Verify content matches.
	dstContent, err := os.ReadFile(dstFile)
	require.NoError(t, err)
	assert.Equal(t, content, dstContent)

	// Verify permissions are preserved.
	srcInfo, err := os.Stat(srcFile)
	require.NoError(t, err)
	dstInfo, err := os.Stat(dstFile)
	require.NoError(t, err)
	assert.Equal(t, srcInfo.Mode(), dstInfo.Mode())
}

// TestCopyFileExecutable tests copying an executable file.
func TestCopyFileExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("Skipping executable bit test on Windows: Windows does not use Unix execute permissions")
	}

	// Create temporary directories for test.
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create a source file with executable permissions.
	srcFile := filepath.Join(srcDir, "script.sh")
	content := []byte("#!/bin/bash\necho 'test'")
	err := os.WriteFile(srcFile, content, 0o755)
	require.NoError(t, err)

	// Copy the file.
	dstFile := filepath.Join(dstDir, "script.sh")
	err = copyFile(srcFile, dstFile)
	require.NoError(t, err)

	// Verify executable bit is preserved.
	dstInfo, err := os.Stat(dstFile)
	require.NoError(t, err)
	assert.NotEqual(t, os.FileMode(0), dstInfo.Mode()&0o111, "Executable bit should be preserved")
}

// TestCopyDirBasic tests basic directory copying.
func TestCopyDirBasic(t *testing.T) {
	// Create temporary directories for test.
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create source directory structure.
	srcSubDir := filepath.Join(srcDir, "components")
	err := os.MkdirAll(srcSubDir, 0o755)
	require.NoError(t, err)

	// Create files in source.
	file1 := filepath.Join(srcSubDir, "file1.txt")
	file2 := filepath.Join(srcSubDir, "file2.yaml")
	err = os.WriteFile(file1, []byte("content1"), 0o644)
	require.NoError(t, err)
	err = os.WriteFile(file2, []byte("content2"), 0o644)
	require.NoError(t, err)

	// Copy directory.
	dstSubDir := filepath.Join(dstDir, "components")
	err = copyDir(srcSubDir, dstSubDir)
	require.NoError(t, err)

	// Verify files were copied.
	assert.FileExists(t, filepath.Join(dstSubDir, "file1.txt"))
	assert.FileExists(t, filepath.Join(dstSubDir, "file2.yaml"))

	// Verify content matches.
	content, err := os.ReadFile(filepath.Join(dstSubDir, "file1.txt"))
	require.NoError(t, err)
	assert.Equal(t, []byte("content1"), content)
}

// TestCopyDirExcludesTerraformArtifacts tests that terraform artifacts are excluded.
func TestCopyDirExcludesTerraformArtifacts(t *testing.T) {
	// Create temporary directories for test.
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create source directory structure.
	srcComponents := filepath.Join(srcDir, "terraform")
	err := os.MkdirAll(srcComponents, 0o755)
	require.NoError(t, err)

	// Create regular files that should be copied.
	regularFile := filepath.Join(srcComponents, "main.tf")
	err = os.WriteFile(regularFile, []byte("resource {}"), 0o644)
	require.NoError(t, err)

	// Create terraform artifacts that should be excluded.
	terraformDir := filepath.Join(srcComponents, ".terraform")
	err = os.MkdirAll(terraformDir, 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(terraformDir, "provider.tf"), []byte("provider"), 0o644)
	require.NoError(t, err)

	lockFile := filepath.Join(srcComponents, ".terraform.lock.hcl")
	err = os.WriteFile(lockFile, []byte("lock"), 0o644)
	require.NoError(t, err)

	tfvarsFile := filepath.Join(srcComponents, "test.terraform.tfvars.json")
	err = os.WriteFile(tfvarsFile, []byte("{}"), 0o644)
	require.NoError(t, err)

	stateFile := filepath.Join(srcComponents, "terraform.tfstate")
	err = os.WriteFile(stateFile, []byte("{}"), 0o644)
	require.NoError(t, err)

	backupFile := filepath.Join(srcComponents, "terraform.tfstate.backup")
	err = os.WriteFile(backupFile, []byte("{}"), 0o644)
	require.NoError(t, err)

	planFile := filepath.Join(srcComponents, "test.planfile")
	err = os.WriteFile(planFile, []byte("plan"), 0o644)
	require.NoError(t, err)

	backendFile := filepath.Join(srcComponents, "backend.tf.json")
	err = os.WriteFile(backendFile, []byte("{}"), 0o644)
	require.NoError(t, err)

	stateDirFile := filepath.Join(srcComponents, "terraform.tfstate.d")
	err = os.MkdirAll(stateDirFile, 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(stateDirFile, "env.tfstate"), []byte("{}"), 0o644)
	require.NoError(t, err)

	planJSONFile := filepath.Join(srcComponents, "test.planfile.json")
	err = os.WriteFile(planJSONFile, []byte("{}"), 0o644)
	require.NoError(t, err)

	// Copy directory.
	dstComponents := filepath.Join(dstDir, "terraform")
	err = copyDir(srcComponents, dstComponents)
	require.NoError(t, err)

	// Verify regular file was copied.
	assert.FileExists(t, filepath.Join(dstComponents, "main.tf"))

	// Verify terraform artifacts were NOT copied.
	assert.NoDirExists(t, filepath.Join(dstComponents, ".terraform"))
	assert.NoFileExists(t, filepath.Join(dstComponents, ".terraform.lock.hcl"))
	assert.NoFileExists(t, filepath.Join(dstComponents, "test.terraform.tfvars.json"))
	assert.NoFileExists(t, filepath.Join(dstComponents, "terraform.tfstate"))
	assert.NoFileExists(t, filepath.Join(dstComponents, "terraform.tfstate.backup"))
	assert.NoFileExists(t, filepath.Join(dstComponents, "test.planfile"))
	assert.NoFileExists(t, filepath.Join(dstComponents, "backend.tf.json"))
	assert.NoDirExists(t, filepath.Join(dstComponents, "terraform.tfstate.d"))
	assert.NoFileExists(t, filepath.Join(dstComponents, "test.planfile.json"))
}

// TestCopyDirNestedStructure tests copying nested directory structures.
func TestCopyDirNestedStructure(t *testing.T) {
	// Create temporary directories for test.
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create nested directory structure.
	nestedDirs := []string{
		"terraform/vpc",
		"terraform/vpc/modules",
		"terraform/vpc/modules/subnets",
		"terraform/eks",
		"helmfile/releases",
	}

	for _, dir := range nestedDirs {
		fullPath := filepath.Join(srcDir, dir)
		err := os.MkdirAll(fullPath, 0o755)
		require.NoError(t, err)

		// Create a file in each directory.
		testFile := filepath.Join(fullPath, "test.yaml")
		err = os.WriteFile(testFile, []byte("test: "+dir), 0o644)
		require.NoError(t, err)
	}

	// Copy directory.
	err := copyDir(srcDir, dstDir)
	require.NoError(t, err)

	// Verify all nested directories and files were copied.
	for _, dir := range nestedDirs {
		dirPath := filepath.Join(dstDir, dir)
		assert.DirExists(t, dirPath)

		testFile := filepath.Join(dirPath, "test.yaml")
		assert.FileExists(t, testFile)

		content, err := os.ReadFile(testFile)
		require.NoError(t, err)
		assert.Equal(t, []byte("test: "+dir), content)
	}
}

// TestCopyDirPreservesPermissions tests that directory permissions are preserved.
func TestCopyDirPreservesPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("Skipping permission preservation test on Windows: Windows uses a different permission model than Unix")
	}

	// Create temporary directories for test.
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create files with different permissions.
	readOnlyFile := filepath.Join(srcDir, "readonly.txt")
	err := os.WriteFile(readOnlyFile, []byte("readonly"), 0o444)
	require.NoError(t, err)

	executableFile := filepath.Join(srcDir, "script.sh")
	err = os.WriteFile(executableFile, []byte("#!/bin/bash"), 0o755)
	require.NoError(t, err)

	normalFile := filepath.Join(srcDir, "normal.txt")
	err = os.WriteFile(normalFile, []byte("normal"), 0o644)
	require.NoError(t, err)

	// Copy directory.
	err = copyDir(srcDir, dstDir)
	require.NoError(t, err)

	// Verify permissions are preserved.
	readOnlyInfo, err := os.Stat(filepath.Join(dstDir, "readonly.txt"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o444), readOnlyInfo.Mode().Perm())

	executableInfo, err := os.Stat(filepath.Join(dstDir, "script.sh"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), executableInfo.Mode().Perm())

	normalInfo, err := os.Stat(filepath.Join(dstDir, "normal.txt"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), normalInfo.Mode().Perm())
}

// TestCopyDirEmptyDirectory tests copying an empty directory.
func TestCopyDirEmptyDirectory(t *testing.T) {
	// Create temporary directories for test.
	srcDir := t.TempDir()
	dstDir := filepath.Join(t.TempDir(), "empty")

	// Copy empty directory.
	err := copyDir(srcDir, dstDir)
	require.NoError(t, err)

	// Verify destination directory exists.
	assert.DirExists(t, dstDir)

	// Verify it's empty.
	entries, err := os.ReadDir(dstDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}
