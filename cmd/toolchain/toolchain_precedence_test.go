package toolchain

import (
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	toolchainpkg "github.com/cloudposse/atmos/pkg/toolchain"
)

// Compile-time sentinel for the schema fields these tests configure.
var _ = schema.Toolchain{FilePath: "x", VersionsFile: "y"}

// runToolchainPreRun executes the toolchain command's PersistentPreRunE against a configuration
// that already sets a project-level toolchain.file_path, restoring all global state afterwards.
// The args are parsed as command-line flags before the hook runs.
func runToolchainPreRun(t *testing.T, configured *schema.AtmosConfiguration, args ...string) {
	t.Helper()

	previous := toolchainpkg.GetAtmosConfig()
	toolchainpkg.SetAtmosConfig(configured)
	t.Cleanup(func() { toolchainpkg.SetAtmosConfig(previous) })

	// Reset the (global) command's flag state so a flag set here cannot leak into other tests.
	t.Cleanup(func() {
		toolchainCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		})
	})

	// init() binds the flag to this Viper key on the global Viper; other tests in the package may
	// have reset the global Viper since, so restore the production wiring before the hook reads it.
	require.NoError(t, viper.GetViper().BindPFlag("toolchain.tool-versions", toolchainCmd.PersistentFlags().Lookup(flagToolVersions)))

	require.NoError(t, toolchainCmd.ParseFlags(args))
	require.NoError(t, toolchainCmd.PersistentPreRunE(toolchainCmd, nil))
}

// TestToolchainPreRun_ToolVersionsOverrideBeatsConfiguredFilePath guards the precedence contract:
// the toolchain package resolves toolchain.file_path ahead of toolchain.versions_file, so an
// explicit --tool-versions flag must set BOTH names or a project's
// atmos.yaml file_path would silently win over the user's explicit override.
func TestToolchainPreRun_ToolVersionsOverrideBeatsConfiguredFilePath(t *testing.T) {
	dir := t.TempDir()
	configuredPath := filepath.Join(dir, "configured", ".tool-versions")
	overridePath := filepath.Join(dir, "override", ".tool-versions")

	newConfig := func() *schema.AtmosConfiguration {
		return &schema.AtmosConfiguration{
			Toolchain: schema.Toolchain{FilePath: configuredPath},
		}
	}

	t.Run("flag override wins over configured file_path", func(t *testing.T) {
		config := newConfig()
		runToolchainPreRun(t, config, "--"+flagToolVersions+"="+overridePath)

		assert.Equal(t, overridePath, config.Toolchain.FilePath)
		assert.Equal(t, overridePath, config.Toolchain.VersionsFile)
		assert.Equal(t, overridePath, toolchainpkg.GetToolVersionsFilePath())
	})

	t.Run("ATMOS_TOOL_VERSIONS override wins over configured file_path", func(t *testing.T) {
		t.Setenv("ATMOS_TOOL_VERSIONS", overridePath)
		config := newConfig()
		runToolchainPreRun(t, config)

		assert.Equal(t, overridePath, config.Toolchain.FilePath)
		assert.Equal(t, overridePath, config.Toolchain.VersionsFile)
		assert.Equal(t, overridePath, toolchainpkg.GetToolVersionsFilePath())
	})

	t.Run("without an override the configured file_path is preserved", func(t *testing.T) {
		config := newConfig()
		runToolchainPreRun(t, config)

		assert.Equal(t, configuredPath, config.Toolchain.FilePath)
		assert.Empty(t, config.Toolchain.VersionsFile, "the flag default must not be applied as an override")
		assert.Equal(t, configuredPath, toolchainpkg.GetToolVersionsFilePath())
	})
}

// TestToolchainPreRun_ToolchainPathEnvOverride verifies that ATMOS_TOOLCHAIN_PATH alone (no flag)
// replaces the configured install path with the environment value, not the flag default.
func TestToolchainPreRun_ToolchainPathEnvOverride(t *testing.T) {
	override := filepath.Join(t.TempDir(), "env-tools")
	t.Setenv("ATMOS_TOOLCHAIN_PATH", override)
	require.NoError(t, viper.GetViper().BindPFlag("toolchain.path", toolchainCmd.PersistentFlags().Lookup(flagToolchainPath)))

	config := &schema.AtmosConfiguration{Toolchain: schema.Toolchain{InstallPath: filepath.Join(t.TempDir(), "configured")}}
	runToolchainPreRun(t, config)

	assert.Equal(t, override, config.Toolchain.InstallPath)
	assert.Equal(t, override, config.Toolchain.ToolsDir)
}
