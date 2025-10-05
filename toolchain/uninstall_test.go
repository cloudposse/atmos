package toolchain

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
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
		err := os.MkdirAll(versionDir, defaultMkdirPermissions)
		require.NoError(t, err)
		err = os.WriteFile(binaryPath, []byte("mock binary"), defaultMkdirPermissions)
		require.NoError(t, err)
		latestFile := filepath.Join(tempDir, owner, repo, "latest")
		err = os.WriteFile(latestFile, []byte(actualVersion), defaultFileWritePermissions)
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
		err := os.MkdirAll(filepath.Join(tempDir, owner, repo), defaultMkdirPermissions)
		require.NoError(t, err)
		err = os.WriteFile(latestFile, []byte(actualVersion), defaultFileWritePermissions)
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
	err := os.MkdirAll(versionDir, defaultMkdirPermissions)
	require.NoError(t, err)
	binaryPath := filepath.Join(versionDir, repo)
	err = os.WriteFile(binaryPath, []byte("mock binary"), defaultMkdirPermissions)
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

func TestUninstallWithNoArgs(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)

	// Create a .tool-versions file with some tools
	toolVersionsPath := filepath.Join(tempDir, DefaultToolVersionsFilePath)
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4", "1.9.8"},
			"helm":      {"3.17.4"},
		},
	}
	err := SaveToolVersions(toolVersionsPath, toolVersions)
	require.NoError(t, err)

	// Create mock installed binaries
	installer := NewInstaller()
	installer.binDir = tempDir

	// Create terraform binaries
	terraformPath1 := installer.getBinaryPath("hashicorp", "terraform", "1.11.4")
	err = os.MkdirAll(filepath.Dir(terraformPath1), defaultMkdirPermissions)
	require.NoError(t, err)
	err = os.WriteFile(terraformPath1, []byte("mock terraform 1.11.4"), defaultMkdirPermissions)
	require.NoError(t, err)

	terraformPath2 := installer.getBinaryPath("hashicorp", "terraform", "1.9.8")
	err = os.MkdirAll(filepath.Dir(terraformPath2), defaultMkdirPermissions)
	require.NoError(t, err)
	err = os.WriteFile(terraformPath2, []byte("mock terraform 1.9.8"), defaultMkdirPermissions)
	require.NoError(t, err)

	// Create helm binary
	helmPath := installer.getBinaryPath("helm", "helm", "3.17.4")
	err = os.MkdirAll(filepath.Dir(helmPath), defaultMkdirPermissions)
	require.NoError(t, err)
	err = os.WriteFile(helmPath, []byte("mock helm 3.17.4"), defaultMkdirPermissions)
	require.NoError(t, err)

	// Verify binaries exist
	_, err = os.Stat(terraformPath1)
	assert.NoError(t, err)
	_, err = os.Stat(terraformPath2)
	assert.NoError(t, err)
	_, err = os.Stat(helmPath)
	assert.NoError(t, err)

	// Test uninstall with no arguments by calling uninstallFromToolVersions directly
	err = uninstallFromToolVersions(toolVersionsPath, installer)
	assert.NoError(t, err)

	// Verify all binaries are removed
	_, err = os.Stat(terraformPath1)
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(terraformPath2)
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(helmPath)
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestRunUninstallWithNoArgs(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)

	// Create a .tool-versions file with some tools
	toolVersionsPath := filepath.Join(tempDir, DefaultToolVersionsFilePath)
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4"},
			"helm":      {"3.17.4"},
		},
	}
	err := SaveToolVersions(toolVersionsPath, toolVersions)
	require.NoError(t, err)

	// Create mock installed binaries so uninstall has something to work with
	installer := NewInstaller()
	installer.binDir = tempDir

	// Create terraform binary
	terraformPath := installer.getBinaryPath("hashicorp", "terraform", "1.11.4")
	err = os.MkdirAll(filepath.Dir(terraformPath), defaultMkdirPermissions)
	require.NoError(t, err)
	err = os.WriteFile(terraformPath, []byte("mock terraform 1.11.4"), defaultMkdirPermissions)
	require.NoError(t, err)

	// Create helm binary
	helmPath := installer.getBinaryPath("helm", "helm", "3.17.4")
	err = os.MkdirAll(filepath.Dir(helmPath), defaultMkdirPermissions)
	require.NoError(t, err)
	err = os.WriteFile(helmPath, []byte("mock helm 3.17.4"), defaultMkdirPermissions)
	require.NoError(t, err)

	// Temporarily set the global toolVersionsFile variable
	originalToolVersionsFile := GetToolsConfigFilePath()
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{FilePath: toolVersionsPath}})
	defer func() {
		SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{FilePath: originalToolVersionsFile}})
	}()

	// Test that runUninstall with no arguments doesn't error
	// This prevents regression where the function might error when no specific tool is provided
	cmd := &cobra.Command{}
	err = runUninstall(cmd, []string{})
	assert.NoError(t, err)
}

// FakeInstaller implements the minimal interface your RunUninstall needs
type FakeInstaller struct {
	CalledParseToolSpec   bool
	ParseToolSpecOwner    string
	ParseToolSpecRepo     string
	ParseToolSpecErr      error
	ReadLatestFileVersion string
	ReadLatestFileErr     error
	BinaryExists          bool
	UninstallCalled       bool
	UninstallErr          error
	BinDir                string
}

func (f *FakeInstaller) parseToolSpec(tool string) (string, string, error) {
	f.CalledParseToolSpec = true
	return f.ParseToolSpecOwner, f.ParseToolSpecRepo, f.ParseToolSpecErr
}

func (f *FakeInstaller) ReadLatestFile(owner, repo string) (string, error) {
	return f.ReadLatestFileVersion, f.ReadLatestFileErr
}

func TestRunUninstall(t *testing.T) {
	tests := []struct {
		name         string
		toolSpec     string
		installer    *FakeInstaller
		expectErr    bool
		expectUninst bool
	}{
		{
			name:      "invalid tool spec",
			toolSpec:  "@wrong",
			installer: &FakeInstaller{},
			expectErr: true,
		},
		{
			name:     "uninstall latest but no latest file",
			toolSpec: "tool@latest",
			installer: &FakeInstaller{
				ParseToolSpecOwner: "tool",
				ParseToolSpecRepo:  "tool",
				ReadLatestFileErr:  errors.New("not found"),
			},
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{}})
			err := RunUninstall(tc.toolSpec) // might need to allow DI of installer
			if tc.expectErr && err == nil {
				t.Errorf("expected error but got nil")
			}
			if !tc.expectErr && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
			if tc.expectUninst && !tc.installer.UninstallCalled {
				t.Errorf("expected uninstallSingleTool to be called")
			}
		})
	}
}
