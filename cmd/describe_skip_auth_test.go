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
