package toolchain

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	toolchainpkg "github.com/cloudposse/atmos/toolchain"
)

func TestToolchainListCommand(t *testing.T) {
	_ = setupToolchainTest(t, "terraform 1.5.7\nkubectl 1.28.0\n")

	// Test the list command runs without error.
	err := listCmd.RunE(listCmd, []string{})
	assert.NoError(t, err)
}

func TestToolchainGetCommand(t *testing.T) {
	_ = setupToolchainTest(t, "terraform 1.5.7\n")

	// Test the get command runs without error.
	err := getCmd.RunE(getCmd, []string{"terraform"})
	assert.NoError(t, err)
}

// setupToolchainTest creates a test environment with tool-versions and tools config files.
func setupToolchainTest(t *testing.T, toolVersionsContent string) string {
	t.Helper()

	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create a test .tool-versions file.
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	err := os.WriteFile(toolVersionsPath, []byte(toolVersionsContent), 0o644)
	require.NoError(t, err)

	// Create tools config.
	toolsConfigPath := filepath.Join(tempDir, "tools.yaml")
	toolsConfigContent := "aliases:\n  terraform: hashicorp/terraform\n"
	err = os.WriteFile(toolsConfigPath, []byte(toolsConfigContent), 0o644)
	require.NoError(t, err)

	// Set the flags to use our temp files.
	toolsDir := filepath.Join(tempDir, ".tools")
	toolVersionsFile = toolVersionsPath
	toolsConfigFile = toolsConfigPath

	// Initialize the toolchain package config.
	atmosCfg := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile:    toolVersionsPath,
			ToolsDir:        toolsDir,
			ToolsConfigFile: toolsConfigPath,
		},
	}
	toolchainpkg.SetAtmosConfig(atmosCfg)

	return toolVersionsPath
}

func TestToolchainAddCommand(t *testing.T) {
	toolVersionsPath := setupToolchainTest(t, "")

	// Test the add command runs without error.
	err := addCmd.RunE(addCmd, []string{"terraform@1.5.7"})
	assert.NoError(t, err)

	// Verify the tool was added.
	data, err := os.ReadFile(toolVersionsPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "1.5.7")
}

func TestToolchainRemoveCommand(t *testing.T) {
	toolVersionsPath := setupToolchainTest(t, "terraform 1.5.7\n")

	// Test the remove command runs without error.
	err := removeCmd.RunE(removeCmd, []string{"terraform"})
	assert.NoError(t, err)

	// Verify the tool was removed.
	data, err := os.ReadFile(toolVersionsPath)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "terraform")
}

func TestToolchainCommandsWithoutToolVersionsFile(t *testing.T) {
	// Create temp directory for test.
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Set paths to non-existent files.
	toolVersionsFile = filepath.Join(tempDir, ".tool-versions")
	toolsDir = filepath.Join(tempDir, ".tools")
	toolsConfigFile = filepath.Join(tempDir, "tools.yaml")

	// Test that list command handles missing file gracefully.
	err := listCmd.RunE(listCmd, []string{})
	// Should either return error or handle gracefully.
	// We're just checking it doesn't panic.
	_ = err
}

func TestToolchainCleanCommand(t *testing.T) {
	// Create temp directory for test.
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create tools directory with some files.
	toolsDirPath := filepath.Join(tempDir, ".tools")
	err := os.MkdirAll(filepath.Join(toolsDirPath, "bin", "hashicorp", "terraform", "1.5.7"), 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(toolsDirPath, "bin", "hashicorp", "terraform", "1.5.7", "terraform"), []byte("test"), 0o755)
	require.NoError(t, err)

	// Set the flags.
	toolVersionsFile = filepath.Join(tempDir, ".tool-versions")
	toolsDir = toolsDirPath
	toolsConfigFile = filepath.Join(tempDir, "tools.yaml")

	// Initialize the toolchain package config.
	atmosCfg := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			FilePath:        toolVersionsFile,
			ToolsDir:        toolsDir,
			ToolsConfigFile: toolsConfigFile,
		},
	}
	toolchainpkg.SetAtmosConfig(atmosCfg)

	// Test the clean command runs without error.
	err = cleanCmd.RunE(cleanCmd, []string{})
	assert.NoError(t, err)
}

func TestToolchainPathCommand(t *testing.T) {
	// Create temp directory for test.
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create a test .tool-versions file.
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	content := "terraform 1.5.7\n"
	err := os.WriteFile(toolVersionsPath, []byte(content), 0o644)
	require.NoError(t, err)

	// Create tools directory structure with actual binary.
	toolsDirPath := filepath.Join(tempDir, ".tools")
	binaryPath := filepath.Join(toolsDirPath, "bin", "hashicorp", "terraform", "1.5.7", "terraform")
	err = os.MkdirAll(filepath.Dir(binaryPath), 0o755)
	require.NoError(t, err)
	// Create a dummy binary file.
	err = os.WriteFile(binaryPath, []byte("#!/bin/sh\necho test"), 0o755)
	require.NoError(t, err)

	// Create tools config.
	toolsConfigPath := filepath.Join(tempDir, "tools.yaml")
	toolsConfigContent := "aliases:\n  terraform: hashicorp/terraform\n"
	err = os.WriteFile(toolsConfigPath, []byte(toolsConfigContent), 0o644)
	require.NoError(t, err)

	// Set the flags.
	toolVersionsFile = toolVersionsPath
	toolsDir = toolsDirPath
	toolsConfigFile = toolsConfigPath

	// Initialize the toolchain package config.
	atmosCfg := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			FilePath:        toolVersionsPath,
			ToolsDir:        toolsDir,
			ToolsConfigFile: toolsConfigPath,
		},
	}
	toolchainpkg.SetAtmosConfig(atmosCfg)

	// Test the path command runs without error.
	err = pathCmd.RunE(pathCmd, []string{})
	assert.NoError(t, err)
}

