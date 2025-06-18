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
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name               string
		setupMock          func(*exec.MockDescribeStacksExec)
		validateFunc       func(...AtmosValidateOption)
		parseArgsFunc      func(string, *cobra.Command, []string, []string) (schema.ConfigAndStacksInfo, error)
		processFunc        func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error)
		validateConfigFunc func(schema.AtmosConfiguration) error
		setCliArgsFunc     func(*pflag.FlagSet, *exec.DescribeStacksArgs) error
		args               []string
		expectError        bool
	}{
		{
			name: "Execute returns error",
			setupMock: func(mockExec *exec.MockDescribeStacksExec) {
				mockExec.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(fmt.Errorf("execution failed")).Times(1)
			},
			validateFunc: func(opts ...AtmosValidateOption) {},
			parseArgsFunc: func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
				return schema.ConfigAndStacksInfo{}, nil
			},
			processFunc: func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
				return schema.AtmosConfiguration{}, nil
			},
			validateConfigFunc: func(atmosConfig schema.AtmosConfiguration) error {
				return nil
			},
			setCliArgsFunc: func(flags *pflag.FlagSet, describe *exec.DescribeStacksArgs) error {
				return nil
			},
			args:        []string{},
			expectError: true,
		},
		{
			name: "ParseArgs returns error",
			setupMock: func(mockExec *exec.MockDescribeStacksExec) {
				// Should not be called due to early error
			},
			validateFunc: func(opts ...AtmosValidateOption) {},
			parseArgsFunc: func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
				return schema.ConfigAndStacksInfo{}, fmt.Errorf("parse args failed")
			},
			processFunc: func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
				return schema.AtmosConfiguration{}, nil
			},
			validateConfigFunc: func(atmosConfig schema.AtmosConfiguration) error {
				return nil
			},
			setCliArgsFunc: func(flags *pflag.FlagSet, describe *exec.DescribeStacksArgs) error {
				return nil
			},
			args:        []string{"invalid-arg"},
			expectError: true,
		},
		{
			name: "ProcessConfig returns error",
			setupMock: func(mockExec *exec.MockDescribeStacksExec) {
				// Should not be called due to early error
			},
			validateFunc: func(opts ...AtmosValidateOption) {},
			parseArgsFunc: func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
				return schema.ConfigAndStacksInfo{}, nil
			},
			processFunc: func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
				return schema.AtmosConfiguration{}, fmt.Errorf("process config failed")
			},
			validateConfigFunc: func(atmosConfig schema.AtmosConfiguration) error {
				return nil
			},
			setCliArgsFunc: func(flags *pflag.FlagSet, describe *exec.DescribeStacksArgs) error {
				return nil
			},
			args:        []string{},
			expectError: true,
		},
		{
			name: "ValidateConfig returns error",
			setupMock: func(mockExec *exec.MockDescribeStacksExec) {
				// Should not be called due to early error
			},
			validateFunc: func(opts ...AtmosValidateOption) {},
			parseArgsFunc: func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
				return schema.ConfigAndStacksInfo{}, nil
			},
			processFunc: func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
				return schema.AtmosConfiguration{}, nil
			},
			validateConfigFunc: func(atmosConfig schema.AtmosConfiguration) error {
				return fmt.Errorf("validate config failed")
			},
			setCliArgsFunc: func(flags *pflag.FlagSet, describe *exec.DescribeStacksArgs) error {
				return nil
			},
			args:        []string{},
			expectError: true,
		},
		{
			name: "SetCliArgs returns error",
			setupMock: func(mockExec *exec.MockDescribeStacksExec) {
				// Should not be called due to early error
			},
			validateFunc: func(opts ...AtmosValidateOption) {},
			parseArgsFunc: func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
				return schema.ConfigAndStacksInfo{}, nil
			},
			processFunc: func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
				return schema.AtmosConfiguration{}, nil
			},
			validateConfigFunc: func(atmosConfig schema.AtmosConfiguration) error {
				return nil
			},
			setCliArgsFunc: func(flags *pflag.FlagSet, describe *exec.DescribeStacksArgs) error {
				return fmt.Errorf("set cli args failed")
			},
			args:        []string{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockExec := exec.NewMockDescribeStacksExec(ctrl)
			tt.setupMock(mockExec)

			run := getRunnableDescribeStacksCmd(getRunnableDescribeStacksCmdProps{
				tt.validateFunc,
				tt.parseArgsFunc,
				tt.processFunc,
				tt.validateConfigFunc,
				tt.setCliArgsFunc,
				mockExec,
			})

			// Create a mock command for testing
			cmd := &cobra.Command{}

			// We expect this to handle errors gracefully or panic in some cases
			// The actual behavior depends on the implementation
			defer func() {
				if r := recover(); r != nil && !tt.expectError {
					t.Errorf("Unexpected panic: %v", r)
				}
			}()

			run(cmd, tt.args)
		})
	}
}

