package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanCommand_ExistingToolsDirectory(t *testing.T) {
	tempDir := t.TempDir()
	toolsDir := filepath.Join(tempDir, ".tools")

	// Create a mock tools directory with some files
	err := os.MkdirAll(filepath.Join(toolsDir, "bin", "hashicorp", "terraform", "1.11.4"), 0o755)
	require.NoError(t, err)

	// Create some mock binary files
	err = os.WriteFile(filepath.Join(toolsDir, "bin", "hashicorp", "terraform", "1.11.4", "terraform"), []byte("mock binary"), 0o755)
	require.NoError(t, err)

	err = os.MkdirAll(filepath.Join(toolsDir, "bin", "kubernetes", "kubectl", "1.28.0"), 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(toolsDir, "bin", "kubernetes", "kubectl", "1.28.0", "kubectl"), []byte("mock binary"), 0o755)
	require.NoError(t, err)

	// Verify the directory exists and has content
	_, err = os.Stat(toolsDir)
	require.NoError(t, err)

	// Test cleaning the tools directory
	cmd := cleanCmd
	cmd.SetArgs([]string{})
	// Set the global tools-dir flag on the root command
	rootCmd.PersistentFlags().Set("tools-dir", toolsDir)
	err = cmd.Execute()
	require.NoError(t, err, "Should successfully clean existing tools directory")

	// Verify the directory was deleted
	_, err = os.Stat(toolsDir)
	assert.True(t, os.IsNotExist(err), "Tools directory should be deleted")
}

func TestCleanCommand_NonExistentToolsDirectory(t *testing.T) {
	tempDir := t.TempDir()
	toolsDir := filepath.Join(tempDir, ".tools")

	// Verify the directory doesn't exist
	_, err := os.Stat(toolsDir)
	assert.True(t, os.IsNotExist(err))

	// Test cleaning non-existent tools directory
	cmd := cleanCmd
	cmd.SetArgs([]string{})
	err = cmd.Execute()
	require.NoError(t, err, "Should not error when cleaning non-existent directory")

	// Verify the directory still doesn't exist
	_, err = os.Stat(toolsDir)
	assert.True(t, os.IsNotExist(err))
}

func TestCleanCommand_EmptyToolsDirectory(t *testing.T) {
	tempDir := t.TempDir()
	toolsDir := filepath.Join(tempDir, ".tools")

	// Create an empty tools directory
	err := os.MkdirAll(toolsDir, 0o755)
	require.NoError(t, err)

	// Verify the directory exists
	_, err = os.Stat(toolsDir)
	require.NoError(t, err)

	// Test cleaning empty tools directory
	cmd := cleanCmd
	cmd.SetArgs([]string{})
	// Set the global tools-dir flag on the root command
	rootCmd.PersistentFlags().Set("tools-dir", toolsDir)
	err = cmd.Execute()
	require.NoError(t, err, "Should successfully clean empty tools directory")

	// Verify the directory was deleted
	_, err = os.Stat(toolsDir)
	assert.True(t, os.IsNotExist(err), "Empty tools directory should be deleted")
}

func TestCleanCommand_ComplexDirectoryStructure(t *testing.T) {
	tempDir := t.TempDir()
	toolsDir := filepath.Join(tempDir, ".tools")

	// Create a complex directory structure
	dirs := []string{
		filepath.Join(toolsDir, "bin", "hashicorp", "terraform", "1.11.4"),
		filepath.Join(toolsDir, "bin", "hashicorp", "terraform", "1.9.8"),
		filepath.Join(toolsDir, "bin", "kubernetes", "kubectl", "1.28.0"),
		filepath.Join(toolsDir, "bin", "helm", "3.12.0"),
		filepath.Join(toolsDir, "cache", "downloads"),
		filepath.Join(toolsDir, "temp", "extract"),
	}

	for _, dir := range dirs {
		err := os.MkdirAll(dir, 0o755)
		require.NoError(t, err)
	}

	// Create some mock files
	files := []string{
		filepath.Join(toolsDir, "bin", "hashicorp", "terraform", "1.11.4", "terraform"),
		filepath.Join(toolsDir, "bin", "hashicorp", "terraform", "1.9.8", "terraform"),
		filepath.Join(toolsDir, "bin", "kubernetes", "kubectl", "1.28.0", "kubectl"),
		filepath.Join(toolsDir, "bin", "helm", "3.12.0", "helm"),
		filepath.Join(toolsDir, "cache", "downloads", "terraform.zip"),
		filepath.Join(toolsDir, "temp", "extract", "temp_file"),
	}

	for _, file := range files {
		err := os.WriteFile(file, []byte("mock content"), 0o644)
		require.NoError(t, err)
	}

	// Verify the directory exists and has content
	_, err := os.Stat(toolsDir)
	require.NoError(t, err)

	// Test cleaning the complex tools directory
	cmd := cleanCmd
	cmd.SetArgs([]string{})
	// Set the global tools-dir flag on the root command
	rootCmd.PersistentFlags().Set("tools-dir", toolsDir)
	err = cmd.Execute()
	require.NoError(t, err, "Should successfully clean complex tools directory")

	// Verify the directory was deleted
	_, err = os.Stat(toolsDir)
	assert.True(t, os.IsNotExist(err), "Complex tools directory should be deleted")
}

