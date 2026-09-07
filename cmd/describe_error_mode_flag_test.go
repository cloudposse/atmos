package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// BindFlagsToViper's error branch (line ~70 of describe_error_mode_flag.go) is
// unreachable for this parser's flag config: bindFlagToViper only calls
// viper.BindEnv when env vars are present (guaranteed here) and BindEnv only
// fails on empty input; BindPFlag is pre-guarded against nil flags. Not tested
// for that reason.

func TestResolveDescribeErrorModeFlag_NoOverride(t *testing.T) {
	parser := newDescribeErrorModeParser()
	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)
	v := viper.New()

	err := resolveDescribeErrorModeFlag(cmd, v, parser)

	require.NoError(t, err)
	assert.False(t, cmd.Flags().Changed("error-mode"), "flag should be left untouched when nothing was set")
}

func TestResolveDescribeErrorModeFlag_EnvVarTakesEffect(t *testing.T) {
	parser := newDescribeErrorModeParser()
	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)
	v := viper.New()

	t.Setenv("ATMOS_DESCRIBE_ERROR_MODE", "warn")

	err := resolveDescribeErrorModeFlag(cmd, v, parser)

	require.NoError(t, err)
	val, getErr := cmd.Flags().GetString("error-mode")
	require.NoError(t, getErr)
	assert.Equal(t, "warn", val)
	assert.True(t, cmd.Flags().Changed("error-mode"), "Set must have actually run, not a coincidental default match")
}

func TestResolveDescribeErrorModeFlag_ViperKeyTakesEffect(t *testing.T) {
	parser := newDescribeErrorModeParser()
	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)
	v := viper.New()

	v.Set(describeErrorModeViperKey, "silent")

	err := resolveDescribeErrorModeFlag(cmd, v, parser)

	require.NoError(t, err)
	val, getErr := cmd.Flags().GetString("error-mode")
	require.NoError(t, getErr)
	assert.Equal(t, "silent", val)
	assert.True(t, cmd.Flags().Changed("error-mode"))
}
