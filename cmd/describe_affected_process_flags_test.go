package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
)

// newProcessFlagsTestCmd builds a bare command with only the --process-templates and
// --process-functions flags registered (as persistent flags, matching production), plus a
// fresh, isolated Viper. It avoids the global viper.GetViper() so tests don't leak state into
// each other or into the real describeAffectedCmd.
//
// The cliArgs parameter simulates the command line; ParseFlags both merges the persistent
// flags into cmd.Flags() (which cobra does during execution, but tests skip) and applies any
// explicit flag values, marking them Changed exactly as a real invocation would.
func newProcessFlagsTestCmd(t *testing.T, cliArgs ...string) (*cobra.Command, *viper.Viper) {
	t.Helper()

	parser := newDescribeAffectedProcessFlagsParser()
	cmd := &cobra.Command{Use: "affected"}
	parser.RegisterPersistentFlags(cmd)
	require.NoError(t, cmd.ParseFlags(cliArgs))

	// Bind the ATMOS_PROCESS_* env vars to v, as init() does in production; the resolver reads
	// them from v but does not bind them itself.
	v := viper.New()
	require.NoError(t, parser.BindToViper(v))
	return cmd, v
}

// mustResolveProcessFlags runs the resolver and fails the test on any (unexpected) error.
func mustResolveProcessFlags(t *testing.T, cmd *cobra.Command, v *viper.Viper) {
	t.Helper()
	require.NoError(t, resolveDescribeAffectedProcessFlags(cmd, v))
}

// TestNewDescribeAffectedProcessFlagsParser verifies both flags register with the expected
// name, default (true), and description.
func TestNewDescribeAffectedProcessFlagsParser(t *testing.T) {
	parser := newDescribeAffectedProcessFlagsParser()
	require.NotNil(t, parser)

	cmd := &cobra.Command{Use: "affected"}
	parser.RegisterPersistentFlags(cmd)

	for _, name := range []string{processTemplatesFlagName, processFunctionsFlagName} {
		flag := cmd.PersistentFlags().Lookup(name)
		require.NotNilf(t, flag, "flag %q must be registered", name)
		assert.Equal(t, "true", flag.DefValue, "flag %q must default to true", name)
		assert.NotEmpty(t, flag.Usage, "flag %q must have a description", name)
	}
}

// TestResolveDescribeAffectedProcessFlags_NoOverride confirms that with neither an env var nor
// a CLI flag set, both flags are left untouched at their default (true).
func TestResolveDescribeAffectedProcessFlags_NoOverride(t *testing.T) {
	cmd, v := newProcessFlagsTestCmd(t)

	mustResolveProcessFlags(t, cmd, v)

	for _, name := range []string{processTemplatesFlagName, processFunctionsFlagName} {
		assert.Falsef(t, cmd.Flags().Changed(name), "flag %q must not be marked changed when nothing set it", name)
		val, err := cmd.Flags().GetBool(name)
		require.NoError(t, err)
		assert.Truef(t, val, "flag %q must keep its default (true)", name)
	}
}

// TestResolveDescribeAffectedProcessFlags_EnvVarTakesEffect covers each env var independently
// disabling its flag, and confirms the other flag is left at its default.
func TestResolveDescribeAffectedProcessFlags_EnvVarTakesEffect(t *testing.T) {
	tests := []struct {
		name          string
		envVar        string
		targetFlag    string
		untouchedFlag string
	}{
		{
			name:          "ATMOS_PROCESS_FUNCTIONS disables process-functions only",
			envVar:        "ATMOS_PROCESS_FUNCTIONS",
			targetFlag:    processFunctionsFlagName,
			untouchedFlag: processTemplatesFlagName,
		},
		{
			name:          "ATMOS_PROCESS_TEMPLATES disables process-templates only",
			envVar:        "ATMOS_PROCESS_TEMPLATES",
			targetFlag:    processTemplatesFlagName,
			untouchedFlag: processFunctionsFlagName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, v := newProcessFlagsTestCmd(t)
			t.Setenv(tt.envVar, "false")

			mustResolveProcessFlags(t, cmd, v)

			target, err := cmd.Flags().GetBool(tt.targetFlag)
			require.NoError(t, err)
			assert.Falsef(t, target, "%s must set %s to false", tt.envVar, tt.targetFlag)
			assert.Truef(t, cmd.Flags().Changed(tt.targetFlag), "%s must mark %s changed, not a coincidental default", tt.envVar, tt.targetFlag)

			untouched, err := cmd.Flags().GetBool(tt.untouchedFlag)
			require.NoError(t, err)
			assert.Truef(t, untouched, "%s must keep its default (true)", tt.untouchedFlag)
			assert.Falsef(t, cmd.Flags().Changed(tt.untouchedFlag), "%s must not be marked changed", tt.untouchedFlag)
		})
	}
}

