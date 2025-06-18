package cmd

import (
	"fmt"
	"testing"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/golang/mock/gomock"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
)

func TestDescribeStacksRunnable(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockExec := exec.NewMockDescribeStacksExec(ctrl)
	mockExec.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	run := getRunnableDescribeStacksCmd(getRunnableDescribeStacksCmdProps{
		func(opts ...AtmosValidateOption) {},
		func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
			return schema.ConfigAndStacksInfo{}, nil
		},
		func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, nil
		},
		func(atmosConfig schema.AtmosConfiguration) error {
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
		initFlags     func(*pflag.FlagSet)
		setFlags      func(*pflag.FlagSet)
		describe      *exec.DescribeStacksArgs
		expected      *exec.DescribeStacksArgs
		expectedPanic bool
		panicMessage  string
	}{
		{
			name: "Set string and bool flags",
			initFlags: func(fs *pflag.FlagSet) {
				fs.String("file", "", "Write the result to file")
				fs.String("format", "yaml", "Specify the output format (`yaml` is default)")
				fs.StringP("stack", "s", "", "Filter by a specific stack\nThe filter supports names of the top-level stack manifests (including subfolder paths), and `atmos` stack names (derived from the context vars)")
				fs.StringSlice("components", nil, "Filter by specific `atmos` components")
				fs.StringSlice("component-types", nil, "Filter by specific component types. Supported component types: terraform, helmfile")
				fs.StringSlice("sections", nil, "Output only the specified component sections. Available component sections: `backend`, `backend_type`, `deps`, `env`, `inheritance`, `metadata`, `remote_state_backend`, `remote_state_backend_type`, `settings`, `vars`")
				fs.Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command")
				fs.Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command")
				fs.Bool("include-empty-stacks", false, "Include stacks with no components in the output")
				fs.StringSlice("skip", nil, "Skip executing a YAML function in the Atmos stack manifests when executing the command")
			},
			setFlags: func(fs *pflag.FlagSet) {
				fs.Parse([]string{
					"--format", "json",
					"--skip", "tests",
					"--process-templates",
				})
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format:           "json",
				ProcessTemplates: true,
				Skip:             []string{"tests"},
			},
		},
		{
			name: "No flags changed, set default format",
			initFlags: func(fs *pflag.FlagSet) {
				fs.String("file", "", "Write the result to file")
				fs.String("format", "yaml", "Specify the output format (`yaml` is default)")
				fs.StringP("stack", "s", "", "Filter by a specific stack\nThe filter supports names of the top-level stack manifests (including subfolder paths), and `atmos` stack names (derived from the context vars)")
				fs.StringSlice("components", nil, "Filter by specific `atmos` components")
				fs.StringSlice("component-types", nil, "Filter by specific component types. Supported component types: terraform, helmfile")
				fs.StringSlice("sections", nil, "Output only the specified component sections. Available component sections: `backend`, `backend_type`, `deps`, `env`, `inheritance`, `metadata`, `remote_state_backend`, `remote_state_backend_type`, `settings`, `vars`")
				fs.Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command")
				fs.Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command")
				fs.Bool("include-empty-stacks", false, "Include stacks with no components in the output")
				fs.StringSlice("skip", nil, "Skip executing a YAML function in the Atmos stack manifests when executing the command")
			},
			setFlags: func(fs *pflag.FlagSet) {
				// No flags set
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format: "yaml",
			},
		},
		{
			name: "Set format explicitly, no override",
			initFlags: func(fs *pflag.FlagSet) {
				fs.String("file", "", "Write the result to file")
				fs.String("format", "yaml", "Specify the output format (`yaml` is default)")
				fs.StringP("stack", "s", "", "Filter by a specific stack\nThe filter supports names of the top-level stack manifests (including subfolder paths), and `atmos` stack names (derived from the context vars)")
				fs.StringSlice("components", nil, "Filter by specific `atmos` components")
				fs.StringSlice("component-types", nil, "Filter by specific component types. Supported component types: terraform, helmfile")
				fs.StringSlice("sections", nil, "Output only the specified component sections. Available component sections: `backend`, `backend_type`, `deps`, `env`, `inheritance`, `metadata`, `remote_state_backend`, `remote_state_backend_type`, `settings`, `vars`")
				fs.Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command")
				fs.Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command")
				fs.Bool("include-empty-stacks", false, "Include stacks with no components in the output")
				fs.StringSlice("skip", nil, "Skip executing a YAML function in the Atmos stack manifests when executing the command")
			},
			setFlags: func(fs *pflag.FlagSet) {
				fs.Set("format", "json")
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format: "json",
			},
		},
		{
			name: "Parse comma separated slice flags",
			initFlags: func(fs *pflag.FlagSet) {
				fs.String("file", "", "Write the result to file")
				fs.String("format", "yaml", "Specify the output format (`yaml` is default)")
				fs.StringP("stack", "s", "", "Filter by a specific stack\nThe filter supports names of the top-level stack manifests (including subfolder paths), and `atmos` stack names (derived from the context vars)")
				fs.StringSlice("components", nil, "Filter by specific `atmos` components")
				fs.StringSlice("component-types", nil, "Filter by specific component types. Supported component types: terraform, helmfile")
				fs.StringSlice("sections", nil, "Output only the specified component sections. Available component sections: `backend`, `backend_type`, `deps`, `env`, `inheritance`, `metadata`, `remote_state_backend`, `remote_state_backend_type`, `settings`, `vars`")
				fs.Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command")
				fs.Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command")
				fs.Bool("include-empty-stacks", false, "Include stacks with no components in the output")
				fs.StringSlice("skip", nil, "Skip executing a YAML function in the Atmos stack manifests when executing the command")
			},
			setFlags: func(fs *pflag.FlagSet) {
				fs.Parse([]string{
					"--sections", "vars,metadata",
					"--components", "comp1,comp2",
					"--component-types", "terraform,helmfile",
				})
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Sections:       []string{"vars", "metadata"},
				Components:     []string{"comp1", "comp2"},
				ComponentTypes: []string{"terraform", "helmfile"},
				Format:         "yaml",
			},
		},
		{
			name: "Panic when slice flags defined as string",
			initFlags: func(fs *pflag.FlagSet) {
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
			},
			setFlags: func(fs *pflag.FlagSet) {
				fs.Parse([]string{"--sections", "vars"})
			},
			describe:      &exec.DescribeStacksArgs{},
			expected:      &exec.DescribeStacksArgs{},
			expectedPanic: true,
			panicMessage:  "trying to get stringSlice value of flag of type string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new flag set
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)

			// Initialize flags for this test case
			tt.initFlags(fs)

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

			setCliArgsForDescribeStackCli(fs, tt.describe)

			// Assert the describe struct matches the expected values
			assert.Equal(t, tt.expected, tt.describe, "Describe struct does not match expected")
		})
	}
}
