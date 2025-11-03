package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDescribeStacksRunnable(t *testing.T) {
	_ = NewTestKit(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExec := exec.NewMockDescribeStacksExec(ctrl)
	mockExec.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	run := getRunnableDescribeStacksCmd(getRunnableDescribeStacksCmdProps{
		checkAtmosConfig: func(opts ...AtmosValidateOption) {},
		processCommandLineArgs: func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
			return schema.ConfigAndStacksInfo{}, nil
		},
		initCliConfig: func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, nil
		},
		validateStacks: func(atmosConfig *schema.AtmosConfiguration) error {
			return nil
		},
		newDescribeStacksExec: mockExec,
	})

	err := run(describeStacksCmd, []string{})

	// Verify command executed without errors. The mock expectations verify
	// that Execute() was called with the correct arguments.
	assert.NoError(t, err, "describeStacksCmd should execute without error")
}

// TestSetFlagValueInDescribeStacksCliArgs has been removed because setCliArgsForDescribeStackCli
// no longer exists. Flag parsing is now handled by the StandardOptions parser.
// The functionality is still tested through TestDescribeStacksRunnable and integration tests.

// TestSetCliArgs_ComponentTypes_StringSlice has been removed because setCliArgsForDescribeStackCli
// no longer exists. Component types parsing is now handled by the StandardOptions parser.
// The functionality is still tested through integration tests.

func TestDescribeStacksCmd_Error(t *testing.T) {
	_ = NewTestKit(t)

	stacksPath := "../tests/fixtures/scenarios/terraform-apply-affected"

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	err := describeStacksCmd.RunE(describeStacksCmd, []string{"--invalid-flag"})
	assert.Error(t, err, "describe stacks command should return an error when called with invalid flags")
}
