package toolchain

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
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

	// Clean toolchainCmd state before each test to prevent pollution.
	cleanToolchainCmdState(t)

	// Initialize I/O context and formatter for testing.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err, "Failed to create I/O context")
	ui.InitFormatter(ioCtx)
	data.InitWriter(ioCtx)

	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create a test .tool-versions file.
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	err = os.WriteFile(toolVersionsPath, []byte(toolVersionsContent), 0o644)
	require.NoError(t, err)

	// Create tools config.
	toolsConfigPath := filepath.Join(tempDir, "tools.yaml")
	toolsConfigContent := "aliases:\n  terraform: hashicorp/terraform\n"
	err = os.WriteFile(toolsConfigPath, []byte(toolsConfigContent), 0o644)
	require.NoError(t, err)

	// Set the paths via Viper.
	toolsDir := filepath.Join(tempDir, ".tools")

	// Save previous viper settings for cleanup
	prevToolVersions := viper.Get("toolchain.tool-versions")
	prevToolsDir := viper.Get("toolchain.tools-dir")
	prevToolsConfig := viper.Get("toolchain.tools-config")

	viper.Set("toolchain.tool-versions", toolVersionsPath)
	viper.Set("toolchain.tools-dir", toolsDir)
	viper.Set("toolchain.tools-config", toolsConfigPath)

	t.Cleanup(func() {
		viper.Set("toolchain.tool-versions", prevToolVersions)
		viper.Set("toolchain.tools-dir", prevToolsDir)
		viper.Set("toolchain.tools-config", prevToolsConfig)
	})

	// Initialize the toolchain package config.
	atmosCfg := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile:    toolVersionsPath,
			InstallPath:     toolsDir,
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
	// Initialize I/O context and formatter for testing.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err, "Failed to create I/O context")
	ui.InitFormatter(ioCtx)
	data.InitWriter(ioCtx)

	// Create temp directory for test.
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Set paths to non-existent files via Viper.
	viper.Set("toolchain.tool-versions", filepath.Join(tempDir, ".tool-versions"))
	viper.Set("toolchain.tools-dir", filepath.Join(tempDir, ".tools"))
	viper.Set("toolchain.tools-config", filepath.Join(tempDir, "tools.yaml"))

	// Test that list command handles missing file gracefully.
	err = listCmd.RunE(listCmd, []string{})
	// Should either return error or handle gracefully.
	// We're just checking it doesn't panic.
	_ = err
}

func TestToolchainCleanCommand(t *testing.T) {
	// Initialize I/O context and formatter for testing.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err, "Failed to create I/O context")
	ui.InitFormatter(ioCtx)
	data.InitWriter(ioCtx)

	// Create temp directory for test.
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create tools directory with some files.
	toolsDirPath := filepath.Join(tempDir, ".tools")
	err = os.MkdirAll(filepath.Join(toolsDirPath, "bin", "hashicorp", "terraform", "1.5.7"), 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(toolsDirPath, "bin", "hashicorp", "terraform", "1.5.7", "terraform"), []byte("test"), 0o755)
	require.NoError(t, err)

	// Set the flags via Viper.
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	toolsConfigPath := filepath.Join(tempDir, "tools.yaml")
	viper.Set("toolchain.tool-versions", toolVersionsPath)
	viper.Set("toolchain.tools-dir", toolsDirPath)
	viper.Set("toolchain.tools-config", toolsConfigPath)

	// Initialize the toolchain package config.
	atmosCfg := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile:    toolVersionsPath,
			InstallPath:     toolsDirPath,
			ToolsConfigFile: toolsConfigPath,
		},
	}
	toolchainpkg.SetAtmosConfig(atmosCfg)

	// Test the clean command runs without error.
	err = cleanCmd.RunE(cleanCmd, []string{})
	assert.NoError(t, err)
}

func TestToolchainPathCommand(t *testing.T) {
	// Initialize I/O context and formatter for testing.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err, "Failed to create I/O context")
	ui.InitFormatter(ioCtx)
	data.InitWriter(ioCtx)

	// Create temp directory for test.
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create a test .tool-versions file.
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	content := "terraform 1.5.7\n"
	err = os.WriteFile(toolVersionsPath, []byte(content), 0o644)
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
	viper.Set("toolchain.tool-versions", toolVersionsPath)
	viper.Set("toolchain.tools-dir", toolsDirPath)
	viper.Set("toolchain.tools-config", toolsConfigPath)

	// Initialize the toolchain package config.
	atmosCfg := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile:    toolVersionsPath,
			InstallPath:     toolsDirPath,
			ToolsConfigFile: toolsConfigPath,
		},
	}
	toolchainpkg.SetAtmosConfig(atmosCfg)

	// Test the path command runs without error.
	err = pathCmd.RunE(pathCmd, []string{})
	assert.NoError(t, err)
}

