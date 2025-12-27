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

	"github.com/cloudposse/atmos/pkg/config/homedir"
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

// resetViperWithToolchainBindings resets Viper to a clean state for test isolation.
// Flag bindings persist through the registered command, so re-binding is unnecessary.
func resetViperWithToolchainBindings(t *testing.T) {
	t.Helper()

	// Reset Viper to clean state (clears all keys and settings).
	viper.Reset()
}

// setupToolchainTest creates a test environment with tool-versions and tools config files.
func setupToolchainTest(t *testing.T, toolVersionsContent string) string {
	t.Helper()

	// Initialize test kit to auto-clean RootCmd state.
	newTestKit(t)

	// Clean toolchainCmd state before each test to prevent pollution.
	cleanToolchainCmdState(t)

	// Reset Viper and re-bind flags for test isolation.
	resetViperWithToolchainBindings(t)

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
	// Cleanup handled automatically by cleanRootCmdState.
	toolsDir := filepath.Join(tempDir, ".tools")

	viper.Set("toolchain.tool-versions", toolVersionsPath)
	viper.Set("toolchain.path", toolsDir)
	viper.Set("toolchain.tools-config", toolsConfigPath)

	// Initialize the toolchain package config.
	// Save previous config for cleanup.
	prevAtmosConfig := toolchainpkg.GetAtmosConfig()

	atmosCfg := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: toolVersionsPath,
			InstallPath:  toolsDir,
			FilePath:     toolsConfigPath,
		},
	}
	toolchainpkg.SetAtmosConfig(atmosCfg)

	// Restore the original config after the test.
	t.Cleanup(func() {
		toolchainpkg.SetAtmosConfig(prevAtmosConfig)
	})

	return toolVersionsPath
}

func TestToolchainAddCommand(t *testing.T) {
	toolVersionsPath := setupToolchainTest(t, "")

	// Test the add command runs without error.
	err := addCmd.RunE(addCmd, []string{"terraform@1.5.7"})
	assert.NoError(t, err)

	// Verify the tool was added.
	fileContent, err := os.ReadFile(toolVersionsPath)
	require.NoError(t, err)
	assert.Contains(t, string(fileContent), "1.5.7")
}

func TestToolchainRemoveCommand(t *testing.T) {
	toolVersionsPath := setupToolchainTest(t, "terraform 1.5.7\n")

	// Test the remove command runs without error.
	err := removeCmd.RunE(removeCmd, []string{"terraform"})
	assert.NoError(t, err)

	// Verify the tool was removed.
	fileContent, err := os.ReadFile(toolVersionsPath)
	require.NoError(t, err)
	assert.NotContains(t, string(fileContent), "terraform")
}

func TestToolchainCommandsWithoutToolVersionsFile(t *testing.T) {
	// Initialize test kit for automatic state cleanup.
	newTestKit(t)
	cleanToolchainCmdState(t)

	// Reset Viper for test isolation.
	viper.Reset()
	t.Cleanup(func() { viper.Reset() })

	// Initialize I/O context and formatter for testing.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err, "Failed to create I/O context")
	ui.InitFormatter(ioCtx)
	data.InitWriter(ioCtx)

	// Create temp directory for test.
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Set paths to non-existent files via Viper.
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	toolsDir := filepath.Join(tempDir, ".tools")
	toolsConfigPath := filepath.Join(tempDir, "tools.yaml")

	viper.Set("toolchain.tool-versions", toolVersionsPath)
	viper.Set("toolchain.path", toolsDir)
	viper.Set("toolchain.tools-config", toolsConfigPath)

	// Initialize the toolchain package config to ensure GetAtmosConfig() returns valid state.
	// Save previous config for cleanup.
	prevAtmosConfig := toolchainpkg.GetAtmosConfig()

	atmosCfg := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: toolVersionsPath,
			InstallPath:  toolsDir,
			FilePath:     toolsConfigPath,
		},
	}
	toolchainpkg.SetAtmosConfig(atmosCfg)

	// Restore the original config after the test.
	t.Cleanup(func() {
		toolchainpkg.SetAtmosConfig(prevAtmosConfig)
	})

	// Test that list command handles missing file gracefully.
	err = listCmd.RunE(listCmd, []string{})
	// Command should handle missing tool-versions file gracefully without error.
	assert.NoError(t, err, "list command should handle missing .tool-versions gracefully")
}

