package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
)

// atmosConfigWithBrokenDefaultIdentity returns an AtmosConfiguration with a default
// identity that references a non-existent provider. If auth resolution is attempted,
// it will fail — causing the command to return an error. This ensures the test fails
// if someone removes the process-functions guard that skips auth.
func atmosConfigWithBrokenDefaultIdentity() schema.AtmosConfiguration {
	return schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Identities: map[string]schema.Identity{
				"broken-identity": {
					Default: true,
					Kind:    "aws",
					Via:     &schema.IdentityVia{Provider: "nonexistent-provider"},
				},
			},
		},
	}
}

// newTestCmdWithFunctionsDisabled creates a minimal cobra.Command with
// --process-functions=false and an --identity flag (for GetIdentityFromFlags).
// Uses local flags (not PersistentFlags) because Cobra does not propagate
// the Changed state from PersistentFlags to the merged Flags() FlagSet.
func newTestCmdWithFunctionsDisabled(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Bool("process-functions", true, "")
	cmd.Flags().StringP("identity", "i", "", "")
	require.NoError(t, cmd.Flags().Set("process-functions", "false"))
	return cmd
}

// newTestCmdWithFunctionsDisabledAndExplicitIdentity creates a command with
// --process-functions=false AND --identity explicitly set (simulates CLI flag).
func newTestCmdWithFunctionsDisabledAndExplicitIdentity(t *testing.T, identityValue string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Bool("process-functions", true, "")
	cmd.Flags().StringP("identity", "i", "", "")
	require.NoError(t, cmd.Flags().Set("process-functions", "false"))
	require.NoError(t, cmd.Flags().Set("identity", identityValue))
	return cmd
}

// clearIdentityEnvVars prevents CI-set identity env vars from triggering auth.
func clearIdentityEnvVars(t *testing.T) {
	t.Helper()
	viper.Reset()
	t.Setenv("ATMOS_IDENTITY", "")
	t.Setenv("IDENTITY", "")
}

// TestDescribeStacks_SkipsAuthWhenFunctionsDisabled verifies that describe stacks
// does not attempt identity resolution when --process-functions=false.
//
// Regression protection: the returned AtmosConfiguration has a default identity with
// a broken provider reference. If the guard were removed, auth would be attempted,
// fail, and the test would catch the error.
func TestDescribeStacks_SkipsAuthWhenFunctionsDisabled(t *testing.T) {
	_ = NewTestKit(t)
	clearIdentityEnvVars(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExec := exec.NewMockDescribeStacksExec(ctrl)
	mockExec.EXPECT().Execute(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ *schema.AtmosConfiguration, args *exec.DescribeStacksArgs) error {
			assert.Nil(t, args.AuthManager, "AuthManager must be nil when --process-functions=false")
			assert.False(t, args.ProcessYamlFunctions, "ProcessYamlFunctions must be false")
			return nil
		},
	).Times(1)

	testCmd := newTestCmdWithFunctionsDisabled(t)

	run := getRunnableDescribeStacksCmd(getRunnableDescribeStacksCmdProps{
		func(opts ...AtmosValidateOption) {},
		func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
			return schema.ConfigAndStacksInfo{}, nil
		},
		func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
			return atmosConfigWithBrokenDefaultIdentity(), nil
		},
		func(_ *schema.AtmosConfiguration) error { return nil },
		func(_ *pflag.FlagSet, _ *exec.DescribeStacksArgs) error { return nil },
		mockExec,
	})

	err := run(testCmd, []string{})
	assert.NoError(t, err, "should succeed when functions disabled — auth must be skipped entirely")
}

