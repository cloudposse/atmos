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
		name                     string
		validateFunc             func(opts ...AtmosValidateOption)
		processStacksFunc        func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error)
		processConfigFunc        func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error)
		validateConfigFunc       func(atmosConfig schema.AtmosConfiguration) error
		setCliArgsFunc           func(flags *pflag.FlagSet, describe *exec.DescribeStacksArgs) error
		mockExecSetup            func(*exec.MockDescribeStacksExec)
		expectedError            bool
		expectedExecuteCalls     int
	}{
		{
			name: "ProcessStacks returns error",
			validateFunc: func(opts ...AtmosValidateOption) {},
			processStacksFunc: func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
				return schema.ConfigAndStacksInfo{}, fmt.Errorf("process stacks error")
			},
			processConfigFunc: func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
				return schema.AtmosConfiguration{}, nil
			},
			validateConfigFunc: func(atmosConfig schema.AtmosConfiguration) error {
				return nil
			},
			setCliArgsFunc: func(flags *pflag.FlagSet, describe *exec.DescribeStacksArgs) error {
				return nil
			},
			mockExecSetup: func(mockExec *exec.MockDescribeStacksExec) {
				// No expectations since execution should not reach mockExec
			},
			expectedError:        false,
			expectedExecuteCalls: 0,
		},
		{
			name: "ProcessConfig returns error",
			validateFunc: func(opts ...AtmosValidateOption) {},
			processStacksFunc: func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
				return schema.ConfigAndStacksInfo{}, nil
			},
			processConfigFunc: func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
				return schema.AtmosConfiguration{}, fmt.Errorf("process config error")
			},
			validateConfigFunc: func(atmosConfig schema.AtmosConfiguration) error {
				return nil
			},
			setCliArgsFunc: func(flags *pflag.FlagSet, describe *exec.DescribeStacksArgs) error {
				return nil
			},
			mockExecSetup: func(mockExec *exec.MockDescribeStacksExec) {
				// No expectations since execution should not reach mockExec
			},
			expectedError:        false,
			expectedExecuteCalls: 0,
		},
		{
			name: "ValidateConfig returns error",
			validateFunc: func(opts ...AtmosValidateOption) {},
			processStacksFunc: func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
				return schema.ConfigAndStacksInfo{}, nil
			},
			processConfigFunc: func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
				return schema.AtmosConfiguration{}, nil
			},
			validateConfigFunc: func(atmosConfig schema.AtmosConfiguration) error {
				return fmt.Errorf("validate config error")
			},
			setCliArgsFunc: func(flags *pflag.FlagSet, describe *exec.DescribeStacksArgs) error {
				return nil
			},
			mockExecSetup: func(mockExec *exec.MockDescribeStacksExec) {
				// No expectations since execution should not reach mockExec
			},
			expectedError:        false,
			expectedExecuteCalls: 0,
		},
		{
			name: "SetCliArgs returns error",
			validateFunc: func(opts ...AtmosValidateOption) {},
			processStacksFunc: func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
				return schema.ConfigAndStacksInfo{}, nil
			},
			processConfigFunc: func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
				return schema.AtmosConfiguration{}, nil
			},
			validateConfigFunc: func(atmosConfig schema.AtmosConfiguration) error {
				return nil
			},
			setCliArgsFunc: func(flags *pflag.FlagSet, describe *exec.DescribeStacksArgs) error {
				return fmt.Errorf("set CLI args error")
			},
			mockExecSetup: func(mockExec *exec.MockDescribeStacksExec) {
				// No expectations since execution should not reach mockExec
			},
			expectedError:        false,
			expectedExecuteCalls: 0,
		},
		{
			name: "MockExec Execute returns error",
			validateFunc: func(opts ...AtmosValidateOption) {},
			processStacksFunc: func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
				return schema.ConfigAndStacksInfo{}, nil
			},
			processConfigFunc: func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
				return schema.AtmosConfiguration{}, nil
			},
			validateConfigFunc: func(atmosConfig schema.AtmosConfiguration) error {
				return nil
			},
			setCliArgsFunc: func(flags *pflag.FlagSet, describe *exec.DescribeStacksArgs) error {
				return nil
			},
			mockExecSetup: func(mockExec *exec.MockDescribeStacksExec) {
				mockExec.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(fmt.Errorf("execution error")).Times(1)
			},
			expectedError:        false,
			expectedExecuteCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockExec := exec.NewMockDescribeStacksExec(ctrl)
			tt.mockExecSetup(mockExec)

			run := getRunnableDescribeStacksCmd(getRunnableDescribeStacksCmdProps{
				tt.validateFunc,
				tt.processStacksFunc,
				tt.processConfigFunc,
				tt.validateConfigFunc,
				tt.setCliArgsFunc,
				mockExec,
			})

			// Test should not panic regardless of internal errors
			assert.NotPanics(t, func() {
				run(describeStacksCmd, []string{})
			})
		})
	}
}