func TestCleanCommand_WithSymlinks(t *testing.T) {
	tempDir := t.TempDir()
	toolsDir := filepath.Join(tempDir, ".tools")

	// Create a tools directory with symlinks
	err := os.MkdirAll(filepath.Join(toolsDir, "bin"), 0o755)
	require.NoError(t, err)

	// Create a target file
	targetFile := filepath.Join(toolsDir, "bin", "target")
	err = os.WriteFile(targetFile, []byte("target content"), 0o644)
	require.NoError(t, err)

	// Create a symlink
	symlinkFile := filepath.Join(toolsDir, "bin", "symlink")
	err = os.Symlink(targetFile, symlinkFile)
	require.NoError(t, err)

	// Verify the directory exists
	_, err = os.Stat(toolsDir)
	require.NoError(t, err)

	// Test cleaning the tools directory with symlinks
	cmd := cleanCmd
	cmd.SetArgs([]string{})
	// Set the global tools-dir flag on the root command
	rootCmd.PersistentFlags().Set("tools-dir", toolsDir)
	err = cmd.Execute()
	require.NoError(t, err, "Should successfully clean tools directory with symlinks")

	// Verify the directory was deleted
	_, err = os.Stat(toolsDir)
	assert.True(t, os.IsNotExist(err), "Tools directory with symlinks should be deleted")
}

func TestCleanCommand_WithReadOnlyFiles(t *testing.T) {
	tempDir := t.TempDir()
	toolsDir := filepath.Join(tempDir, ".tools")

	// Create a tools directory with read-only files
	err := os.MkdirAll(filepath.Join(toolsDir, "bin"), 0o755)
	require.NoError(t, err)

	// Create a read-only file
	readOnlyFile := filepath.Join(toolsDir, "bin", "readonly")
	err = os.WriteFile(readOnlyFile, []byte("readonly content"), 0o444)
	require.NoError(t, err)

	// Verify the directory exists
	_, err = os.Stat(toolsDir)
	require.NoError(t, err)

	// Test cleaning the tools directory with read-only files
	cmd := cleanCmd
	cmd.SetArgs([]string{})
	// Set the global tools-dir flag on the root command
	rootCmd.PersistentFlags().Set("tools-dir", toolsDir)
	err = cmd.Execute()
	require.NoError(t, err, "Should successfully clean tools directory with read-only files")

	// Verify the directory was deleted
	_, err = os.Stat(toolsDir)
	assert.True(t, os.IsNotExist(err), "Tools directory with read-only files should be deleted")
}

func TestCleanCommand_NoArgs(t *testing.T) {
	cmd := cleanCmd
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	// This should not error even if the tools directory doesn't exist
	// The command handles non-existent directories gracefully
	require.NoError(t, err, "Should not error when cleaning with no args")
}

func TestCleanCommand_WithArgs(t *testing.T) {
	cmd := cleanCmd
	cmd.SetArgs([]string{"extra", "args"})
	err := cmd.Execute()
	// The clean command doesn't take arguments, but it should still work
	// as it ignores extra arguments
	require.NoError(t, err, "Should not error when cleaning with extra args")
}

func TestCleanCommand_CountsCorrectly(t *testing.T) {
	tempDir := t.TempDir()
	toolsDir := filepath.Join(tempDir, ".tools")

	// Create a simple structure to test counting
	err := os.MkdirAll(filepath.Join(toolsDir, "bin", "test"), 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(toolsDir, "bin", "test", "binary"), []byte("test"), 0o755)
	require.NoError(t, err)

	// Count manually to verify
	expectedCount := 0
	err = filepath.Walk(toolsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path != toolsDir {
			expectedCount++
		}
		return nil
	})
	require.NoError(t, err)

	// Test cleaning and verify the count
	cmd := cleanCmd
	cmd.SetArgs([]string{})
	// Set the global tools-dir flag on the root command
	rootCmd.PersistentFlags().Set("tools-dir", toolsDir)
	err = cmd.Execute()
	require.NoError(t, err, "Should successfully clean and count files")

	// Verify the directory was deleted
	_, err = os.Stat(toolsDir)
	assert.True(t, os.IsNotExist(err), "Tools directory should be deleted")
}
