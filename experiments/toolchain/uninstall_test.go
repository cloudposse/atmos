package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUninstallCleansUpLatestFile_Present(t *testing.T) {
	t.Run("both binary and latest file exist", func(t *testing.T) {
		tempDir := t.TempDir()
		os.Setenv("HOME", tempDir)

		installer := NewInstaller()
		installer.binDir = tempDir
		owner := "hashicorp"
		repo := "terraform"
		actualVersion := "1.9.8"

		// Simulate install: create versioned binary and latest file
		binaryPath := installer.getBinaryPath(owner, repo, actualVersion)
		versionDir := filepath.Dir(binaryPath)
		err := os.MkdirAll(versionDir, 0755)
		require.NoError(t, err)
		err = os.WriteFile(binaryPath, []byte("mock binary"), 0755)
		require.NoError(t, err)
		latestFile := filepath.Join(tempDir, owner, repo, "latest")
		err = os.WriteFile(latestFile, []byte(actualVersion), 0644)
		require.NoError(t, err)

		// Ensure latest file exists
		_, err = os.Stat(latestFile)
		assert.NoError(t, err)

		// Uninstall with @latest
		cmd := &cobra.Command{}
		cmd.SetArgs([]string{"hashicorp/terraform@latest"})
		err = runUninstallWithInstaller(cmd, []string{"hashicorp/terraform@latest"}, installer)
		assert.NoError(t, err)

		// latest file should be gone
		_, err = os.Stat(latestFile)
		assert.Error(t, err)
		assert.True(t, os.IsNotExist(err))

		// versioned binary should be gone
		_, err = os.Stat(binaryPath)
		assert.Error(t, err)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("only latest file exists", func(t *testing.T) {
		tempDir := t.TempDir()
		os.Setenv("HOME", tempDir)

		installer := NewInstaller()
		installer.binDir = tempDir
		owner := "hashicorp"
		repo := "terraform"
		actualVersion := "1.9.8"

		// Do NOT create versioned binary, only latest file
		latestFile := filepath.Join(tempDir, owner, repo, "latest")
		err := os.MkdirAll(filepath.Join(tempDir, owner, repo), 0755)
		require.NoError(t, err)
		err = os.WriteFile(latestFile, []byte(actualVersion), 0644)
		require.NoError(t, err)

		// Ensure latest file exists
		_, err = os.Stat(latestFile)
		assert.NoError(t, err)

		// Uninstall with @latest
		cmd := &cobra.Command{}
		cmd.SetArgs([]string{"hashicorp/terraform@latest"})
		err = runUninstallWithInstaller(cmd, []string{"hashicorp/terraform@latest"}, installer)
		assert.NoError(t, err)

		// latest file should be gone
		_, err = os.Stat(latestFile)
		assert.Error(t, err)
		assert.True(t, os.IsNotExist(err))

		// versioned binary should not exist (and that's fine)
		binaryPath := filepath.Join(tempDir, owner, repo, actualVersion, repo)
		_, err = os.Stat(binaryPath)
		assert.Error(t, err)
		assert.True(t, os.IsNotExist(err))
	})
}

func TestUninstallCleansUpLatestFile_Missing(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)

	installer := NewInstaller()
	installer.binDir = tempDir
	owner := "hashicorp"
	repo := "terraform"
	actualVersion := "1.9.8"

	// Simulate install: create versioned binary but NO latest file
	versionDir := filepath.Join(tempDir, owner, repo, actualVersion)
	err := os.MkdirAll(versionDir, 0755)
	require.NoError(t, err)
	binaryPath := filepath.Join(versionDir, repo)
	err = os.WriteFile(binaryPath, []byte("mock binary"), 0755)
	require.NoError(t, err)
	latestFile := filepath.Join(tempDir, owner, repo, "latest")
	_ = os.Remove(latestFile) // Ensure latest file does not exist

	// Uninstall with @latest
	cmd := &cobra.Command{}
	cmd.SetArgs([]string{"hashicorp/terraform@latest"})
	err = runUninstallWithInstaller(cmd, []string{"hashicorp/terraform@latest"}, installer)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no latest file found")

	// latest file should still not exist
	_, statErr := os.Stat(latestFile)
	assert.Error(t, statErr)
	assert.True(t, os.IsNotExist(statErr))

	// versioned binary should still exist (not deleted)
	_, statErr = os.Stat(binaryPath)
	assert.NoError(t, statErr)
}
