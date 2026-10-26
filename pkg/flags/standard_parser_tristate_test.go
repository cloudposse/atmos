package flags_test

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/flags"
)

// newTriStateParser creates a StandardParser with a single bool flag for tri-state testing.
// The flag has one env var binding, mirroring the --verify-plan / ATMOS_TERRAFORM_VERIFY_PLAN setup.
func newTriStateParser(t *testing.T) (*flags.StandardParser, *cobra.Command) {
	t.Helper()
	parser := flags.NewStandardParser(
		flags.WithBoolFlag("verify-plan", "", false, "Enable planfile drift verification"),
		flags.WithEnvVars("verify-plan", "ATMOS_TEST_VERIFY_PLAN"),
	)
	cmd := &cobra.Command{Use: "deploy"}
	parser.RegisterFlags(cmd)
	require.NoError(t, parser.BindToViper(viper.New()))
	return parser, cmd
}

// TestIsBoolFlagExplicitlySet verifies all tri-state cases for bool flags.
func TestIsBoolFlagExplicitlySet(t *testing.T) {
	t.Run("flag unset and env unset: not set", func(t *testing.T) {
		parser, cmd := newTriStateParser(t)
		set, val := parser.IsBoolFlagExplicitlySet(cmd, "verify-plan")
		assert.False(t, set, "should report not set when neither flag nor env was provided")
		assert.False(t, val, "value should be false (zero) when not set")
	})

	t.Run("flag set true via CLI: set=true value=true", func(t *testing.T) {
		parser, cmd := newTriStateParser(t)
		require.NoError(t, cmd.Flags().Set("verify-plan", "true"))
		set, val := parser.IsBoolFlagExplicitlySet(cmd, "verify-plan")
		assert.True(t, set, "should report set when flag was explicitly passed")
		assert.True(t, val, "value should be true when --verify-plan=true")
	})

	t.Run("flag set false via CLI: set=true value=false", func(t *testing.T) {
		parser, cmd := newTriStateParser(t)
		require.NoError(t, cmd.Flags().Set("verify-plan", "false"))
		set, val := parser.IsBoolFlagExplicitlySet(cmd, "verify-plan")
		assert.True(t, set, "should report set when flag was explicitly passed as false")
		assert.False(t, val, "value should be false when --verify-plan=false")
	})

	t.Run("env var true, no CLI flag: set=true value=true", func(t *testing.T) {
		t.Setenv("ATMOS_TEST_VERIFY_PLAN", "true")
		parser, cmd := newTriStateParser(t)
		set, val := parser.IsBoolFlagExplicitlySet(cmd, "verify-plan")
		assert.True(t, set, "should report set when env var is present")
		assert.True(t, val, "value should be true when env var=true")
	})

	t.Run("env var false, no CLI flag: set=true value=false", func(t *testing.T) {
		t.Setenv("ATMOS_TEST_VERIFY_PLAN", "false")
		parser, cmd := newTriStateParser(t)
		set, val := parser.IsBoolFlagExplicitlySet(cmd, "verify-plan")
		assert.True(t, set, "should report set when env var is present")
		assert.False(t, val, "value should be false when env var=false")
	})

	t.Run("CLI false wins over env var true: set=true value=false", func(t *testing.T) {
		t.Setenv("ATMOS_TEST_VERIFY_PLAN", "true")
		parser, cmd := newTriStateParser(t)
		require.NoError(t, cmd.Flags().Set("verify-plan", "false"))
		set, val := parser.IsBoolFlagExplicitlySet(cmd, "verify-plan")
		assert.True(t, set, "should report set")
		assert.False(t, val, "CLI flag=false must win over env var=true")
	})

	t.Run("nil command falls back to env var: set=true value=true", func(t *testing.T) {
		t.Setenv("ATMOS_TEST_VERIFY_PLAN", "true")
		parser, _ := newTriStateParser(t)
		set, val := parser.IsBoolFlagExplicitlySet(nil, "verify-plan")
		assert.True(t, set, "should report set via env var even when cmd is nil")
		assert.True(t, val, "value should be true from env var")
	})

	t.Run("nil command no env: not set", func(t *testing.T) {
		parser, _ := newTriStateParser(t)
		set, val := parser.IsBoolFlagExplicitlySet(nil, "verify-plan")
		assert.False(t, set, "should report not set when cmd=nil and no env var")
		assert.False(t, val, "value should be false when not set")
	})

	t.Run("unparseable env var is ignored: not set", func(t *testing.T) {
		t.Setenv("ATMOS_TEST_VERIFY_PLAN", "notabool")
		parser, cmd := newTriStateParser(t)
		set, val := parser.IsBoolFlagExplicitlySet(cmd, "verify-plan")
		assert.False(t, set, "unparseable env var must not count as explicitly set")
		assert.False(t, val, "value should be false when env var is not parseable")
	})

	t.Run("viper SetDefault does not cause false positive: not set", func(t *testing.T) {
		// This is the core regression guard for the viper.IsSet false-positive bug.
		// After SetDefault(false) viper.IsSet returns true, which was the original bug.
		// Our implementation must return (false, false) when only the default is registered.
		parser, cmd := newTriStateParser(t)
		// Explicitly call BindFlagsToViper to trigger SetDefault (same as RunE does).
		v := viper.New()
		require.NoError(t, parser.BindFlagsToViper(cmd, v))
		set, val := parser.IsBoolFlagExplicitlySet(cmd, "verify-plan")
		assert.False(t, set, "SetDefault must not cause IsBoolFlagExplicitlySet to report set")
		assert.False(t, val, "value should be false when only SetDefault was called")
	})

	t.Run("unknown flag name: not set", func(t *testing.T) {
		parser, cmd := newTriStateParser(t)
		set, val := parser.IsBoolFlagExplicitlySet(cmd, "does-not-exist")
		assert.False(t, set, "unknown flag should report not set")
		assert.False(t, val, "unknown flag should return false value")
	})
}