func TestToolchainCleanCommand(t *testing.T) {
	// Initialize test kit for automatic state cleanup.
	newTestKit(t)
	cleanToolchainCmdState(t)

	// Reset Viper and re-bind flags for test isolation.
	resetViperWithToolchainBindings(t)

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
	viper.Set("toolchain.path", toolsDirPath)
	viper.Set("toolchain.tools-config", toolsConfigPath)

	// Initialize the toolchain package config.
	// Save previous config for cleanup.
	prevAtmosConfig := toolchainpkg.GetAtmosConfig()

	atmosCfg := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: toolVersionsPath,
			InstallPath:  toolsDirPath,
			FilePath:     toolsConfigPath,
		},
	}
	toolchainpkg.SetAtmosConfig(atmosCfg)

	// Restore the original config after the test.
	t.Cleanup(func() {
		toolchainpkg.SetAtmosConfig(prevAtmosConfig)
	})

	// Test the clean command runs without error.
	err = cleanCmd.RunE(cleanCmd, []string{})
	assert.NoError(t, err)
}

func TestToolchainPathCommand(t *testing.T) {
	// Initialize test kit for automatic state cleanup.
	newTestKit(t)
	cleanToolchainCmdState(t)

	// Reset Viper and re-bind flags for test isolation.
	resetViperWithToolchainBindings(t)

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
	viper.Set("toolchain.path", toolsDirPath)
	viper.Set("toolchain.tools-config", toolsConfigPath)

	// Initialize the toolchain package config.
	// Save previous config for cleanup.
	prevAtmosConfig := toolchainpkg.GetAtmosConfig()

	atmosCfg := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: toolVersionsPath,
			InstallPath:  toolsDirPath,
			FilePath:     toolsConfigPath,
		},
	}
	toolchainpkg.SetAtmosConfig(atmosCfg)

	// Restore the original config after the test.
	t.Cleanup(func() {
		toolchainpkg.SetAtmosConfig(prevAtmosConfig)
	})

	// Test the path command runs without error.
	err = pathCmd.RunE(pathCmd, []string{})
	assert.NoError(t, err)
}

func TestToolchainWhichCommand(t *testing.T) {
	// Initialize test kit for automatic state cleanup.
	newTestKit(t)
	cleanToolchainCmdState(t)

	// Reset Viper and re-bind flags for test isolation.
	resetViperWithToolchainBindings(t)

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
	viper.Set("toolchain.path", toolsDirPath)
	viper.Set("toolchain.tools-config", toolsConfigPath)

	// Initialize the toolchain package config.
	// Save previous config for cleanup.
	prevAtmosConfig := toolchainpkg.GetAtmosConfig()

	atmosCfg := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: toolVersionsPath,
			InstallPath:  toolsDirPath,
			FilePath:     toolsConfigPath,
		},
	}
	toolchainpkg.SetAtmosConfig(atmosCfg)

	// Restore the original config after the test.
	t.Cleanup(func() {
		toolchainpkg.SetAtmosConfig(prevAtmosConfig)
	})

	// Test the which command runs without error.
	err = whichCmd.RunE(whichCmd, []string{"terraform"})
	assert.NoError(t, err)
}

func TestToolchainSetCommand(t *testing.T) {
	// Initialize test kit for automatic state cleanup.
	newTestKit(t)
	cleanToolchainCmdState(t)

	// Reset Viper and re-bind flags for test isolation.
	resetViperWithToolchainBindings(t)

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
	viper.Set("toolchain.path", toolsDirPath)
	viper.Set("toolchain.tools-config", toolsConfigPath)

	// Initialize the toolchain package config.
	// Save previous config for cleanup.
	prevAtmosConfig := toolchainpkg.GetAtmosConfig()

	atmosCfg := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: toolVersionsPath,
			InstallPath:  toolsDirPath,
			FilePath:     toolsConfigPath,
		},
	}
	toolchainpkg.SetAtmosConfig(atmosCfg)

	// Restore the original config after the test.
	t.Cleanup(func() {
		toolchainpkg.SetAtmosConfig(prevAtmosConfig)
	})

	// Test the set command runs without error.
	err = setCmd.RunE(setCmd, []string{"terraform", "1.5.7"})
	assert.NoError(t, err)

	// Verify the version was set as default (first).
	fileContent, err := os.ReadFile(toolVersionsPath)
	require.NoError(t, err)
	assert.Contains(t, string(fileContent), "1.5.7")
}