// TestDescribeStacks_SkipsAuthWhenEnvVarSetButFunctionsDisabled verifies that an
// ATMOS_IDENTITY environment variable does NOT bypass the process-functions guard.
// Only an explicit --identity CLI flag should force auth when functions are disabled.
func TestDescribeStacks_SkipsAuthWhenEnvVarSetButFunctionsDisabled(t *testing.T) {
	_ = NewTestKit(t)
	viper.Reset()
	// Re-bind after reset so viper can see the env var via GetIdentityFromFlags.
	require.NoError(t, viper.BindEnv(IdentityFlagName, "ATMOS_IDENTITY", "IDENTITY"))
	// Set the env var — this should NOT trigger auth when functions are disabled.
	t.Setenv("ATMOS_IDENTITY", "some-identity")
	t.Setenv("IDENTITY", "")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExec := exec.NewMockDescribeStacksExec(ctrl)
	mockExec.EXPECT().Execute(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ *schema.AtmosConfiguration, args *exec.DescribeStacksArgs) error {
			assert.Nil(t, args.AuthManager, "AuthManager must be nil when ATMOS_IDENTITY env var is set but --process-functions=false and no explicit --identity flag")
			assert.False(t, args.ProcessYamlFunctions, "ProcessYamlFunctions must be false")
			return nil
		},
	).Times(1)

	testCmd := newTestCmdWithFunctionsDisabled(t)

	run := getRunnableDescribeStacksCmd(getRunnableDescribeStacksCmdProps{
		func(opts ...AtmosValidateOption) {},
		func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
			return schema.ConfigAndStacksInfo{}, nil
		},
		func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
			return atmosConfigWithBrokenDefaultIdentity(), nil
		},
		func(_ *schema.AtmosConfiguration) error { return nil },
		func(_ *pflag.FlagSet, _ *exec.DescribeStacksArgs) error { return nil },
		mockExec,
	})

	err := run(testCmd, []string{})
	assert.NoError(t, err, "env var ATMOS_IDENTITY must not trigger auth when --process-functions=false")
}

// TestDescribeStacks_ExplicitIdentityForcesAuthWhenFunctionsDisabled verifies that
// an explicit --identity CLI flag forces auth even when --process-functions=false.
// We prove auth is attempted by providing a broken config — if the guard skipped auth,
// no error would occur. The error proves the guard was bypassed (as intended).
func TestDescribeStacks_ExplicitIdentityForcesAuthWhenFunctionsDisabled(t *testing.T) {
	_ = NewTestKit(t)
	clearIdentityEnvVars(t)

	// Use a command with --identity explicitly set via the CLI flag.
	testCmd := newTestCmdWithFunctionsDisabledAndExplicitIdentity(t, "broken-identity")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Mock should NOT be called — auth should fail before execution.
	mockExec := exec.NewMockDescribeStacksExec(ctrl)

	run := getRunnableDescribeStacksCmd(getRunnableDescribeStacksCmdProps{
		func(opts ...AtmosValidateOption) {},
		func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
			return schema.ConfigAndStacksInfo{}, nil
		},
		func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
			return atmosConfigWithBrokenDefaultIdentity(), nil
		},
		func(_ *schema.AtmosConfiguration) error { return nil },
		func(_ *pflag.FlagSet, _ *exec.DescribeStacksArgs) error { return nil },
		mockExec,
	})

	err := run(testCmd, []string{})
	assert.Error(t, err, "explicit --identity must trigger auth even when functions disabled; broken config should cause error")
}

// TestDescribeDependents_SkipsAuthWhenFunctionsDisabled verifies that describe dependents
// does not attempt identity resolution when --process-functions=false.
func TestDescribeDependents_SkipsAuthWhenFunctionsDisabled(t *testing.T) {
	_ = NewTestKit(t)
	clearIdentityEnvVars(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExec := exec.NewMockDescribeDependentsExec(ctrl)
	mockExec.EXPECT().Execute(gomock.Any()).DoAndReturn(
		func(props *exec.DescribeDependentsExecProps) error {
			assert.Nil(t, props.AuthManager, "AuthManager must be nil when --process-functions=false")
			assert.False(t, props.ProcessYamlFunctions, "ProcessYamlFunctions must be false")
			return nil
		},
	).Times(1)

	testCmd := newTestCmdWithFunctionsDisabled(t)

	run := getRunnableDescribeDependentsCmd(
		func(opts ...AtmosValidateOption) {},
		func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
			return schema.ConfigAndStacksInfo{}, nil
		},
		func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
			return atmosConfigWithBrokenDefaultIdentity(), nil
		},
		func(_ *schema.AtmosConfiguration) exec.DescribeDependentsExec {
			return mockExec
		},
	)

	err := run(testCmd, []string{"test-component"})
	assert.NoError(t, err, "should succeed when functions disabled — auth must be skipped entirely")
}