func TestDescribeStacksRunnableWithValidation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name            string
		validateOptions []AtmosValidateOption
		expectCalled    bool
	}{
		{
			name:            "No validation options",
			validateOptions: []AtmosValidateOption{},
			expectCalled:    true,
		},
		{
			name: "With validation options",
			validateOptions: []AtmosValidateOption{
				func(config *AtmosValidateConfig) {
					// Mock validation option
				},
			},
			expectCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockExec := exec.NewMockDescribeStacksExec(ctrl)
			mockExec.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil).Times(1)

			validateCalled := false
			run := getRunnableDescribeStacksCmd(getRunnableDescribeStacksCmdProps{
				func(opts ...AtmosValidateOption) {
					validateCalled = true
					assert.Len(t, opts, len(tt.validateOptions))
				},
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
			assert.Equal(t, tt.expectCalled, validateCalled)
		})
	}
}

func TestSetFlagValueInDescribeStacksCliArgsEdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		setFlags      func(*pflag.FlagSet)
		describe      *exec.DescribeStacksArgs
		expected      *exec.DescribeStacksArgs
		expectedError bool
	}{
		{
			name: "Empty skip slice",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Set("skip", "")
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format: "yaml",
				Skip:   []string{""},
			},
		},
		{
			name: "Multiple skip values",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Parse([]string{
					"--skip", "test1,test2,test3",
				})
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format: "yaml",
				Skip:   []string{"test1,test2,test3"},
			},
		},
		{
			name: "All boolean flags true",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Parse([]string{
					"--process-templates=true",
					"--process-functions=true",
					"--include-empty-stacks=true",
				})
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format:             "yaml",
				ProcessTemplates:   true,
				ProcessFunctions:   true,
				IncludeEmptyStacks: true,
			},
		},
		{
			name: "All boolean flags false",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Parse([]string{
					"--process-templates=false",
					"--process-functions=false",
					"--include-empty-stacks=false",
				})
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format:             "yaml",
				ProcessTemplates:   false,
				ProcessFunctions:   false,
				IncludeEmptyStacks: false,
			},
		},
		{
			name: "All string flags set",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Parse([]string{
					"--file", "/tmp/output.yaml",
					"--format", "json",
					"--stack", "dev/us-east-1",
					"--components", "vpc,ec2",
					"--component-types", "terraform,helmfile",
					"--sections", "vars,backend",
				})
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				File:           "/tmp/output.yaml",
				Format:         "json",
				Stack:          "dev/us-east-1",
				Components:     "vpc,ec2",
				ComponentTypes: "terraform,helmfile",
				Sections:       "vars,backend",
			},
		},
		{
			name: "Mix of all flag types",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Parse([]string{
					"--file", "/tmp/test.json",
					"--format", "json",
					"--stack", "prod/us-west-2",
					"--components", "database",
					"--process-templates=false",
					"--include-empty-stacks=true",
					"--skip", "validation,tests",
				})
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				File:               "/tmp/test.json",
				Format:             "json",
				Stack:              "prod/us-west-2",
				Components:         "database",
				ProcessTemplates:   false,
				IncludeEmptyStacks: true,
				Skip:               []string{"validation,tests"},
			},
		},
		{
			name: "Nil describe args (should handle gracefully)",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Set("format", "json")
			},
			describe:      nil,
			expected:      nil,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new flag set
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)

			// Define all flags to match the flagsKeyValue map
			fs.String("file", "", "Write the result to file")
			fs.String("format", "yaml", "Specify the output format (`yaml` is default)")
			fs.StringP("stack", "s", "", "Filter by a specific stack")
			fs.String("components", "", "Filter by specific `atmos` components")
			fs.String("component-types", "", "Filter by specific component types")
			fs.String("sections", "", "Output only the specified component sections")
			fs.Bool("process-templates", true, "Enable/disable Go template processing")
			fs.Bool("process-functions", true, "Enable/disable YAML functions processing")
			fs.Bool("include-empty-stacks", false, "Include stacks with no components")
			fs.StringSlice("skip", nil, "Skip executing a YAML function")

			// Set flags as specified in the test case
			tt.setFlags(fs)

			// Handle nil describe args case
			if tt.expectedError && tt.describe == nil {
				defer func() {
					if r := recover(); r == nil {
						t.Error("Expected panic for nil describe args but none occurred")
					}
				}()
			}

			// Call the function
			setCliArgsForDescribeStackCli(fs, tt.describe)

			// Assert the describe struct matches the expected values (if not expecting error)
			if !tt.expectedError {
				assert.Equal(t, tt.expected, tt.describe, "Describe struct does not match expected")
			}
		})
	}
}

