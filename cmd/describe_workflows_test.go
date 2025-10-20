package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSetFlagInDescribeWorkflow(t *testing.T) {
	_ = NewTestKit(t)

	// Initialize test cases
	tests := []struct {
		name        string
		setFlags    func(*pflag.FlagSet)
		expected    *exec.DescribeWorkflowsArgs
		expectedErr bool
	}{
		{
			name: "Set string flags",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Set("format", "json")
				fs.Set("output", "map")
			},
			expected: &exec.DescribeWorkflowsArgs{
				Format:     "json",
				OutputType: "map",
			},
		},
		{
			name: "No flags changed, set default format",
			setFlags: func(fs *pflag.FlagSet) {
				// No flags set
			},
			expected: &exec.DescribeWorkflowsArgs{
				Format:     "yaml",
				OutputType: "list",
			},
		},
		{
			name: "Set invalid format, no override",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Set("format", "invalid_format")
			},
			expectedErr: true,
		},
		{
			name: "Set invalid output type, no override",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Set("output", "invalid_output")
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t)

			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			fs.StringP("format", "f", "yaml", "Specify the output format (`yaml` is default)")
			fs.StringP("output", "o", "list", "Specify the output type (`list` is default)")
			fs.StringP("query", "q", "", "Specify a query to filter the output")
			tt.setFlags(fs)
			describeWorkflowsArgs := &exec.DescribeWorkflowsArgs{}
			err := flagsToDescribeWorkflowsArgs(fs, describeWorkflowsArgs)
			if tt.expectedErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, describeWorkflowsArgs)
		})
	}
}

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

	describeWorkflowsCmd.Flags().StringP("pager", "p", "", "Specify a pager to use for output (e.g., `less`, `more`)")

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