// TestDescribeDependents_SkipsAuthWhenEnvVarSetButFunctionsDisabled verifies that
// ATMOS_IDENTITY env var does not bypass the guard for describe dependents.
func TestDescribeDependents_SkipsAuthWhenEnvVarSetButFunctionsDisabled(t *testing.T) {
	_ = NewTestKit(t)
	viper.Reset()
	require.NoError(t, viper.BindEnv(IdentityFlagName, "ATMOS_IDENTITY", "IDENTITY"))
	t.Setenv("ATMOS_IDENTITY", "some-identity")
	t.Setenv("IDENTITY", "")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExec := exec.NewMockDescribeDependentsExec(ctrl)
	mockExec.EXPECT().Execute(gomock.Any()).DoAndReturn(
		func(props *exec.DescribeDependentsExecProps) error {
			assert.Nil(t, props.AuthManager, "AuthManager must be nil when ATMOS_IDENTITY env var set but no explicit --identity flag")
			assert.False(t, props.ProcessYamlFunctions, "ProcessYamlFunctions must be false")
			return nil
		},
	).Times(1)

	testCmd := newTestCmdWithFunctionsDisabled(t)

	run := getRunnableDescribeDependentsCmd(
		func(opts ...AtmosValidateOption) {},
		func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
			return schema.ConfigAndStacksInfo{}, nil
		},
		func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
			return atmosConfigWithBrokenDefaultIdentity(), nil
		},
		func(_ *schema.AtmosConfiguration) exec.DescribeDependentsExec {
			return mockExec
		},
	)

	err := run(testCmd, []string{"test-component"})
	assert.NoError(t, err, "env var ATMOS_IDENTITY must not trigger auth when --process-functions=false")
}

// TestDescribeDependents_ExplicitIdentityForcesAuthWhenFunctionsDisabled verifies that
// an explicit --identity CLI flag forces auth even when --process-functions=false.
// We prove auth is attempted by providing a broken config — the error proves the guard was bypassed.
func TestDescribeDependents_ExplicitIdentityForcesAuthWhenFunctionsDisabled(t *testing.T) {
	_ = NewTestKit(t)
	clearIdentityEnvVars(t)

	testCmd := newTestCmdWithFunctionsDisabledAndExplicitIdentity(t, "broken-identity")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Mock should NOT be called — auth should fail before execution.
	mockExec := exec.NewMockDescribeDependentsExec(ctrl)

	run := getRunnableDescribeDependentsCmd(
		func(opts ...AtmosValidateOption) {},
		func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
			return schema.ConfigAndStacksInfo{}, nil
		},
		func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
			return atmosConfigWithBrokenDefaultIdentity(), nil
		},
		func(_ *schema.AtmosConfiguration) exec.DescribeDependentsExec {
			return mockExec
		},
	)

	err := run(testCmd, []string{"test-component"})
	assert.Error(t, err, "explicit --identity must trigger auth even when functions disabled; broken config should cause error")
}

// TestDescribeAffected_SkipsAuthWhenFunctionsDisabled verifies that describe affected
// does not attempt identity resolution when ProcessYamlFunctions=false.
func TestDescribeAffected_SkipsAuthWhenFunctionsDisabled(t *testing.T) {
	_ = NewTestKit(t)
	clearIdentityEnvVars(t)

	brokenConfig := atmosConfigWithBrokenDefaultIdentity()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExec := exec.NewMockDescribeAffectedExec(ctrl)
	mockExec.EXPECT().Execute(gomock.Any()).DoAndReturn(
		func(args *exec.DescribeAffectedCmdArgs) error {
			assert.Nil(t, args.AuthManager, "AuthManager must be nil when ProcessYamlFunctions=false")
			assert.False(t, args.ProcessYamlFunctions, "ProcessYamlFunctions must be false")
			return nil
		},
	).Times(1)

	testCmd := newTestCmdWithFunctionsDisabled(t)

	run := getRunnableDescribeAffectedCmd(
		func(opts ...AtmosValidateOption) {},
		func(_ *cobra.Command, _ []string) (exec.DescribeAffectedCmdArgs, error) {
			return exec.DescribeAffectedCmdArgs{
				CLIConfig:            &brokenConfig,
				ProcessYamlFunctions: false,
				Format:               "json",
			}, nil
		},
		func(_ *schema.AtmosConfiguration) exec.DescribeAffectedExec {
			return mockExec
		},
	)

	err := run(testCmd, []string{})
	assert.NoError(t, err, "should succeed when functions disabled — auth must be skipped entirely")
}