// TestResolveDescribeAffectedProcessFlags_EnvVarTrue confirms an env var set to the flag's
// default value (true) is a correct no-op: the resolved value stays true. The flag is not
// marked changed because the resolved value already matches the default — there is nothing to
// override, and the effective value is identical either way.
func TestResolveDescribeAffectedProcessFlags_EnvVarTrue(t *testing.T) {
	cmd, v := newProcessFlagsTestCmd(t)
	t.Setenv("ATMOS_PROCESS_FUNCTIONS", "true")

	mustResolveProcessFlags(t, cmd, v)

	val, err := cmd.Flags().GetBool(processFunctionsFlagName)
	require.NoError(t, err)
	assert.True(t, val)
}

// TestResolveDescribeAffectedProcessFlags_CLIWinsOverEnv verifies CLI > env precedence: an
// explicitly-set flag is not overwritten by a conflicting env var.
func TestResolveDescribeAffectedProcessFlags_CLIWinsOverEnv(t *testing.T) {
	// Simulate `--process-functions=true` on the command line.
	cmd, v := newProcessFlagsTestCmd(t, "--process-functions=true")
	require.True(t, cmd.Flags().Changed(processFunctionsFlagName))

	// A conflicting env var must NOT override the explicit CLI value.
	t.Setenv("ATMOS_PROCESS_FUNCTIONS", "false")

	mustResolveProcessFlags(t, cmd, v)

	val, err := cmd.Flags().GetBool(processFunctionsFlagName)
	require.NoError(t, err)
	assert.True(t, val, "explicit CLI flag must win over the env var")
}

// TestResolveDescribeAffectedProcessFlags_ViperKeyTakesEffect confirms a value set directly on
// the (bare) Viper key (e.g. from a config source) is applied.
func TestResolveDescribeAffectedProcessFlags_ViperKeyTakesEffect(t *testing.T) {
	cmd, v := newProcessFlagsTestCmd(t)
	v.Set(processFunctionsFlagName, false)

	mustResolveProcessFlags(t, cmd, v)

	val, err := cmd.Flags().GetBool(processFunctionsFlagName)
	require.NoError(t, err)
	assert.False(t, val)
	assert.True(t, cmd.Flags().Changed(processFunctionsFlagName))
}

// TestResolveDescribeAffectedProcessFlags_UnsetOnUnboundViperNoOp verifies that when Viper has
// no binding or default for the keys (e.g. after viper.Reset(), which the real command's tests
// do), the resolver treats the empty value as unset — leaving the flag at its default and
// returning no error, rather than failing to parse "".
func TestResolveDescribeAffectedProcessFlags_UnsetOnUnboundViperNoOp(t *testing.T) {
	parser := newDescribeAffectedProcessFlagsParser()
	cmd := &cobra.Command{Use: "affected"}
	parser.RegisterPersistentFlags(cmd)
	require.NoError(t, cmd.ParseFlags(nil))

	// Deliberately NOT bound: no env binding and no SetDefault, so GetString returns "".
	v := viper.New()

	require.NoError(t, resolveDescribeAffectedProcessFlags(cmd, v))

	assert.False(t, cmd.Flags().Changed(processFunctionsFlagName))
	val, err := cmd.Flags().GetBool(processFunctionsFlagName)
	require.NoError(t, err)
	assert.True(t, val, "flag must keep its default when Viper has no value for it")
}

// TestResolveDescribeAffectedProcessFlags_InvalidEnvValueErrors verifies a non-empty invalid
// boolean env value is rejected with a wrapped ErrInvalidFlagValue rather than silently coerced
// to false (which viper's GetBool would do), and that the flag is left untouched.
func TestResolveDescribeAffectedProcessFlags_InvalidEnvValueErrors(t *testing.T) {
	cmd, v := newProcessFlagsTestCmd(t)
	t.Setenv("ATMOS_PROCESS_FUNCTIONS", "yes")

	err := resolveDescribeAffectedProcessFlags(cmd, v)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
	assert.Contains(t, err.Error(), "ATMOS_PROCESS_FUNCTIONS", "error must name the offending env var")
	assert.False(t, cmd.Flags().Changed(processFunctionsFlagName), "an invalid value must not flip the flag")
}

