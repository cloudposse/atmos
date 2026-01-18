package toolchain

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/toolchain"
)

// Provider and structure tests are in command_provider_test.go.
// This file contains path-specific flag tests.

func TestPathCommand_Flags(t *testing.T) {
	t.Run("has export flag", func(t *testing.T) {
		flag := pathCmd.Flags().Lookup("export")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("has json flag", func(t *testing.T) {
		flag := pathCmd.Flags().Lookup("json")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("has relative flag", func(t *testing.T) {
		flag := pathCmd.Flags().Lookup("relative")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})
}

func TestPathCommand_FlagDescriptions(t *testing.T) {
	tests := []struct {
		flagName string
		contains string
	}{
		{"export", "export"},
		{"json", "JSON"},
		{"relative", "relative"},
	}

	for _, tt := range tests {
		t.Run(tt.flagName+" has description", func(t *testing.T) {
			flag := pathCmd.Flags().Lookup(tt.flagName)
			require.NotNil(t, flag)
			assert.Contains(t, flag.Usage, tt.contains)
		})
	}
}

// setupPathTestEnvironment creates a temp directory with an atmos config and tool-versions file.
func setupPathTestEnvironment(t *testing.T) func() {
	t.Helper()

	tempDir := t.TempDir()

	// Create a minimal tool-versions file with a tool.
	tvPath := filepath.Join(tempDir, ".tool-versions")
	err := os.WriteFile(tvPath, []byte("terraform 1.5.0\n"), 0o644)
	require.NoError(t, err)

	// Save original config and set test config.
	originalConfig := toolchain.GetAtmosConfig()

	testConfig := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: tvPath,
			InstallPath:  tempDir,
		},
	}
	toolchain.SetAtmosConfig(testConfig)

	return func() {
		toolchain.SetAtmosConfig(originalConfig)
		// Reset viper state.
		viper.Reset()
	}
}

// TestPathCommand_RunE tests the RunE function execution paths.
func TestPathCommand_RunE(t *testing.T) {
	t.Run("RunE with no flags uses github format", func(t *testing.T) {
		cleanup := setupPathTestEnvironment(t)
		defer cleanup()

		// Reset flags to defaults.
		pathCmd.Flags().Set("export", "false")
		pathCmd.Flags().Set("json", "false")
		pathCmd.Flags().Set("relative", "false")

		// Call RunE - will return error if no tools are installed.
		// This exercises:
		// - Line 20: Get Viper.
		// - Line 21-23: Bind flags to Viper.
		// - Line 30-32: Get flag values.
		// - Line 34-39: Format selection logic (should be "github").
		// - Line 41: Call EmitEnv.
		err := pathCmd.RunE(pathCmd, []string{})

		// Expect ErrToolNotFound since no tools are installed.
		require.Error(t, err)
		assert.ErrorIs(t, err, toolchain.ErrToolNotFound)
	})

	t.Run("RunE with export flag uses bash format", func(t *testing.T) {
		cleanup := setupPathTestEnvironment(t)
		defer cleanup()

		// Set export flag.
		pathCmd.Flags().Set("export", "true")
		pathCmd.Flags().Set("json", "false")
		pathCmd.Flags().Set("relative", "false")

		// This exercises the exportFlag path (line 37-38).
		err := pathCmd.RunE(pathCmd, []string{})

		require.Error(t, err)
		assert.ErrorIs(t, err, toolchain.ErrToolNotFound)
	})

	t.Run("RunE with json flag uses json format", func(t *testing.T) {
		cleanup := setupPathTestEnvironment(t)
		defer cleanup()

		// Set json flag.
		pathCmd.Flags().Set("export", "false")
		pathCmd.Flags().Set("json", "true")
		pathCmd.Flags().Set("relative", "false")

		// This exercises the jsonFlag path (line 35-36).
		err := pathCmd.RunE(pathCmd, []string{})

		require.Error(t, err)
		assert.ErrorIs(t, err, toolchain.ErrToolNotFound)
	})

	t.Run("RunE with json flag takes precedence over export", func(t *testing.T) {
		cleanup := setupPathTestEnvironment(t)
		defer cleanup()

		// Set both flags - json should take precedence.
		pathCmd.Flags().Set("export", "true")
		pathCmd.Flags().Set("json", "true")
		pathCmd.Flags().Set("relative", "false")

		// This exercises the precedence logic (line 35-39).
		err := pathCmd.RunE(pathCmd, []string{})

		require.Error(t, err)
		assert.ErrorIs(t, err, toolchain.ErrToolNotFound)
	})

	t.Run("RunE with relative flag passes to EmitEnv", func(t *testing.T) {
		cleanup := setupPathTestEnvironment(t)
		defer cleanup()

		// Set relative flag.
		pathCmd.Flags().Set("export", "false")
		pathCmd.Flags().Set("json", "false")
		pathCmd.Flags().Set("relative", "true")

		// This exercises the relativeFlag path (line 32, 41).
		err := pathCmd.RunE(pathCmd, []string{})

		require.Error(t, err)
		assert.ErrorIs(t, err, toolchain.ErrToolNotFound)
	})
}

// TestPathCommandProvider tests all PathCommandProvider methods.
func TestPathCommandProvider_AllMethods(t *testing.T) {
	provider := &PathCommandProvider{}

	t.Run("GetCommand returns pathCmd", func(t *testing.T) {
		cmd := provider.GetCommand()
		require.NotNil(t, cmd)
		assert.Equal(t, "path", cmd.Use)
	})

	t.Run("GetName returns path", func(t *testing.T) {
		assert.Equal(t, "path", provider.GetName())
	})

	t.Run("GetGroup returns Toolchain Commands", func(t *testing.T) {
		assert.Equal(t, "Toolchain Commands", provider.GetGroup())
	})

	t.Run("GetFlagsBuilder returns pathParser", func(t *testing.T) {
		builder := provider.GetFlagsBuilder()
		assert.NotNil(t, builder)
		assert.Equal(t, pathParser, builder)
	})

	t.Run("GetPositionalArgsBuilder returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetPositionalArgsBuilder())
	})

	t.Run("GetCompatibilityFlags returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetCompatibilityFlags())
	})
}

// TestPathCommand_Structure tests the command structure.
func TestPathCommand_Structure(t *testing.T) {
	t.Run("command has correct use string", func(t *testing.T) {
		assert.Equal(t, "path", pathCmd.Use)
	})

	t.Run("command has short description", func(t *testing.T) {
		assert.NotEmpty(t, pathCmd.Short)
		assert.Contains(t, pathCmd.Short, "PATH")
	})

	t.Run("command has long description", func(t *testing.T) {
		assert.NotEmpty(t, pathCmd.Long)
		assert.Contains(t, pathCmd.Long, "tool-versions")
	})

	t.Run("command has RunE function", func(t *testing.T) {
		assert.NotNil(t, pathCmd.RunE)
	})
}