func TestSetFlagValueInDescribeStacksCliArgsEdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		setFlags     func(*pflag.FlagSet)
		describe     *exec.DescribeStacksArgs
		expected     *exec.DescribeStacksArgs
	}{
		{
			name: "All flags set with multiple values",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Parse([]string{
					"--format", "json",
					"--file", "/path/to/output.json",
					"--stack", "my-stack",
					"--components", "component1,component2",
					"--component-types", "terraform,helmfile",
					"--sections", "backend,vars,metadata",
					"--process-templates=false",
					"--process-functions=false",
					"--include-empty-stacks",
					"--skip", "function1,function2",
				})
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format:               "json",
				File:                 "/path/to/output.json",
				FilterByStack:        "my-stack",
				Components:           []string{"component1", "component2"},
				ComponentTypes:       []string{"terraform", "helmfile"},
				Sections:             []string{"backend", "vars", "metadata"},
				ProcessTemplates:     false,
				ProcessYamlFunctions: false,
				IncludeEmptyStacks:   true,
				Skip:                 []string{"function1", "function2"},
			},
		},
		{
			name: "Empty string values",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Parse([]string{
					"--format", "",
					"--file", "",
					"--stack", "",
				})
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format:        "yaml",
				File:          "",
				FilterByStack: "",
			},
		},
		{
			name: "Boolean flags with explicit false values",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Parse([]string{
					"--process-templates=false",
					"--process-functions=false",
					"--include-empty-stacks=false",
				})
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format:              "yaml",
				ProcessTemplates:    false,
				ProcessYamlFunctions: false,
				IncludeEmptyStacks:  false,
			},
		},
		{
			name: "Mixed comma-separated and individual flag values",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Parse([]string{
					"--skip", "func1,func2",
					"--skip", "func3",
				})
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format: "yaml",
				Skip:   []string{"func1", "func2", "func3"},
			},
		},
		{
			name: "Pre-existing values in describe struct should be overwritten",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Parse([]string{
					"--format", "json",
					"--stack", "new-stack",
				})
			},
			describe: &exec.DescribeStacksArgs{
				Format:         "yaml",
				FilterByStack:  "old-stack",
				File:           "existing-file.yaml",
			},
			expected: &exec.DescribeStacksArgs{
				Format:         "json",
				FilterByStack:  "new-stack",
				File:           "existing-file.yaml",
			},
		},
		{
			name: "Query flag processing",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Parse([]string{
					"--query", ".stacks | keys",
				})
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format: "yaml",
				Query:  ".stacks | keys",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)

			fs.String("file", "", "Write the result to file")
			fs.String("format", "yaml", "Specify the output format")
			fs.StringP("stack", "s", "", "Filter by a specific stack")
			fs.String("components", "", "Filter by specific components")
			fs.String("component-types", "", "Filter by specific component types")
			fs.StringSlice("sections", nil, "Output only the specified component sections")
			fs.Bool("process-templates", true, "Enable/disable Go template processing")
			fs.Bool("process-functions", true, "Enable/disable YAML functions processing")
			fs.Bool("include-empty-stacks", false, "Include stacks with no components")
			fs.StringSlice("skip", nil, "Skip executing a YAML function")
			fs.String("query", "", "Query expression for filtering output")

			tt.setFlags(fs)
			err := setCliArgsForDescribeStackCli(fs, tt.describe)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, tt.describe)
		})
	}
}