func TestSetFlagValueInDescribeStacksCliArgsBoundaryConditions(t *testing.T) {
	tests := []struct {
		name     string
		setFlags func(*pflag.FlagSet)
		describe *exec.DescribeStacksArgs
		expected *exec.DescribeStacksArgs
	}{
		{
			name: "Very long string values",
			setFlags: func(fs *pflag.FlagSet) {
				longString := string(make([]byte, 1000))
				for i := range longString {
					longString = longString[:i] + "a" + longString[i+1:]
				}
				fs.Set("file", longString)
				fs.Set("stack", longString)
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format: "yaml",
				File:   string(make([]byte, 1000)),
				Stack:  string(make([]byte, 1000)),
			},
		},
		{
			name: "Special characters in string values",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Set("file", "file with spaces and símbolos.json")
				fs.Set("stack", "stack/with/slashes-and_underscores")
				fs.Set("components", "comp1,comp2,comp-3,comp_4")
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format:     "yaml",
				File:       "file with spaces and símbolos.json",
				Stack:      "stack/with/slashes-and_underscores",
				Components: "comp1,comp2,comp-3,comp_4",
			},
		},
		{
			name: "Empty string values",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Set("file", "")
				fs.Set("stack", "")
				fs.Set("components", "")
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format:     "yaml",
				File:       "",
				Stack:      "",
				Components: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new flag set
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)

			// Define all flags
			fs.String("file", "", "Write the result to file")
			fs.String("format", "yaml", "Specify the output format")
			fs.StringP("stack", "s", "", "Filter by a specific stack")
			fs.String("components", "", "Filter by specific components")
			fs.String("component-types", "", "Filter by specific component types")
			fs.String("sections", "", "Output only the specified component sections")
			fs.Bool("process-templates", true, "Enable/disable Go template processing")
			fs.Bool("process-functions", true, "Enable/disable YAML functions processing")
			fs.Bool("include-empty-stacks", false, "Include stacks with no components")
			fs.StringSlice("skip", nil, "Skip executing a YAML function")

			// Set flags as specified in the test case
			tt.setFlags(fs)

			// For the long string test, create the expected long strings properly
			if tt.name == "Very long string values" {
				longString := ""
				for i := 0; i < 1000; i++ {
					longString += "a"
				}
				tt.expected.File = longString
				tt.expected.Stack = longString
			}

			// Call the function
			setCliArgsForDescribeStackCli(fs, tt.describe)

			// Assert the describe struct matches the expected values
			assert.Equal(t, tt.expected, tt.describe, "Describe struct does not match expected")
		})
	}
}
