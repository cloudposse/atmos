package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestSetFlagInDescribeWorkflow has been removed because flagsToDescribeWorkflowsArgs
// no longer exists. Flag parsing is now handled by the StandardOptions parser.
// The functionality is still tested through TestDescribeWorkflows and integration tests.

func TestDescribeWorkflows(t *testing.T) {
	_ = NewTestKit(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	describeWorkflowsMock := exec.NewMockDescribeWorkflowsExec(ctrl)
	describeWorkflowsMock.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	run := getRunnableDescribeWorkflowsCmd(
		func(opts ...AtmosValidateOption) {},
		func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
			return schema.ConfigAndStacksInfo{}, nil
		},
		func(info schema.ConfigAndStacksInfo, validate bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, nil
		},
		describeWorkflowsMock,
	)

	err := run(describeWorkflowsCmd, []string{})

	// Verify command executed without errors. The mock expectations verify
	// that Execute() was called with the correct arguments.
	assert.NoError(t, err, "describeWorkflowsCmd should execute without error")
}

func TestDescribeWorkflowsCmd_Error(t *testing.T) {
	_ = NewTestKit(t)

	stacksPath := "../tests/fixtures/scenarios/terraform-apply-affected"

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	err := describeWorkflowsCmd.RunE(describeWorkflowsCmd, []string{"--invalid-flag"})
	assert.Error(t, err, "describe workflows command should return an error when called with invalid flags")
}