func TestSetFlagValueInDescribeStacksCliArgsValidation(t *testing.T) {
	tests := []struct {
		name        string
		setupFlags  func() *pflag.FlagSet
		describe    *exec.DescribeStacksArgs
		expectError bool
		expectPanic bool
	}{
		{
			name: "Nil describe struct should panic",
			setupFlags: func() *pflag.FlagSet {
				fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
				fs.String("format", "yaml", "Format")
				return fs
			},
			describe:    nil,
			expectPanic: true,
		},
		{
			name: "Empty flag set",
			setupFlags: func() *pflag.FlagSet {
				return pflag.NewFlagSet("test", pflag.ContinueOnError)
			},
			describe:    &exec.DescribeStacksArgs{},
			expectPanic: false,
		},
		{
			name: "Invalid format should return error",
			setupFlags: func() *pflag.FlagSet {
				fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
				fs.String("format", "yaml", "Format")
				fs.Set("format", "invalid-format")
				return fs
			},
			describe:    &exec.DescribeStacksArgs{},
			expectError: true,
			expectPanic: false,
		},
		{
			name: "Valid json format should not return error",
			setupFlags: func() *pflag.FlagSet {
				fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
				fs.String("format", "yaml", "Format")
				fs.Set("format", "json")
				return fs
			},
			describe:    &exec.DescribeStacksArgs{},
			expectError: false,
			expectPanic: false,
		},
		{
			name: "Valid yaml format should not return error",
			setupFlags: func() *pflag.FlagSet {
				fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
				fs.String("format", "yaml", "Format")
				fs.Set("format", "yaml")
				return fs
			},
			describe:    &exec.DescribeStacksArgs{},
			expectError: false,
			expectPanic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := tt.setupFlags()
			if tt.expectPanic {
				assert.Panics(t, func() {
					setCliArgsForDescribeStackCli(fs, tt.describe)
				})
			} else {
				var err error
				assert.NotPanics(t, func() {
					err = setCliArgsForDescribeStackCli(fs, tt.describe)
				})
				if tt.expectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			}
		})
	}
}