func TestToolchainUninstallCommand(t *testing.T) {
	// Initialize test kit for automatic state cleanup.
	newTestKit(t)
	cleanToolchainCmdState(t)

	// Reset Viper and re-bind flags for test isolation.
	resetViperWithToolchainBindings(t)

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
	viper.Set("toolchain.path", toolsDirPath)
	viper.Set("toolchain.tools-config", toolsConfigPath)

	// Initialize the toolchain package config.
	// Save previous config for cleanup.
	prevAtmosConfig := toolchainpkg.GetAtmosConfig()

	atmosCfg := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: toolVersionsPath,
			InstallPath:  toolsDirPath,
			FilePath:     toolsConfigPath,
		},
	}
	toolchainpkg.SetAtmosConfig(atmosCfg)

	// Restore the original config after the test.
	t.Cleanup(func() {
		toolchainpkg.SetAtmosConfig(prevAtmosConfig)
	})

	// Test the uninstall command runs without error.
	err = uninstallCmd.RunE(uninstallCmd, []string{"terraform@1.5.7"})
	assert.NoError(t, err)

	// Verify the binary was removed.
	_, err = os.Stat(binaryPath)
	assert.True(t, os.IsNotExist(err), "Binary should be removed")
}

// TestSetAtmosConfig tests the SetAtmosConfig function.
func TestSetAtmosConfig(t *testing.T) {
	// Capture current config for restoration.
	prevAtmosConfig := toolchainpkg.GetAtmosConfig()
	t.Cleanup(func() {
		toolchainpkg.SetAtmosConfig(prevAtmosConfig)
	})

	tempDir := t.TempDir()
	atmosCfg := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: filepath.Join(tempDir, ".tool-versions"),
			InstallPath:  filepath.Join(tempDir, ".tools"),
			FilePath:     filepath.Join(tempDir, "tools.yaml"),
		},
	}

	// Test that SetAtmosConfig doesn't panic.
	assert.NotPanics(t, func() {
		SetAtmosConfig(atmosCfg)
	}, "SetAtmosConfig should not panic")
}

// TestToolchainPersistentPreRunPreservesConfig verifies that PersistentPreRunE
// preserves important config fields like UseToolVersions, UseLockFile, and Registries
// when applying flag/env overrides (regression test for CodeRabbit issue).
func TestToolchainPersistentPreRunPreservesConfig(t *testing.T) {
	// Initialize test kit to auto-clean RootCmd state.
	newTestKit(t)

	// Clean toolchainCmd state before test.
	cleanToolchainCmdState(t)

	// Initialize I/O context and formatter for testing.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err, "Failed to create I/O context")
	ui.InitFormatter(ioCtx)
	data.InitWriter(ioCtx)

	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create a full config with UseToolVersions and other important fields set.
	fullConfig := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile:    filepath.Join(tempDir, ".tool-versions"),
			InstallPath:     filepath.Join(tempDir, ".tools"),
			ToolsDir:        filepath.Join(tempDir, ".tools"),
			FilePath:        filepath.Join(tempDir, "tools.yaml"),
			UseToolVersions: true, // Important field that should be preserved.
			UseLockFile:     true, // Important field that should be preserved.
			Registries: []schema.ToolchainRegistry{
				{
					Name:     "test-registry",
					Type:     "aqua",
					Source:   "https://example.com/registry.yaml",
					Priority: 100,
				},
			},
		},
	}

	// Set the config in the toolchain package (simulating root.go Execute()).
	prevAtmosConfig := toolchainpkg.GetAtmosConfig()
	toolchainpkg.SetAtmosConfig(fullConfig)
	t.Cleanup(func() {
		toolchainpkg.SetAtmosConfig(prevAtmosConfig)
		viper.Reset()
	})

	// Set up viper with flag values.
	v := viper.GetViper()
	v.Set("toolchain.tool-versions", filepath.Join(tempDir, ".tool-versions"))
	v.Set("toolchain.path", filepath.Join(tempDir, ".tools"))
	v.Set("toolchain.tools-config", filepath.Join(tempDir, "tools.yaml"))

	// Run PersistentPreRunE.
	err = toolchainCmd.PersistentPreRunE(toolchainCmd, []string{})
	require.NoError(t, err, "PersistentPreRunE should not return error")

	// Verify the config after PersistentPreRunE.
	configAfter := toolchainpkg.GetAtmosConfig()
	require.NotNil(t, configAfter, "Config should not be nil after PersistentPreRunE")

	// Verify that important fields are preserved.
	assert.True(t, configAfter.Toolchain.UseToolVersions, "UseToolVersions should be preserved")
	assert.True(t, configAfter.Toolchain.UseLockFile, "UseLockFile should be preserved")
	assert.Len(t, configAfter.Toolchain.Registries, 1, "Registries should be preserved")
	assert.Equal(t, "test-registry", configAfter.Toolchain.Registries[0].Name, "Registry details should be preserved")

	// Verify that the path fields were updated (these can be overridden by flags/env).
	assert.Equal(t, filepath.Join(tempDir, ".tool-versions"), configAfter.Toolchain.VersionsFile)
	assert.Equal(t, filepath.Join(tempDir, ".tools"), configAfter.Toolchain.InstallPath)
	assert.Equal(t, filepath.Join(tempDir, ".tools"), configAfter.Toolchain.ToolsDir)
	assert.Equal(t, filepath.Join(tempDir, "tools.yaml"), configAfter.Toolchain.FilePath)
}