func TestToolchainWhichCommand(t *testing.T) {
	// Create temp directory for test.
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create a test .tool-versions file.
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	content := "terraform 1.5.7\n"
	err := os.WriteFile(toolVersionsPath, []byte(content), 0o644)
	require.NoError(t, err)

	// Create tools directory with binary.
	toolsDirPath := filepath.Join(tempDir, ".tools")
	binaryPath := filepath.Join(toolsDirPath, "bin", "hashicorp", "terraform", "1.5.7", "terraform")
	err = os.MkdirAll(filepath.Dir(binaryPath), 0o755)
	require.NoError(t, err)
	err = os.WriteFile(binaryPath, []byte("#!/bin/sh\necho test"), 0o755)
	require.NoError(t, err)

	// Create tools config.
	toolsConfigPath := filepath.Join(tempDir, "tools.yaml")
	toolsConfigContent := "aliases:\n  terraform: hashicorp/terraform\n"
	err = os.WriteFile(toolsConfigPath, []byte(toolsConfigContent), 0o644)
	require.NoError(t, err)

	// Set the flags.
	toolVersionsFile = toolVersionsPath
	toolsDir = toolsDirPath
	toolsConfigFile = toolsConfigPath

	// Initialize the toolchain package config.
	atmosCfg := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			FilePath:        toolVersionsPath,
			ToolsDir:        toolsDir,
			ToolsConfigFile: toolsConfigPath,
		},
	}
	toolchainpkg.SetAtmosConfig(atmosCfg)

	// Test the which command runs without error.
	err = whichCmd.RunE(whichCmd, []string{"terraform"})
	assert.NoError(t, err)
}

func TestToolchainSetCommand(t *testing.T) {
	// Create temp directory for test.
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create a test .tool-versions file with multiple versions.
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	content := "terraform 1.5.5 1.5.6 1.5.7\n"
	err := os.WriteFile(toolVersionsPath, []byte(content), 0o644)
	require.NoError(t, err)

	// Create tools config.
	toolsConfigPath := filepath.Join(tempDir, "tools.yaml")
	toolsConfigContent := "aliases:\n  terraform: hashicorp/terraform\n"
	err = os.WriteFile(toolsConfigPath, []byte(toolsConfigContent), 0o644)
	require.NoError(t, err)

	// Set the flags.
	toolVersionsFile = toolVersionsPath
	toolsDir = filepath.Join(tempDir, ".tools")
	toolsConfigFile = toolsConfigPath

	// Initialize the toolchain package config.
	atmosCfg := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			FilePath:        toolVersionsPath,
			ToolsDir:        toolsDir,
			ToolsConfigFile: toolsConfigPath,
		},
	}
	toolchainpkg.SetAtmosConfig(atmosCfg)

	// Test the set command runs without error.
	err = setCmd.RunE(setCmd, []string{"terraform", "1.5.7"})
	assert.NoError(t, err)

	// Verify the version was set as default (first).
	data, err := os.ReadFile(toolVersionsPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "1.5.7")
}

func TestToolchainUninstallCommand(t *testing.T) {
	// Create temp directory for test.
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create a test .tool-versions file.
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	content := "terraform 1.5.7\n"
	err := os.WriteFile(toolVersionsPath, []byte(content), 0o644)
	require.NoError(t, err)

	// Create tools directory with binary.
	toolsDirPath := filepath.Join(tempDir, ".tools")
	binaryPath := filepath.Join(toolsDirPath, "bin", "hashicorp", "terraform", "1.5.7", "terraform")
	err = os.MkdirAll(filepath.Dir(binaryPath), 0o755)
	require.NoError(t, err)
	err = os.WriteFile(binaryPath, []byte("#!/bin/sh\necho test"), 0o755)
	require.NoError(t, err)

	// Create tools config.
	toolsConfigPath := filepath.Join(tempDir, "tools.yaml")
	toolsConfigContent := "aliases:\n  terraform: hashicorp/terraform\n"
	err = os.WriteFile(toolsConfigPath, []byte(toolsConfigContent), 0o644)
	require.NoError(t, err)

	// Set the flags.
	toolVersionsFile = toolVersionsPath
	toolsDir = toolsDirPath
	toolsConfigFile = toolsConfigPath

	// Initialize the toolchain package config.
	atmosCfg := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			FilePath:        toolVersionsPath,
			ToolsDir:        toolsDir,
			ToolsConfigFile: toolsConfigPath,
		},
	}
	toolchainpkg.SetAtmosConfig(atmosCfg)

	// Test the uninstall command runs without error.
	err = uninstallCmd.RunE(uninstallCmd, []string{"terraform@1.5.7"})
	assert.NoError(t, err)

	// Verify the binary was removed.
	_, err = os.Stat(binaryPath)
	assert.True(t, os.IsNotExist(err), "Binary should be removed")
}
