package flags

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newViperKeyTestParser builds a parser whose "tags" flag lives under a namespaced Viper key and
// binds it to a fresh Viper that mimics the root command (SetEnvPrefix + AutomaticEnv).
func newViperKeyTestParser(t *testing.T) (*StandardFlagParser, *cobra.Command, *viper.Viper) {
	t.Helper()

	parser := NewStandardFlagParser(
		WithStringFlag("tags", "", "", "Filter by tags"),
		WithEnvVars("tags", "ATMOS_COMPONENT_TAGS"),
		WithViperKey("tags", "list.tags"),
	)
	cmd := &cobra.Command{Use: "test", Args: cobra.NoArgs}
	parser.RegisterFlags(cmd)

	v := viper.New()
	v.SetEnvPrefix("ATMOS")
	v.AutomaticEnv()
	require.NoError(t, parser.BindFlagsToViper(cmd, v))

	return parser, cmd, v
}

func TestWithViperKey_StoresOverride(t *testing.T) {
	cfg := &parserConfig{registry: NewFlagRegistry()}

	WithViperKey("tags", "list.tags")(cfg)

	assert.Equal(t, map[string]string{"tags": "list.tags"}, cfg.viperKeys)
}

func TestStandardFlagParser_GetViperKey_Override(t *testing.T) {
	t.Run("override beats prefix", func(t *testing.T) {
		parser := NewStandardFlagParser(
			WithStringFlag("tags", "", "", "Tags"),
			WithStringFlag("stack", "", "", "Stack"),
			WithViperPrefix("pfx"),
			WithViperKey("tags", "list.tags"),
		)
		assert.Equal(t, "list.tags", parser.getViperKey("tags"))
		assert.Equal(t, "pfx.stack", parser.getViperKey("stack"))
	})

	t.Run("no override keeps the flag name", func(t *testing.T) {
		parser := NewStandardFlagParser(WithStringFlag("tags", "", "", "Tags"))
		assert.Equal(t, "tags", parser.getViperKey("tags"))
	})
}

// TestStandardFlagParser_ViperKey_IsolatesFromAutomaticEnv proves a namespaced Viper key is not
// reachable through the global ATMOS_<KEY> automatic env lookup, while the explicitly bound
// command-specific env var is honoured.
func TestStandardFlagParser_ViperKey_IsolatesFromAutomaticEnv(t *testing.T) {
	t.Run("explicit env var wins over leaked ATMOS_TAGS", func(t *testing.T) {
		t.Setenv("ATMOS_TAGS", "leak")
		t.Setenv("ATMOS_COMPONENT_TAGS", "good")

		_, _, v := newViperKeyTestParser(t)

		assert.Equal(t, "good", v.GetString("list.tags"))
	})

	t.Run("ATMOS_TAGS alone does not leak", func(t *testing.T) {
		t.Setenv("ATMOS_TAGS", "leak")

		_, _, v := newViperKeyTestParser(t)

		assert.Empty(t, v.GetString("list.tags"))
	})

	t.Run("bare key still leaks without the override (documents the root cause)", func(t *testing.T) {
		t.Setenv("ATMOS_TAGS", "leak")

		parser := NewStandardFlagParser(
			WithStringFlag("tags", "", "", "Filter by tags"),
			WithEnvVars("tags", "ATMOS_COMPONENT_TAGS"),
		)
		cmd := &cobra.Command{Use: "test", Args: cobra.NoArgs}
		parser.RegisterFlags(cmd)
		v := viper.New()
		v.SetEnvPrefix("ATMOS")
		v.AutomaticEnv()
		require.NoError(t, parser.BindFlagsToViper(cmd, v))

		assert.Equal(t, "leak", v.GetString("tags"))
	})

	t.Run("CLI flag wins over both env vars", func(t *testing.T) {
		t.Setenv("ATMOS_TAGS", "leak")
		t.Setenv("ATMOS_COMPONENT_TAGS", "good")

		_, cmd, v := newViperKeyTestParser(t)
		require.NoError(t, cmd.Flags().Set("tags", "cli"))

		assert.Equal(t, "cli", v.GetString("list.tags"))
	})
}

func TestStandardFlagParser_ViperKey_ParseKeepsFlagName(t *testing.T) {
	t.Setenv("ATMOS_TAGS", "leak")
	t.Setenv("ATMOS_COMPONENT_TAGS", "good")

	parser, cmd, _ := newViperKeyTestParser(t)

	result, err := parser.Parse(context.Background(), []string{})
	require.NoError(t, err)
	assert.Equal(t, "good", GetString(result.Flags, "tags"))
	assert.NotContains(t, result.Flags, "list.tags")

	require.NoError(t, cmd.Flags().Set("tags", "cli"))
	result, err = parser.Parse(context.Background(), []string{})
	require.NoError(t, err)
	assert.Equal(t, "cli", GetString(result.Flags, "tags"))
}
