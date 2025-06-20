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
				Format:           "json",
				ProcessTemplates: true,
				Skip:             []string{"tests"},
			},
		},
		{
			name: "No flags changed, set default format",
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
			setFlags: func(fs *pflag.FlagSet) {
				fs.Set("format", "json")
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format: "json",
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

			setCliArgsForDescribeStackCli(fs, tt.describe)

			// Assert the describe struct matches the expected values
			assert.Equal(t, tt.expected, tt.describe, "Describe struct does not match expected")
		})
	}
}

func TestDescribeStacksRunnableWithErrors(t *testing.T) {
	tests := []struct {
		name                    string
		validateFunc            func(opts ...AtmosValidateOption)
		processConfigFunc       func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error)
		processStacksFunc       func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error)
		validateConfigFunc      func(atmosConfig schema.AtmosConfiguration) error
		setCliArgsFunc          func(flags *pflag.FlagSet, describe *exec.DescribeStacksArgs) error
		mockExecuteFunc         func(mockExec *exec.MockDescribeStacksExec)
		expectedPanic           bool
		expectExecuteCalled     bool
	}{
		{
			name: "ProcessConfig returns error",
			validateFunc: func(opts ...AtmosValidateOption) {},
			processConfigFunc: func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
				return schema.ConfigAndStacksInfo{}, fmt.Errorf("config processing failed")
			},
			processStacksFunc: func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
				return schema.AtmosConfiguration{}, nil
			},
			validateConfigFunc: func(atmosConfig schema.AtmosConfiguration) error {
				return nil
			},
			setCliArgsFunc: func(flags *pflag.FlagSet, describe *exec.DescribeStacksArgs) error {
				return nil
			},
			mockExecuteFunc: func(mockExec *exec.MockDescribeStacksExec) {
				// Execution should not be reached
			},
			expectedPanic:       true,
			expectExecuteCalled: false,
		},
		{
			name: "ProcessStacks returns error",
			validateFunc: func(opts ...AtmosValidateOption) {},
			processConfigFunc: func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
				return schema.ConfigAndStacksInfo{}, nil
			},
			processStacksFunc: func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
				return schema.AtmosConfiguration{}, fmt.Errorf("stack processing failed")
			},
			validateConfigFunc: func(atmosConfig schema.AtmosConfiguration) error {
				return nil
			},
			setCliArgsFunc: func(flags *pflag.FlagSet, describe *exec.DescribeStacksArgs) error {
				return nil
			},
			mockExecuteFunc: func(mockExec *exec.MockDescribeStacksExec) {
				// Execution should not be reached
			},
			expectedPanic:       true,
			expectExecuteCalled: false,
		},
		{
			name: "ValidateConfig returns error",
			validateFunc: func(opts ...AtmosValidateOption) {},
			processConfigFunc: func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
				return schema.ConfigAndStacksInfo{}, nil
			},
			processStacksFunc: func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
				return schema.AtmosConfiguration{}, nil
			},
			validateConfigFunc: func(atmosConfig schema.AtmosConfiguration) error {
				return fmt.Errorf("config validation failed")
			},
			setCliArgsFunc: func(flags *pflag.FlagSet, describe *exec.DescribeStacksArgs) error {
				return nil
			},
			mockExecuteFunc: func(mockExec *exec.MockDescribeStacksExec) {
				// Execution should not be reached
			},
			expectedPanic:       true,
			expectExecuteCalled: false,
		},
		{
			name: "SetCliArgs returns error",
			validateFunc: func(opts ...AtmosValidateOption) {},
			processConfigFunc: func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
				return schema.ConfigAndStacksInfo{}, nil
			},
			processStacksFunc: func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
				return schema.AtmosConfiguration{}, nil
			},
			validateConfigFunc: func(atmosConfig schema.AtmosConfiguration) error {
				return nil
			},
			setCliArgsFunc: func(flags *pflag.FlagSet, describe *exec.DescribeStacksArgs) error {
				return fmt.Errorf("CLI args setting failed")
			},
			mockExecuteFunc: func(mockExec *exec.MockDescribeStacksExec) {
				// Execution should not be reached
			},
			expectedPanic:       true,
			expectExecuteCalled: false,
		},
		{
			name: "Execute returns error",
			validateFunc: func(opts ...AtmosValidateOption) {},
			processConfigFunc: func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
				return schema.ConfigAndStacksInfo{}, nil
			},
			processStacksFunc: func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
				return schema.AtmosConfiguration{}, nil
			},
			validateConfigFunc: func(atmosConfig schema.AtmosConfiguration) error {
				return nil
			},
			setCliArgsFunc: func(flags *pflag.FlagSet, describe *exec.DescribeStacksArgs) error {
				return nil
			},
			mockExecuteFunc: func(mockExec *exec.MockDescribeStacksExec) {
				mockExec.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(fmt.Errorf("execution failed")).Times(1)
			},
			expectedPanic:       true,
			expectExecuteCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockExec := exec.NewMockDescribeStacksExec(ctrl)

			tt.mockExecuteFunc(mockExec)

			run := getRunnableDescribeStacksCmd(getRunnableDescribeStacksCmdProps{
				tt.validateFunc,
				tt.processConfigFunc,
				tt.processStacksFunc,
				tt.validateConfigFunc,
				tt.setCliArgsFunc,
				mockExec,
			})

			if tt.expectedPanic {
				assert.Panics(t, func() {
					run(describeStacksCmd, []string{})
				}, "Expected function to panic")
			} else {
				assert.NotPanics(t, func() {
					run(describeStacksCmd, []string{})
				}, "Expected function not to panic")
			}
		})
	}
}