// TestDescribeAffected_SkipsAuthWhenEnvVarSetButFunctionsDisabled verifies that
// ATMOS_IDENTITY env var does not bypass the guard for describe affected.
func TestDescribeAffected_SkipsAuthWhenEnvVarSetButFunctionsDisabled(t *testing.T) {
	_ = NewTestKit(t)
	viper.Reset()
	require.NoError(t, viper.BindEnv(IdentityFlagName, "ATMOS_IDENTITY", "IDENTITY"))
	t.Setenv("ATMOS_IDENTITY", "some-identity")
	t.Setenv("IDENTITY", "")

	brokenConfig := atmosConfigWithBrokenDefaultIdentity()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExec := exec.NewMockDescribeAffectedExec(ctrl)
	mockExec.EXPECT().Execute(gomock.Any()).DoAndReturn(
		func(args *exec.DescribeAffectedCmdArgs) error {
			assert.Nil(t, args.AuthManager, "AuthManager must be nil when ATMOS_IDENTITY env var set but no explicit --identity flag")
			assert.False(t, args.ProcessYamlFunctions, "ProcessYamlFunctions must be false")
			return nil
		},
	).Times(1)

	testCmd := newTestCmdWithFunctionsDisabled(t)

	run := getRunnableDescribeAffectedCmd(
		func(opts ...AtmosValidateOption) {},
		func(_ *cobra.Command, _ []string) (exec.DescribeAffectedCmdArgs, error) {
			return exec.DescribeAffectedCmdArgs{
				CLIConfig:            &brokenConfig,
				ProcessYamlFunctions: false,
				Format:               "json",
			}, nil
		},
		func(_ *schema.AtmosConfiguration) exec.DescribeAffectedExec {
			return mockExec
		},
	)

	err := run(testCmd, []string{})
	assert.NoError(t, err, "env var ATMOS_IDENTITY must not trigger auth when --process-functions=false")
}

// TestDescribeAffected_ExplicitIdentityForcesAuthWhenFunctionsDisabled verifies that
// an explicit --identity CLI flag forces auth even when --process-functions=false.
// We prove auth is attempted by providing a broken config — the error proves the guard was bypassed.
func TestDescribeAffected_ExplicitIdentityForcesAuthWhenFunctionsDisabled(t *testing.T) {
	_ = NewTestKit(t)
	clearIdentityEnvVars(t)

	brokenConfig := atmosConfigWithBrokenDefaultIdentity()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Mock should NOT be called — auth should fail before execution.
	mockExec := exec.NewMockDescribeAffectedExec(ctrl)

	testCmd := newTestCmdWithFunctionsDisabledAndExplicitIdentity(t, "broken-identity")

	run := getRunnableDescribeAffectedCmd(
		func(opts ...AtmosValidateOption) {},
		func(_ *cobra.Command, _ []string) (exec.DescribeAffectedCmdArgs, error) {
			return exec.DescribeAffectedCmdArgs{
				CLIConfig:            &brokenConfig,
				ProcessYamlFunctions: false,
				Format:               "json",
			}, nil
		},
		func(_ *schema.AtmosConfiguration) exec.DescribeAffectedExec {
			return mockExec
		},
	)

	err := run(testCmd, []string{})
	assert.Error(t, err, "explicit --identity must trigger auth even when functions disabled; broken config should cause error")
}

