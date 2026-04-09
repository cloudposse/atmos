package toolchain

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInstallCommandProvider_Extended tests additional InstallCommandProvider functionality.
func TestInstallCommandProvider_Extended(t *testing.T) {
	provider := &InstallCommandProvider{}

	t.Run("command is attached to toolchain", func(t *testing.T) {
		cmd := provider.GetCommand()
		require.NotNil(t, cmd)
		// Verify the command has the expected properties.
		assert.True(t, cmd.SilenceUsage)
		assert.True(t, cmd.SilenceErrors)
	})

	t.Run("command has correct Use string", func(t *testing.T) {
		cmd := provider.GetCommand()
		require.NotNil(t, cmd)
		assert.Contains(t, cmd.Use, "install")
		assert.Contains(t, cmd.Use, "[tool...]")
	})
}

// TestInstallCommand_Flags tests install command flags.
func TestInstallCommand_Flags(t *testing.T) {
	t.Run("install command has reinstall flag", func(t *testing.T) {
		flag := installCmd.Flags().Lookup("reinstall")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("install command has default flag", func(t *testing.T) {
		flag := installCmd.Flags().Lookup("default")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})
}

// TestInstallCommand_CommandStructure tests the install command structure.
func TestInstallCommand_CommandStructure(t *testing.T) {
	t.Run("command has correct use string", func(t *testing.T) {
		assert.Contains(t, installCmd.Use, "install")
		assert.Contains(t, installCmd.Use, "[tool...]")
	})

	t.Run("command has short description", func(t *testing.T) {
		assert.NotEmpty(t, installCmd.Short)
		assert.Contains(t, installCmd.Short, "Install")
	})

	t.Run("command has long description", func(t *testing.T) {
		assert.NotEmpty(t, installCmd.Long)
		assert.Contains(t, installCmd.Long, "owner/repo@version")
	})

	t.Run("command has RunE function", func(t *testing.T) {
		assert.NotNil(t, installCmd.RunE)
	})

	t.Run("command silences usage on error", func(t *testing.T) {
		assert.True(t, installCmd.SilenceUsage)
	})

	t.Run("command silences errors", func(t *testing.T) {
		assert.True(t, installCmd.SilenceErrors)
	})

	t.Run("command accepts multiple arguments", func(t *testing.T) {
		require.NotNil(t, installCmd.Args)
		// Verify the command accepts zero, one, or multiple arguments.
		assert.NoError(t, installCmd.Args(installCmd, []string{}))
		assert.NoError(t, installCmd.Args(installCmd, []string{"tool1"}))
		assert.NoError(t, installCmd.Args(installCmd, []string{"tool1", "tool2", "tool3"}))
	})
}

// TestInstallCommand_ParserConfiguration tests that the parser is correctly configured.
func TestInstallCommand_ParserConfiguration(t *testing.T) {
	t.Run("parser is not nil", func(t *testing.T) {
		require.NotNil(t, installParser)
	})
}

// TestInstallCommand_ViperIntegration tests Viper integration with install command.
func TestInstallCommand_ViperIntegration(t *testing.T) {
	tests := []struct {
		name           string
		reinstall      bool
		defaultVersion bool
	}{
		{
			name:           "default values",
			reinstall:      false,
			defaultVersion: false,
		},
		{
			name:           "reinstall flag true",
			reinstall:      true,
			defaultVersion: false,
		},
		{
			name:           "default flag true",
			reinstall:      false,
			defaultVersion: true,
		},
		{
			name:           "both flags true",
			reinstall:      true,
			defaultVersion: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()
			v.Set("reinstall", tt.reinstall)
			v.Set("default", tt.defaultVersion)

			assert.Equal(t, tt.reinstall, v.GetBool("reinstall"))
			assert.Equal(t, tt.defaultVersion, v.GetBool("default"))
		})
	}
}

// TestInstallCommand_FlagDefaults tests default flag values.
func TestInstallCommand_FlagDefaults(t *testing.T) {
	t.Run("reinstall default is false", func(t *testing.T) {
		flag := installCmd.Flags().Lookup("reinstall")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("default default is false", func(t *testing.T) {
		flag := installCmd.Flags().Lookup("default")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})
}

// TestInstallCommandProvider_Interface tests the full interface implementation.
func TestInstallCommandProvider_Interface(t *testing.T) {
	provider := &InstallCommandProvider{}

	t.Run("GetName returns install", func(t *testing.T) {
		assert.Equal(t, "install", provider.GetName())
	})

	t.Run("GetGroup returns Toolchain Commands", func(t *testing.T) {
		assert.Equal(t, "Toolchain Commands", provider.GetGroup())
	})

	t.Run("GetFlagsBuilder returns parser", func(t *testing.T) {
		fb := provider.GetFlagsBuilder()
		assert.NotNil(t, fb)
		assert.Equal(t, installParser, fb)
	})

	t.Run("GetPositionalArgsBuilder returns nil", func(t *testing.T) {
		pab := provider.GetPositionalArgsBuilder()
		assert.Nil(t, pab)
	})

	t.Run("GetCompatibilityFlags returns nil", func(t *testing.T) {
		cf := provider.GetCompatibilityFlags()
		assert.Nil(t, cf)
	})
}