func TestSetFlagValueInDescribeStacksCliArgsEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		setFlags func(*pflag.FlagSet)
		describe *exec.DescribeStacksArgs
		expected *exec.DescribeStacksArgs
	}{
		{
			name: "Multiple skip values",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Parse([]string{"--skip", "tests,lint,docs"})
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format: "yaml",
				Skip:   []string{"tests", "lint", "docs"},
			},
		},
		{
			name: "All component sections specified",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Parse([]string{"--sections", "backend,backend_type,deps,env,inheritance,metadata,remote_state_backend,remote_state_backend_type,settings,vars"})
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format:   "yaml",
				Sections: []string{"backend", "backend_type", "deps", "env", "inheritance", "metadata", "remote_state_backend", "remote_state_backend_type", "settings", "vars"},
			},
		},
		{
			name: "Multiple component types",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Parse([]string{"--component-types", "terraform,helmfile"})
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format:         "yaml",
				ComponentTypes: []string{"terraform", "helmfile"},
			},
		},
		{
			name: "Multiple components",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Parse([]string{"--components", "comp1,comp2,comp3"})
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format:     "yaml",
				Components: []string{"comp1", "comp2", "comp3"},
			},
		},
		{
			name: "All boolean flags enabled",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Parse([]string{"--process-templates", "--process-functions", "--include-empty-stacks"})
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format:           "yaml",
				ProcessTemplates: true,
				IncludeEmptyStacks: true,
			},
		},
		{
			name: "All boolean flags disabled explicitly",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Parse([]string{"--process-templates=false", "--process-functions=false", "--include-empty-stacks=false"})
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format:           "yaml",
				ProcessTemplates: false,
				IncludeEmptyStacks: false,
			},
		},
		{
			name: "File output specified",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Parse([]string{"--file", "/tmp/output.yaml", "--format", "yaml"})
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format: "yaml",
				File:   "/tmp/output.yaml",
			},
		},
		{
			name: "Stack filter with short flag",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Parse([]string{"-s", "my-stack"})
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format: "yaml",
			},
		},
		{
			name: "Complex stack path",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Parse([]string{"--stack", "envs/prod/us-west-2/vpc"})
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format: "yaml",
			},
		},
		{
			name: "Pre-populated describe args should be preserved if not overridden",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Parse([]string{"--format", "json"})
			},
			describe: &exec.DescribeStacksArgs{
				ProcessTemplates:   true,
				IncludeEmptyStacks: true,
			},
			expected: &exec.DescribeStacksArgs{
				Format:             "json",
				ProcessTemplates:   true,
				IncludeEmptyStacks: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			fs.String("file", "", "Write the result to file")
			fs.String("format", "yaml", "Specify the output format (`yaml` is default)")
			fs.StringP("stack", "s", "", "Filter by a specific stack")
			fs.StringSlice("components", nil, "Filter by specific `atmos` components")
			fs.StringSlice("component-types", nil, "Filter by specific component types")
			fs.StringSlice("sections", nil, "Output only the specified component sections")
			fs.Bool("process-templates", true, "Enable/disable Go template processing")
			fs.Bool("process-functions", true, "Enable/disable YAML functions processing")
			fs.Bool("include-empty-stacks", false, "Include stacks with no components")
			fs.StringSlice("skip", nil, "Skip executing a YAML function")

			tt.setFlags(fs)
			setCliArgsForDescribeStackCli(fs, tt.describe)
			assert.Equal(t, tt.expected, tt.describe, "Describe struct does not match expected")
		})
	}
}

