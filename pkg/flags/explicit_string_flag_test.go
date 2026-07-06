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