func TestDescribeStacksRunnableWithDifferentArgs(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectedCalls int
		setupMockExec func(*exec.MockDescribeStacksExec)
	}{
		{
			name: "No arguments",
			args: []string{},
			expectedCalls: 1,
			setupMockExec: func(mockExec *exec.MockDescribeStacksExec) {
				mockExec.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
		},
		{
			name: "With single argument",
			args: []string{"stack-name"},
			expectedCalls: 1,
			setupMockExec: func(mockExec *exec.MockDescribeStacksExec) {
				mockExec.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
		},
		{
			name: "With multiple arguments",
			args: []string{"stack-name", "component-name", "extra-arg"},
			expectedCalls: 1,
			setupMockExec: func(mockExec *exec.MockDescribeStacksExec) {
				mockExec.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
		},
		{
			name: "With empty string arguments",
			args: []string{"", ""},
			expectedCalls: 1,
			setupMockExec: func(mockExec *exec.MockDescribeStacksExec) {
				mockExec.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockExec := exec.NewMockDescribeStacksExec(ctrl)
			tt.setupMockExec(mockExec)

			run := getRunnableDescribeStacksCmd(getRunnableDescribeStacksCmdProps{
				func(opts ...AtmosValidateOption) {},
				func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
					assert.Equal(t, tt.args, args)
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
				run(describeStacksCmd, tt.args)
			})
		})
	}
}

func TestDescribeStacksCompleteFlow(t *testing.T) {
	tests := []struct {
		name                 string
		configAndStacksInfo  schema.ConfigAndStacksInfo
		atmosConfiguration   schema.AtmosConfiguration
		expectedDescribeArgs func() *exec.DescribeStacksArgs
		setupCommand         func(*cobra.Command)
		verifyInteractions   func(*testing.T, *exec.DescribeStacksArgs, *schema.AtmosConfiguration)
	}{
		{
			name: "Complete successful flow with realistic config",
			configAndStacksInfo: schema.ConfigAndStacksInfo{
				ComponentFromArg: "test-component",
				Stack:            "test-stack",
			},
			atmosConfiguration: schema.AtmosConfiguration{
				Stacks: schema.Stacks{
					BasePath:      "stacks",
					IncludedPaths: []string{"**/*"},
				},
			},
			expectedDescribeArgs: func() *exec.DescribeStacksArgs {
				return &exec.DescribeStacksArgs{
					Format:              "yaml",
					ProcessTemplates:    true,
					ProcessYamlFunctions: true,
				}
			},
			setupCommand: func(cmd *cobra.Command) {
				cmd.Flags().Set("format", "yaml")
			},
			verifyInteractions: func(t *testing.T, args *exec.DescribeStacksArgs, config *schema.AtmosConfiguration) {
				assert.Equal(t, "yaml", args.Format)
				assert.True(t, args.ProcessTemplates)
				assert.True(t, args.ProcessYamlFunctions)
				assert.Equal(t, "stacks", config.Stacks.BasePath)
			},
		},
		{
			name: "Flow with custom format and filtering",
			configAndStacksInfo: schema.ConfigAndStacksInfo{
				ComponentFromArg: "custom-component",
				Stack:            "custom-stack",
			},
			atmosConfiguration: schema.AtmosConfiguration{
				Stacks: schema.Stacks{
					BasePath: "custom-stacks",
				},
			},
			expectedDescribeArgs: func() *exec.DescribeStacksArgs {
				return &exec.DescribeStacksArgs{
					Format:        "json",
					FilterByStack: "custom-stack",
					Components:    []string{"custom-component"},
				}
			},
			setupCommand: func(cmd *cobra.Command) {
				cmd.Flags().Set("format", "json")
				cmd.Flags().Set("stack", "custom-stack")
				cmd.Flags().Set("components", "custom-component")
			},
			verifyInteractions: func(t *testing.T, args *exec.DescribeStacksArgs, config *schema.AtmosConfiguration) {
				assert.Equal(t, "json", args.Format)
				assert.Equal(t, "custom-stack", args.FilterByStack)
				assert.Contains(t, args.Components, "custom-component")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockExec := exec.NewMockDescribeStacksExec(ctrl)

			var capturedArgs *exec.DescribeStacksArgs
			var capturedConfig *schema.AtmosConfiguration
			mockExec.EXPECT().Execute(gomock.Any(), gomock.Any()).DoAndReturn(
				func(atmosConfig *schema.AtmosConfiguration, args *exec.DescribeStacksArgs) error {
					capturedArgs = args
					capturedConfig = atmosConfig
					return nil
				},
			).Times(1)

			run := getRunnableDescribeStacksCmd(getRunnableDescribeStacksCmdProps{
				func(opts ...AtmosValidateOption) {},
				func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
					return tt.configAndStacksInfo, nil
				},
				func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
					return tt.atmosConfiguration, nil
				},
				func(atmosConfig schema.AtmosConfiguration) error {
					return nil
				},
				func(flags *pflag.FlagSet, describe *exec.DescribeStacksArgs) error {
					return setCliArgsForDescribeStackCli(flags, describe)
				},
				mockExec,
			})

			cmd := &cobra.Command{}
			cmd.Flags().String("format", "yaml", "Output format")
			cmd.Flags().String("stack", "", "Stack filter")
			cmd.Flags().String("components", "", "Components filter")
			cmd.Flags().String("file", "", "Output file")
			cmd.Flags().Bool("process-templates", true, "Process templates")
			cmd.Flags().Bool("process-functions", true, "Process functions")
			cmd.Flags().Bool("include-empty-stacks", false, "Include empty stacks")
			cmd.Flags().StringSlice("skip", nil, "Skip functions")
			cmd.Flags().String("query", "", "Query expression")
			cmd.Flags().String("component-types", "", "Component types")
			cmd.Flags().StringSlice("sections", nil, "Sections")

			if tt.setupCommand != nil {
				tt.setupCommand(cmd)
			}

			assert.NotPanics(t, func() {
				run(cmd, []string{})
			})

			if tt.verifyInteractions != nil {
				tt.verifyInteractions(t, capturedArgs, capturedConfig)
			}
		})
	}
}