// newTestCmdForDescribeComponent creates a minimal cobra.Command with all flags needed
// for the describe component command. Uses local flags because Cobra does not propagate
// the Changed state from PersistentFlags to the merged Flags() FlagSet.
func newTestCmdForDescribeComponent(t *testing.T, processFunctions bool, identityValue string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringP("stack", "s", "", "")
	cmd.Flags().StringP("format", "f", "yaml", "")
	cmd.Flags().String("file", "", "")
	cmd.Flags().Bool("process-templates", true, "")
	cmd.Flags().Bool("process-functions", true, "")
	cmd.Flags().String("query", "", "")
	cmd.Flags().StringSlice("skip", nil, "")
	cmd.Flags().Bool("provenance", false, "")
	cmd.Flags().StringP("identity", "i", "", "")
	require.NoError(t, cmd.Flags().Set("stack", "test-stack"))
	if !processFunctions {
		require.NoError(t, cmd.Flags().Set("process-functions", "false"))
	}
	if identityValue != "" {
		require.NoError(t, cmd.Flags().Set("identity", identityValue))
	}
	return cmd
}

// describeComponentTestProps returns default props for describe component tests.
// initCliConfig returns a minimal config. executeDescribeComponent and resolveComponentFromPath
// are stubs since the auth guard is exercised before they would be called in the skip case.
func describeComponentTestProps(mockExec exec.DescribeComponentCmdExec, atmosConfig schema.AtmosConfiguration) getRunnableDescribeComponentCmdProps {
	return getRunnableDescribeComponentCmdProps{
		checkAtmosConfigE: func(_ ...AtmosValidateOption) error { return nil },
		initCliConfig: func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
			return atmosConfig, nil
		},
		isExplicitComponentPath: func(_ string) bool { return false },
		resolveComponentFromPath: func(_ *schema.AtmosConfiguration, _ string, _ string) (string, error) {
			return "", nil
		},
		executeDescribeComponent: func(_ *exec.ExecuteDescribeComponentParams) (map[string]any, error) {
			return map[string]any{}, nil
		},
		newDescribeComponentExec: mockExec,
	}
}