// TestResolveDescribeAffectedProcessFlags_EmptyEnvValueFallsBackToDefault verifies an empty env
// value is treated as unset (viper's AllowEmptyEnv is off by default), so the flag keeps its
// default and no error is returned — it must not take the invalid-to-false path.
func TestResolveDescribeAffectedProcessFlags_EmptyEnvValueFallsBackToDefault(t *testing.T) {
	cmd, v := newProcessFlagsTestCmd(t)
	t.Setenv("ATMOS_PROCESS_FUNCTIONS", "")

	mustResolveProcessFlags(t, cmd, v)

	val, err := cmd.Flags().GetBool(processFunctionsFlagName)
	require.NoError(t, err)
	assert.True(t, val, "empty env value must fall back to the default (true)")
	assert.False(t, cmd.Flags().Changed(processFunctionsFlagName))
}

// TestDescribeAffected_InvalidProcessEnvVarReturnsError verifies an invalid ATMOS_PROCESS_*
// value surfaces as a command error through the real RunE path (getRunnableDescribeAffectedCmd),
// not just the resolver in isolation.
func TestDescribeAffected_InvalidProcessEnvVarReturnsError(t *testing.T) {
	_ = NewTestKit(t)

	// getRunnableDescribeAffectedCmd resolves against the global Viper; bind the env vars there
	// (idempotent with init()) so the binding survives any viper.Reset() from other tests.
	require.NoError(t, newDescribeAffectedProcessFlagsParser().BindToViper(viper.GetViper()))

	testCmd := &cobra.Command{Use: "affected"}
	newDescribeAffectedProcessFlagsParser().RegisterPersistentFlags(testCmd)
	require.NoError(t, testCmd.ParseFlags(nil))

	t.Setenv("ATMOS_PROCESS_FUNCTIONS", "garbage")

	run := getRunnableDescribeAffectedCmd(
		func(_ ...AtmosValidateOption) {},
		func(_ *cobra.Command, _ []string) (exec.DescribeAffectedCmdArgs, error) {
			t.Fatal("parse must not be reached when the env var is invalid")
			return exec.DescribeAffectedCmdArgs{}, nil
		},
		func(_ *schema.AtmosConfiguration) exec.DescribeAffectedExec {
			t.Fatal("exec must not be reached when the env var is invalid")
			return nil
		},
	)

	err := run(testCmd, []string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
}

// TestResolveDescribeAffectedProcessFlags_UnregisteredFlagsNoOp verifies the resolver is a
// safe no-op on a command that never registered these flags (e.g. a synthetic test command),
// rather than erroring on an undefined flag.
func TestResolveDescribeAffectedProcessFlags_UnregisteredFlagsNoOp(t *testing.T) {
	cmd := &cobra.Command{Use: "affected"}
	v := viper.New()
	t.Setenv("ATMOS_PROCESS_FUNCTIONS", "false")

	// Must not panic or create a flag when the command never registered these flags.
	mustResolveProcessFlags(t, cmd, v)

	assert.Nil(t, cmd.Flags().Lookup(processFunctionsFlagName), "no flag should be created by the resolver")
}

// TestResolveDescribeAffectedProcessFlags_EnvReachesCliArgs is the end-to-end regression test
// for the bug this change fixes: ATMOS_PROCESS_FUNCTIONS must reach
// DescribeAffectedCmdArgs.ProcessYamlFunctions through the legacy
// exec.SetDescribeAffectedFlagValueInCliArgs reader (which only reads flags marked Changed).
func TestResolveDescribeAffectedProcessFlags_EnvReachesCliArgs(t *testing.T) {
	cmd, v := newProcessFlagsTestCmd(t)
	t.Setenv("ATMOS_PROCESS_FUNCTIONS", "false")

	mustResolveProcessFlags(t, cmd, v)

	describe := exec.DescribeAffectedCmdArgs{CLIConfig: &schema.AtmosConfiguration{}}
	exec.SetDescribeAffectedFlagValueInCliArgs(cmd.Flags(), &describe)

	assert.False(t, describe.ProcessYamlFunctions, "ATMOS_PROCESS_FUNCTIONS=false must disable YAML function processing")
	assert.True(t, describe.ProcessTemplates, "process-templates must keep its default when only ATMOS_PROCESS_FUNCTIONS is set")
}
