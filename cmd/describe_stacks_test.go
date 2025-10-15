package cmd

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDescribeStacksRunnable(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockExec := exec.NewMockDescribeStacksExec(ctrl)
	mockExec.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	run := getRunnableDescribeStacksCmd(getRunnableDescribeStacksCmdProps{
		func(opts ...AtmosValidateOption) error {
			return nil
		},
		func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
			return schema.ConfigAndStacksInfo{}, nil
		},
		func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, nil
		},
		func(atmosConfig *schema.AtmosConfiguration) error {
			return nil
		},
		func(flags *pflag.FlagSet, describe *exec.DescribeStacksArgs) error {
			return nil
		},
		mockExec,
	})
	run(describeStacksCmd, []string{})
}

func TestSetFlagValueInDescribeStacksCliArgs(t *testing.T) {
	// Initialize test cases
	tests := []struct {
		name          string
		setFlags      func(*pflag.FlagSet)
		describe      *exec.DescribeStacksArgs
		expected      *exec.DescribeStacksArgs
		expectedPanic bool
		panicMessage  string
	}{
		{
			name: "Set string and bool flags",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Parse([]string{
					"--format", "json",
					"--skip", "tests",
					"--process-templates",
				})
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format:               "json",
				ProcessTemplates:     true,
				ProcessYamlFunctions: true,
				Skip:                 []string{"tests"},
			},
		},
		{
			name: "No flags changed, set default format",
			setFlags: func(fs *pflag.FlagSet) {
				// No flags set
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format:               "yaml",
				ProcessTemplates:     true,
				ProcessYamlFunctions: true,
			},
		},
		{
			name: "Set format explicitly, no override",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Set("format", "json")
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format:               "json",
				ProcessTemplates:     true,
				ProcessYamlFunctions: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new flag set
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)

			// Define all flags to match the flagsKeyValue map
			fs.String("file", "", "Write the result to file")
			fs.String("format", "yaml", "Specify the output format (`yaml` is default)")
			fs.StringP("stack", "s", "", "Filter by a specific stack\nThe filter supports names of the top-level stack manifests (including subfolder paths), and `atmos` stack names (derived from the context vars)")
			fs.String("components", "", "Filter by specific `atmos` components")
			fs.String("component-types", "", "Filter by specific component types. Supported component types: terraform, helmfile")
			fs.String("sections", "", "Output only the specified component sections. Available component sections: `backend`, `backend_type`, `deps`, `env`, `inheritance`, `metadata`, `remote_state_backend`, `remote_state_backend_type`, `settings`, `vars`")
			fs.Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command")
			fs.Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command")
			fs.Bool("include-empty-stacks", false, "Include stacks with no components in the output")
			fs.StringSlice("skip", nil, "Skip executing a YAML function in the Atmos stack manifests when executing the command")

			// Set flags as specified in the test case
			tt.setFlags(fs)

			// Call the function
			if tt.expectedPanic {
				defer func() {
					if r := recover(); r != nil {
						if fmt.Sprintf("%v", r) != tt.panicMessage {
							t.Errorf("Expected panic message %q, got %v", tt.panicMessage, r)
						}
					} else {
						t.Error("Expected panic but none occurred")
					}
				}()
			}

			err := setCliArgsForDescribeStackCli(fs, tt.describe)
			assert.NoError(t, err)

			// Assert the struct matches the expected values
			assert.Equal(t, tt.expected, tt.describe, "Describe struct does not match expected")
		})
	}
}

func TestSetCliArgs_ComponentTypes_StringSlice(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	// Define only the flags we plan to change
	fs.StringSlice("component-types", nil, "Filter by specific component types")

	// Provide slice-style input (CSV is supported by pflag for StringSlice)
	err := fs.Parse([]string{"--component-types=terraform,helmfile"})
	assert.NoError(t, err)

	args := &exec.DescribeStacksArgs{}
	err = setCliArgsForDescribeStackCli(fs, args)
	assert.NoError(t, err)
	assert.Equal(t, []string{"terraform", "helmfile"}, args.ComponentTypes)
}

func TestDescribeStacksCmd_Error(t *testing.T) {
	stacksPath := "../tests/fixtures/scenarios/terraform-apply-affected"

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	err := describeStacksCmd.RunE(describeStacksCmd, []string{"--invalid-flag"})
	assert.Error(t, err, "describe stacks command should return an error when called with invalid flags")
}