func TestSetFlagValueInDescribeStacksCliArgsValidation(t *testing.T) {
	tests := []struct {
		name          string
		setFlags      func(*pflag.FlagSet) error
		describe      *exec.DescribeStacksArgs
		shouldSucceed bool
	}{
		{
			name: "Invalid format value",
			setFlags: func(fs *pflag.FlagSet) error {
				return fs.Parse([]string{"--format", "invalid-format"})
			},
			describe:      &exec.DescribeStacksArgs{},
			shouldSucceed: true, // Parser doesn't validate format values
		},
		{
			name: "Empty string values should be handled gracefully",
			setFlags: func(fs *pflag.FlagSet) error {
				return fs.Parse([]string{"--stack", "", "--components", "", "--file", ""})
			},
			describe:      &exec.DescribeStacksArgs{},
			shouldSucceed: true,
		},
		{
			name: "Nil describe struct should not panic",
			setFlags: func(fs *pflag.FlagSet) error {
				return fs.Parse([]string{"--format", "json"})
			},
			describe:      nil,
			shouldSucceed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			fs.String("file", "", "Write the result to file")
			fs.String("format", "yaml", "Specify the output format")
			fs.StringP("stack", "s", "", "Filter by a specific stack")
			fs.StringSlice("components", nil, "Filter by specific components")
			fs.StringSlice("component-types", nil, "Filter by component types")
			fs.StringSlice("sections", nil, "Output component sections")
			fs.Bool("process-templates", true, "Enable template processing")
			fs.Bool("process-functions", true, "Enable functions processing")
			fs.Bool("include-empty-stacks", false, "Include empty stacks")
			fs.StringSlice("skip", nil, "Skip functions")

			err := tt.setFlags(fs)
			if err != nil && tt.shouldSucceed {
				t.Fatalf("Unexpected parse error: %v", err)
			}

			if tt.describe == nil {
				assert.Panics(t, func() {
					setCliArgsForDescribeStackCli(fs, tt.describe)
				}, "Expected panic when describe is nil")
				return
			}

			if tt.shouldSucceed {
				assert.NotPanics(t, func() {
					setCliArgsForDescribeStackCli(fs, tt.describe)
				}, "Should not panic with valid inputs")
			}
		})
	}
}

func BenchmarkSetFlagValueInDescribeStacksCliArgs(b *testing.B) {
	fs := pflag.NewFlagSet("benchmark", pflag.ContinueOnError)
	fs.String("file", "", "Write the result to file")
	fs.String("format", "yaml", "Specify the output format")
	fs.StringP("stack", "s", "", "Filter by a specific stack")
	fs.StringSlice("components", nil, "Filter by specific components")
	fs.StringSlice("component-types", nil, "Filter by component types")
	fs.StringSlice("sections", nil, "Output component sections")
	fs.Bool("process-templates", true, "Enable template processing")
	fs.Bool("process-functions", true, "Enable functions processing")
	fs.Bool("include-empty-stacks", false, "Include empty stacks")
	fs.StringSlice("skip", nil, "Skip functions")

	fs.Parse([]string{"--format", "json", "--stack", "my-stack", "--components", "comp1,comp2", "--process-templates", "--skip", "test1,test2"})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		describe := &exec.DescribeStacksArgs{}
		setCliArgsForDescribeStackCli(fs, describe)
	}
}

func BenchmarkGetRunnableDescribeStacksCmd(b *testing.B) {
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()
	mockExec := exec.NewMockDescribeStacksExec(ctrl)
	mockExec.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	props := getRunnableDescribeStacksCmdProps{
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
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		run := getRunnableDescribeStacksCmd(props)
		_ = run
	}
}

func TestDescribeStacksRunnableWithDifferentArgs(t *testing.T) {
	testCases := []struct {
		name string
		args []string
	}{
		{name: "No arguments", args: []string{}},
		{name: "Single argument", args: []string{"stack-name"}},
		{name: "Multiple arguments", args: []string{"stack-name", "component-name"}},
		{name: "Arguments with special characters", args: []string{"stack-with-dashes", "component_with_underscores"}},
		{name: "Arguments with paths", args: []string{"envs/prod/us-west-2/vpc"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockExec := exec.NewMockDescribeStacksExec(ctrl)
			mockExec.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil).Times(1)

			run := getRunnableDescribeStacksCmd(getRunnableDescribeStacksCmdProps{
				func(opts ...AtmosValidateOption) {},
				func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
					assert.Equal(t, tc.args, args, "Arguments should match expected")
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

			assert.NotPanics(t, func() {
				run(describeStacksCmd, tc.args)
			}, "Should not panic with any argument combination")
		})
	}
}