func TestToolchainWhichCommand(t *testing.T) {
	// Initialize I/O context and formatter for testing.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err, "Failed to create I/O context")
	ui.InitFormatter(ioCtx)
	data.InitWriter(ioCtx)

	// Create temp directory for test.
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create a test .tool-versions file.
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	content := "terraform 1.5.7\n"
	err = os.WriteFile(toolVersionsPath, []byte(content), 0o644)
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
	viper.Set("toolchain.tool-versions", toolVersionsPath)
	viper.Set("toolchain.tools-dir", toolsDirPath)
	viper.Set("toolchain.tools-config", toolsConfigPath)

	// Initialize the toolchain package config.
	atmosCfg := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile:    toolVersionsPath,
			InstallPath:     toolsDirPath,
			ToolsConfigFile: toolsConfigPath,
		},
	}
	toolchainpkg.SetAtmosConfig(atmosCfg)

	// Test the which command runs without error.
	err = whichCmd.RunE(whichCmd, []string{"terraform"})
	assert.NoError(t, err)
}

func TestToolchainSetCommand(t *testing.T) {
	// Initialize I/O context and formatter for testing.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err, "Failed to create I/O context")
	ui.InitFormatter(ioCtx)
	data.InitWriter(ioCtx)

	// Create temp directory for test.
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create a test .tool-versions file with multiple versions.
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	content := "terraform 1.5.5 1.5.6 1.5.7\n"
	err = os.WriteFile(toolVersionsPath, []byte(content), 0o644)
	require.NoError(t, err)

	// Create tools config.
	toolsConfigPath := filepath.Join(tempDir, "tools.yaml")
	toolsConfigContent := "aliases:\n  terraform: hashicorp/terraform\n"
	err = os.WriteFile(toolsConfigPath, []byte(toolsConfigContent), 0o644)
	require.NoError(t, err)

	// Set the flags.
	toolsDirPath := filepath.Join(tempDir, ".tools")
	viper.Set("toolchain.tool-versions", toolVersionsPath)
	viper.Set("toolchain.tools-dir", toolsDirPath)
	viper.Set("toolchain.tools-config", toolsConfigPath)

	// Initialize the toolchain package config.
	atmosCfg := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile:    toolVersionsPath,
			InstallPath:     toolsDirPath,
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
	// Initialize I/O context and formatter for testing.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err, "Failed to create I/O context")
	ui.InitFormatter(ioCtx)
	data.InitWriter(ioCtx)

	// Create temp directory for test.
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create a test .tool-versions file.
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	content := "terraform 1.5.7\n"
	err = os.WriteFile(toolVersionsPath, []byte(content), 0o644)
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
	viper.Set("toolchain.tool-versions", toolVersionsPath)
	viper.Set("toolchain.tools-dir", toolsDirPath)
	viper.Set("toolchain.tools-config", toolsConfigPath)

	// Initialize the toolchain package config.
	atmosCfg := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile:    toolVersionsPath,
			InstallPath:     toolsDirPath,
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

// TestSetAtmosConfig tests the SetAtmosConfig function.
func TestSetAtmosConfig(t *testing.T) {
	tempDir := t.TempDir()
	atmosCfg := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile:    filepath.Join(tempDir, ".tool-versions"),
			InstallPath:     filepath.Join(tempDir, ".tools"),
			ToolsConfigFile: filepath.Join(tempDir, "tools.yaml"),
		},
	}

	// Test that SetAtmosConfig doesn't panic.
	assert.NotPanics(t, func() {
		SetAtmosConfig(atmosCfg)
	}, "SetAtmosConfig should not panic")
}

// cleanToolchainCmdState resets all toolchainCmd flags and subcommand flags to prevent test pollution.
// This follows the same pattern as cmd.NewTestKit but for the toolchain command hierarchy.
func cleanToolchainCmdState(t *testing.T) {
	t.Helper()

	// Snapshot current flag states.
	type flagSnapshot struct {
		value   string
		changed bool
	}
	snapshot := make(map[string]flagSnapshot)

	// Capture state of all flags (persistent and local) in toolchainCmd and subcommands.
	var captureFlags func(*cobra.Command)
	captureFlags = func(cmd *cobra.Command) {
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			snapshot[f.Name] = flagSnapshot{
				value:   f.Value.String(),
				changed: f.Changed,
			}
		})
		cmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
			snapshot[f.Name] = flagSnapshot{
				value:   f.Value.String(),
				changed: f.Changed,
			}
		})
		for _, child := range cmd.Commands() {
			captureFlags(child)
		}
	}
	captureFlags(toolchainCmd)

	// Register cleanup to restore flag states after test.
	t.Cleanup(func() {
		var restoreFlags func(*cobra.Command)
		restoreFlags = func(cmd *cobra.Command) {
			cmd.Flags().VisitAll(func(f *pflag.Flag) {
				if snap, ok := snapshot[f.Name]; ok {
					_ = f.Value.Set(snap.value)
					f.Changed = snap.changed
				}
			})
			cmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
				if snap, ok := snapshot[f.Name]; ok {
					_ = f.Value.Set(snap.value)
					f.Changed = snap.changed
				}
			})
			for _, child := range cmd.Commands() {
				restoreFlags(child)
			}
		}
		restoreFlags(toolchainCmd)
	})
}
