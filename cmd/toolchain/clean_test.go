package toolchain

import (
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

// Provider and structure tests are in command_provider_test.go.
// Business-logic coverage (dry-run/cache-only/confirmation behavior) lives in
// pkg/toolchain/clean_test.go, which tests RunClean directly. This file only covers the
// cmd-layer flag wiring.

func TestCleanCommand_Flags(t *testing.T) {
	t.Run("has dry-run flag", func(t *testing.T) {
		flag := cleanCmd.Flags().Lookup("dry-run")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("has cache-only flag", func(t *testing.T) {
		flag := cleanCmd.Flags().Lookup("cache-only")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("has force flag", func(t *testing.T) {
		flag := cleanCmd.Flags().Lookup("force")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})
}

func TestCleanCommand_FlagDescriptions(t *testing.T) {
	tests := []struct {
		flagName string
		contains string
	}{
		{"dry-run", "without actually removing"},
		{"cache-only", "download cache"},
		{"force", "confirmation prompt"},
	}

	for _, tt := range tests {
		t.Run(tt.flagName+" has description", func(t *testing.T) {
			flag := cleanCmd.Flags().Lookup(tt.flagName)
			require.NotNil(t, flag)
			assert.Contains(t, flag.Usage, tt.contains)
		})
	}
}

// TestCleanCommand_RunE_DryRun exercises the real cleanCmd.RunE with --dry-run so it reaches
// toolchain.RunClean via the actual flag-binding path (not a re-implementation of it). --dry-run
// never prompts, so this is safe to run without a TTY. InstallPath and ATMOS_XDG_CACHE_HOME are
// pinned to a temp dir so the (read-only) preview never touches this machine's real toolchain
// install or cache directories.
func TestCleanCommand_RunE_DryRun(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", filepath.Join(tempDir, "xdg-cache"))
	toolchain.SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{InstallPath: filepath.Join(tempDir, ".tools")},
	})

	t.Cleanup(func() {
		toolchain.SetAtmosConfig(nil)
		require.NoError(t, cleanCmd.Flags().Set("dry-run", "false"))
		require.NoError(t, cleanCmd.Flags().Set("cache-only", "false"))
		require.NoError(t, cleanCmd.Flags().Set("force", "false"))
		viper.Reset()
	})

	require.NoError(t, cleanCmd.Flags().Set("dry-run", "true"))

	err := cleanCmd.RunE(cleanCmd, []string{})
	require.NoError(t, err)
}

func TestCleanCommand_Args(t *testing.T) {
	t.Run("accepts zero arguments", func(t *testing.T) {
		err := cleanCmd.Args(cleanCmd, []string{})
		assert.NoError(t, err)
	})

	t.Run("rejects one argument", func(t *testing.T) {
		err := cleanCmd.Args(cleanCmd, []string{"unexpected"})
		assert.Error(t, err)
	})
}
