package flags

import (
	"testing"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveExplicitStringFlag_DisableFlagParsingBareNoSpaceFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "test", DisableFlagParsing: true}
	parser := NewGlobalOptionsBuilder().Build()
	parser.RegisterPersistentFlags(cmd)

	result, err := ResolveExplicitStringFlag(cmd, []string{"--cast", "terraform", "plan"}, cfg.CastFlagName)
	require.NoError(t, err)

	assert.True(t, result.Changed)
	assert.Equal(t, cfg.CastFlagAutoValue, result.Value)
	assert.Equal(t, []string{"terraform", "plan"}, result.Args)
}

func TestResolveExplicitStringFlag_DisableFlagParsingInlineValue(t *testing.T) {
	cmd := &cobra.Command{Use: "test", DisableFlagParsing: true}
	parser := NewGlobalOptionsBuilder().Build()
	parser.RegisterPersistentFlags(cmd)

	result, err := ResolveExplicitStringFlag(cmd, []string{"--cast=demo.cast", "terraform", "plan"}, cfg.CastFlagName)
	require.NoError(t, err)

	assert.True(t, result.Changed)
	assert.Equal(t, "demo.cast", result.Value)
	assert.Equal(t, []string{"terraform", "plan"}, result.Args)
}

func TestResolveExplicitStringFlag_DisableFlagParsingConsumesSpaceValue(t *testing.T) {
	cmd := &cobra.Command{Use: "test", DisableFlagParsing: true}
	parser := NewGlobalOptionsBuilder().Build()
	parser.RegisterPersistentFlags(cmd)

	result, err := ResolveExplicitStringFlag(cmd, []string{"--identity", "prod", "login"}, cfg.IdentityFlagName)
	require.NoError(t, err)

	assert.True(t, result.Changed)
	assert.Equal(t, "prod", result.Value)
	assert.Equal(t, []string{"login"}, result.Args)
}

func TestResolveExplicitStringFlag_StopsAtSeparator(t *testing.T) {
	cmd := &cobra.Command{Use: "test", DisableFlagParsing: true}
	parser := NewGlobalOptionsBuilder().Build()
	parser.RegisterPersistentFlags(cmd)

	result, err := ResolveExplicitStringFlag(cmd, []string{"terraform", "plan", "--", "--cast=downstream.cast"}, cfg.CastFlagName)
	require.NoError(t, err)

	assert.False(t, result.Changed)
	assert.Equal(t, []string{"terraform", "plan", "--", "--cast=downstream.cast"}, result.Args)
}

func TestIsHelpRequested(t *testing.T) {
	t.Run("nil command returns false", func(t *testing.T) {
		assert.False(t, IsHelpRequested(nil, nil))
	})

	t.Run("command named help returns true", func(t *testing.T) {
		cmd := &cobra.Command{Use: "help"}
		assert.True(t, IsHelpRequested(cmd, nil))
	})

	t.Run("help flag changed returns true", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().Bool("help", false, "")
		require.NoError(t, cmd.Flags().Set("help", "true"))
		assert.True(t, IsHelpRequested(cmd, nil))
	})

	t.Run("help arg returns true", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		assert.True(t, IsHelpRequested(cmd, []string{"help"}))
	})

	t.Run("--help arg returns true", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		assert.True(t, IsHelpRequested(cmd, []string{"terraform", "--help"}))
	})

	t.Run("-h arg returns true", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		assert.True(t, IsHelpRequested(cmd, []string{"terraform", "-h"}))
	})

	t.Run("separator before help stops scan and returns false", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		assert.False(t, IsHelpRequested(cmd, []string{"--", "help"}))
	})

	t.Run("no help requested returns false", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		assert.False(t, IsHelpRequested(cmd, []string{"terraform", "plan"}))
	})
}

func TestStripStringFlagArgs(t *testing.T) {
	cmd := &cobra.Command{Use: "test", DisableFlagParsing: true}
	parser := NewGlobalOptionsBuilder().Build()
	parser.RegisterPersistentFlags(cmd)

	result := stripStringFlagArgs(cmd, []string{"--identity", "prod", "login"}, cfg.IdentityFlagName)
	assert.Equal(t, []string{"login"}, result)
}

func TestStringFlagNoOptDefVal_FallsBackToGlobalRegistry(t *testing.T) {
	// The identity flag isn't registered on a bare command, so the lookup
	// should fall back to the GlobalFlagsRegistry to find its NoOptDefVal.
	cmd := &cobra.Command{Use: "test"}
	assert.Equal(t, cfg.IdentityFlagSelectValue, stringFlagNoOptDefVal(cmd, cfg.IdentityFlagName))
}

func TestStringFlagConsumesNextArg_FallsBackToGlobalRegistry(t *testing.T) {
	// cast flag has NoOptDefValNoSpaceValue=true, meaning it does NOT consume the next arg.
	assert.False(t, stringFlagConsumesNextArg(cfg.CastFlagName))
	// Unknown flags default to true (consumes next arg).
	assert.True(t, stringFlagConsumesNextArg("totally-unknown-flag"))
}
