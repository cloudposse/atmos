package toolchain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	toolchainpkg "github.com/cloudposse/atmos/toolchain"
)

// TestToolchainCommandProvider_Extended tests additional ToolchainCommandProvider functionality.
func TestToolchainCommandProvider_Extended(t *testing.T) {
	provider := &ToolchainCommandProvider{}

	t.Run("command has PersistentPreRunE", func(t *testing.T) {
		cmd := provider.GetCommand()
		require.NotNil(t, cmd)
		assert.NotNil(t, cmd.PersistentPreRunE)
	})

	t.Run("command is the parent of subcommands", func(t *testing.T) {
		cmd := provider.GetCommand()
		require.NotNil(t, cmd)
		assert.True(t, cmd.HasSubCommands())
	})
}

// TestToolchainCommand_Structure tests the toolchain command structure.
func TestToolchainCommand_Structure(t *testing.T) {
	t.Run("command has correct use string", func(t *testing.T) {
		assert.Equal(t, "toolchain", toolchainCmd.Use)
	})

	t.Run("command has short description", func(t *testing.T) {
		assert.NotEmpty(t, toolchainCmd.Short)
		assert.Contains(t, toolchainCmd.Short, "tool")
	})

	t.Run("command has long description", func(t *testing.T) {
		assert.NotEmpty(t, toolchainCmd.Long)
	})

	t.Run("command has RunE function", func(t *testing.T) {
		assert.NotNil(t, toolchainCmd.RunE)
	})

	t.Run("command has PersistentPreRunE function", func(t *testing.T) {
		assert.NotNil(t, toolchainCmd.PersistentPreRunE)
	})
}

// TestToolchainCommand_Subcommands tests that toolchain has expected subcommands.
func TestToolchainCommand_Subcommands(t *testing.T) {
	expectedSubcommands := []string{
		"add",
		"clean",
		"du",
		"env",
		"exec",
		"get",
		"info",
		"install",
		"list",
		"path",
		"registry",
		"remove",
		"search",
		"set",
		"uninstall",
		"which",
	}

	for _, subName := range expectedSubcommands {
		t.Run("has subcommand "+subName, func(t *testing.T) {
			subCmd, _, err := toolchainCmd.Find([]string{subName})
			require.NoError(t, err, "subcommand %s should exist", subName)
			assert.Equal(t, subName, subCmd.Name())
		})
	}
}

// TestToolchainCommand_PersistentFlags tests persistent flags.
func TestToolchainCommand_PersistentFlags(t *testing.T) {
	t.Run("has github-token flag", func(t *testing.T) {
		flag := toolchainCmd.PersistentFlags().Lookup(flagGitHubToken)
		require.NotNil(t, flag)
		assert.True(t, flag.Hidden, "github-token should be hidden")
	})

	t.Run("has tool-versions flag", func(t *testing.T) {
		flag := toolchainCmd.PersistentFlags().Lookup(flagToolVersions)
		require.NotNil(t, flag)
		assert.Equal(t, ".tool-versions", flag.DefValue)
	})

	t.Run("has toolchain-path flag", func(t *testing.T) {
		flag := toolchainCmd.PersistentFlags().Lookup(flagToolchainPath)
		require.NotNil(t, flag)
		assert.Equal(t, ".tools", flag.DefValue)
	})
}

// TestToolchainCommand_Constants tests flag name constants.
func TestToolchainCommand_Constants(t *testing.T) {
	assert.Equal(t, "github-token", flagGitHubToken)
	assert.Equal(t, "tool-versions", flagToolVersions)
	assert.Equal(t, "toolchain-path", flagToolchainPath)
}

// TestSetAtmosConfig tests the SetAtmosConfig function.
func TestSetAtmosConfig(t *testing.T) {
	t.Run("sets config successfully", func(t *testing.T) {
		config := &schema.AtmosConfiguration{
			Toolchain: schema.Toolchain{
				VersionsFile: "test-.tool-versions",
				InstallPath:  "test-.tools",
			},
		}

		// Should not panic.
		assert.NotPanics(t, func() {
			SetAtmosConfig(config)
		})

		// Verify config was set.
		got := toolchainpkg.GetAtmosConfig()
		require.NotNil(t, got)
		assert.Equal(t, "test-.tool-versions", got.Toolchain.VersionsFile)
		assert.Equal(t, "test-.tools", got.Toolchain.InstallPath)
	})

	t.Run("handles nil config", func(t *testing.T) {
		// Should not panic with nil config.
		assert.NotPanics(t, func() {
			SetAtmosConfig(nil)
		})
	})
}

// TestToolchainCommand_ParserConfiguration tests that the parser is correctly configured.
func TestToolchainCommand_ParserConfiguration(t *testing.T) {
	t.Run("parser is not nil", func(t *testing.T) {
		require.NotNil(t, toolchainParser)
	})
}

// TestToolchainCommand_FlagDefaults tests default flag values.
func TestToolchainCommand_FlagDefaults(t *testing.T) {
	t.Run("github-token default is empty", func(t *testing.T) {
		flag := toolchainCmd.PersistentFlags().Lookup(flagGitHubToken)
		require.NotNil(t, flag)
		assert.Equal(t, "", flag.DefValue)
	})

	t.Run("tool-versions default is .tool-versions", func(t *testing.T) {
		flag := toolchainCmd.PersistentFlags().Lookup(flagToolVersions)
		require.NotNil(t, flag)
		assert.Equal(t, ".tool-versions", flag.DefValue)
	})

	t.Run("toolchain-path default is .tools", func(t *testing.T) {
		flag := toolchainCmd.PersistentFlags().Lookup(flagToolchainPath)
		require.NotNil(t, flag)
		assert.Equal(t, ".tools", flag.DefValue)
	})
}

// TestToolchainCommand_SubcommandCount tests the number of subcommands.
func TestToolchainCommand_SubcommandCount(t *testing.T) {
	// Count the subcommands (should have at least 15 subcommands).
	assert.GreaterOrEqual(t, len(toolchainCmd.Commands()), 15, "toolchain should have at least 15 subcommands")
}