// TestDescribeComponent_SkipsAuthWhenFunctionsDisabled verifies that describe component
// does not attempt identity resolution when --process-functions=false.
func TestDescribeComponent_SkipsAuthWhenFunctionsDisabled(t *testing.T) {
	_ = NewTestKit(t)
	clearIdentityEnvVars(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExec := exec.NewMockDescribeComponentCmdExec(ctrl)
	mockExec.EXPECT().ExecuteDescribeComponentCmd(gomock.Any()).DoAndReturn(
		func(params exec.DescribeComponentParams) error {
			assert.Nil(t, params.AuthManager, "AuthManager must be nil when --process-functions=false")
			assert.False(t, params.ProcessYamlFunctions, "ProcessYamlFunctions must be false")
			return nil
		},
	).Times(1)

	testCmd := newTestCmdForDescribeComponent(t, false, "")
	run := getRunnableDescribeComponentCmd(describeComponentTestProps(mockExec, atmosConfigWithBrokenDefaultIdentity()))

	err := run(testCmd, []string{"test-component"})
	assert.NoError(t, err, "should succeed when functions disabled — auth must be skipped entirely")
}

// TestDescribeComponent_SkipsAuthWhenEnvVarSetButFunctionsDisabled verifies that
// ATMOS_IDENTITY env var does not bypass the guard for describe component.
func TestDescribeComponent_SkipsAuthWhenEnvVarSetButFunctionsDisabled(t *testing.T) {
	_ = NewTestKit(t)
	viper.Reset()
	require.NoError(t, viper.BindEnv(IdentityFlagName, "ATMOS_IDENTITY", "IDENTITY"))
	t.Setenv("ATMOS_IDENTITY", "some-identity")
	t.Setenv("IDENTITY", "")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExec := exec.NewMockDescribeComponentCmdExec(ctrl)
	mockExec.EXPECT().ExecuteDescribeComponentCmd(gomock.Any()).DoAndReturn(
		func(params exec.DescribeComponentParams) error {
			assert.Nil(t, params.AuthManager, "AuthManager must be nil when ATMOS_IDENTITY env var set but no explicit --identity flag")
			assert.False(t, params.ProcessYamlFunctions, "ProcessYamlFunctions must be false")
			return nil
		},
	).Times(1)

	testCmd := newTestCmdForDescribeComponent(t, false, "")
	run := getRunnableDescribeComponentCmd(describeComponentTestProps(mockExec, atmosConfigWithBrokenDefaultIdentity()))

	err := run(testCmd, []string{"test-component"})
	assert.NoError(t, err, "env var ATMOS_IDENTITY must not trigger auth when --process-functions=false")
}

// TestDescribeComponent_ExplicitIdentityForcesAuthWhenFunctionsDisabled verifies that
// an explicit --identity CLI flag forces auth even when --process-functions=false.
// We prove auth is attempted by providing a broken config — the error proves the guard was bypassed.
func TestDescribeComponent_ExplicitIdentityForcesAuthWhenFunctionsDisabled(t *testing.T) {
	_ = NewTestKit(t)
	clearIdentityEnvVars(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Mock should NOT be called — auth should fail before execution.
	mockExec := exec.NewMockDescribeComponentCmdExec(ctrl)

	testCmd := newTestCmdForDescribeComponent(t, false, "broken-identity")
	run := getRunnableDescribeComponentCmd(describeComponentTestProps(mockExec, atmosConfigWithBrokenDefaultIdentity()))

	err := run(testCmd, []string{"test-component"})
	assert.Error(t, err, "explicit --identity must trigger auth even when functions disabled; broken config should cause error")
}

// TestIdentityExplicitGuard_FlagChangedVsEnvVar verifies the guard logic unit:
// cmd.Flags().Changed(IdentityFlagName) must be false when only env var is set,
// and true when the CLI flag is explicitly set. This protects all describe commands
// that share the identityExplicit guard pattern (including describe component which
// is not testable via factory function injection).
func TestIdentityExplicitGuard_FlagChangedVsEnvVar(t *testing.T) {
	t.Run("env var only does not mark flag as changed", func(t *testing.T) {
		viper.Reset()
		t.Setenv("ATMOS_IDENTITY", "some-identity")

		// Re-bind env after reset so viper can see the env var.
		require.NoError(t, viper.BindEnv(IdentityFlagName, "ATMOS_IDENTITY", "IDENTITY"))

		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().StringP("identity", "i", "", "")

		// Flag should NOT be marked as changed — only env var is set.
		assert.False(t, cmd.Flags().Changed(IdentityFlagName),
			"Flags().Changed must be false when identity comes from env var only")

		// GetIdentityFromFlags should still return the env var value (for when auth IS needed).
		identity := GetIdentityFromFlags(cmd, []string{"test"})
		assert.Equal(t, "some-identity", identity, "GetIdentityFromFlags should return env var value")
	})

	t.Run("explicit CLI flag marks flag as changed", func(t *testing.T) {
		viper.Reset()
		t.Setenv("ATMOS_IDENTITY", "")

		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().StringP("identity", "i", "", "")
		require.NoError(t, cmd.Flags().Set("identity", "explicit-identity"))

		assert.True(t, cmd.Flags().Changed(IdentityFlagName),
			"Flags().Changed must be true when --identity is explicitly set")

		identity := GetIdentityFromFlags(cmd, []string{"test", "--identity", "explicit-identity"})
		assert.Equal(t, "explicit-identity", identity)
	})

	t.Run("neither flag nor env var", func(t *testing.T) {
		viper.Reset()
		t.Setenv("ATMOS_IDENTITY", "")
		t.Setenv("IDENTITY", "")

		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().StringP("identity", "i", "", "")

		assert.False(t, cmd.Flags().Changed(IdentityFlagName),
			"Flags().Changed must be false when nothing is set")

		identity := GetIdentityFromFlags(cmd, []string{"test"})
		assert.Empty(t, identity, "GetIdentityFromFlags should return empty when nothing is set")
	})
}