// newTestKit sets up test environment with automatic RootCmd state cleanup.
// This follows the same pattern as cmd.NewTestKit for the toolchain package.
func newTestKit(t *testing.T) {
	t.Helper()

	// Clean RootCmd state and register cleanup for restoration.
	cleanRootCmdState(t)
}

// cleanToolchainCmdState resets all toolchainCmd flags and subcommand flags to prevent test pollution.
// This follows the same pattern as cmd.NewTestKit but for the toolchain command hierarchy.
// CleanRootCmdState cleans RootCmd and Viper state by accessing it via toolchainCmd.Parent().
func cleanRootCmdState(t *testing.T) {
	t.Helper()

	rootCmd := toolchainCmd.Parent()
	if rootCmd == nil {
		return // Not attached to parent, nothing to clean.
	}

	// Snapshot current flag states.
	type flagSnapshot struct {
		value   string
		changed bool
	}
	snapshot := make(map[string]flagSnapshot)

	// Capture state of all flags in RootCmd.
	rootCmd.Flags().VisitAll(func(f *pflag.Flag) {
		snapshot[f.Name] = flagSnapshot{
			value:   f.Value.String(),
			changed: f.Changed,
		}
	})
	rootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		snapshot[f.Name] = flagSnapshot{
			value:   f.Value.String(),
			changed: f.Changed,
		}
	})

	// Snapshot os std streams.
	osStdout := os.Stdout
	osStderr := os.Stderr
	osStdin := os.Stdin

	// Reset args on RootCmd.
	rootCmd.SetArgs([]string{})

	// Register cleanup to restore flag, Viper, and I/O states after test.
	t.Cleanup(func() {
		// Reset global I/O and UI state BEFORE restoring os std streams.
		// This ensures cached I/O contexts are cleared while tests may still have
		// modified stdout/stderr, preventing the next test from inheriting stale stream references.
		iolib.Reset()
		data.Reset()
		ui.Reset()
		homedir.Reset()

		// Restore os std streams.
		os.Stdout = osStdout
		os.Stderr = osStderr
		os.Stdin = osStdin

		// Restore flags.
		rootCmd.Flags().VisitAll(func(f *pflag.Flag) {
			if snap, ok := snapshot[f.Name]; ok {
				_ = f.Value.Set(snap.value)
				f.Changed = snap.changed
			}
		})
		rootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
			if snap, ok := snapshot[f.Name]; ok {
				_ = f.Value.Set(snap.value)
				f.Changed = snap.changed
			}
		})
	})
}

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
