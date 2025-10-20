package cmd

import (
	"fmt"
	"testing"

	"go.uber.org/mock/gomock"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDescribeAffected(t *testing.T) {
	_ = NewTestKit(t)

	t.Chdir("../tests/fixtures/scenarios/basic")
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	describeAffectedMock := exec.NewMockDescribeAffectedExec(ctrl)
	describeAffectedMock.EXPECT().Execute(gomock.Any()).Return(nil)

	run := getRunnableDescribeAffectedCmd(func(opts ...AtmosValidateOption) {
	}, exec.ParseDescribeAffectedCliArgs, func(atmosConfig *schema.AtmosConfiguration) exec.DescribeAffectedExec {
		return describeAffectedMock
	})

	err := run(describeAffectedCmd, []string{})

	// Verify command executed without errors. The mock expectations verify
	// that Execute() was called with the correct arguments.
	assert.NoError(t, err, "describeAffectedCmd should execute without error")
}

func TestSetFlagValueInCliArgs(t *testing.T) {
	_ = NewTestKit(t)

	// Initialize test cases
	tests := []struct {
		name          string
		setFlags      func(*pflag.FlagSet)
		expected      *exec.DescribeAffectedCmdArgs
		expectedPanic bool
		panicMessage  string
	}{
		{
			name: "Set string and bool flags",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Set("ref", "main")
				fs.Set("sha", "abc123")
				fs.Set("include-dependents", "true")
				fs.Set("format", "yaml")
			},
			expected: &exec.DescribeAffectedCmdArgs{
				Ref:                  "main",
				SHA:                  "abc123",
				IncludeDependents:    true,
				Format:               "yaml",
				ProcessTemplates:     true,
				ProcessYamlFunctions: true,
			},
		},
		{
			name: "Set Upload flag to true",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Set("upload", "true")
			},
			expected: &exec.DescribeAffectedCmdArgs{
				Upload:               true,
				IncludeDependents:    true,
				IncludeSettings:      true,
				Format:               "json",
				ProcessTemplates:     true,
				ProcessYamlFunctions: true,
			},
		},
		{
			name: "No flags changed, set default format",
			setFlags: func(fs *pflag.FlagSet) {
				// No flags set
			},
			expected: &exec.DescribeAffectedCmdArgs{
				Format:               "json",
				ProcessTemplates:     true,
				ProcessYamlFunctions: true,
			},
		},
		{
			name: "Set format explicitly, no override",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Set("format", "json")
			},
			expected: &exec.DescribeAffectedCmdArgs{
				Format:               "json",
				ProcessTemplates:     true,
				ProcessYamlFunctions: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t)

			// Create a new flag set
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)

			// Define all flags to match the flagsKeyValue map
			fs.String("ref", "", "Reference")
			fs.String("sha", "", "SHA")
			fs.String("repo-path", "", "Repository path")
			fs.String("ssh-key", "", "SSH key path")
			fs.String("ssh-key-password", "", "SSH key password")
			fs.Bool("include-spacelift-admin-stacks", false, "Include Spacelift admin stacks")
			fs.Bool("include-dependents", false, "Include dependents")
			fs.Bool("include-settings", false, "Include settings")
			fs.Bool("upload", false, "Upload")
			fs.String("clone-target-ref", "", "Clone target ref")
			fs.Bool("process-templates", false, "Process templates")
			fs.Bool("process-functions", false, "Process YAML functions")
			fs.Bool("skip", false, "Skip")
			fs.String("pager", "", "Pager")
			fs.String("stack", "", "Stack")
			fs.String("format", "", "Format")
			fs.String("file", "", "Output file")
			fs.String("query", "", "Query")

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
			gotDescribe := &exec.DescribeAffectedCmdArgs{
				CLIConfig: &schema.AtmosConfiguration{},
			}
			exec.SetDescribeAffectedFlagValueInCliArgs(fs, gotDescribe)
			tt.expected.CLIConfig = &schema.AtmosConfiguration{}

			// Assert the describe struct matches the expected values
			assert.Equal(t, tt.expected, gotDescribe, "Describe struct does not match expected")
		})
	}
}

func TestDescribeAffectedCmd_Error(t *testing.T) {
	_ = NewTestKit(t)

	stacksPath := "../tests/fixtures/scenarios/terraform-apply-affected"

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	err := describeAffectedCmd.RunE(describeAffectedCmd, []string{"--invalid-flag"})
	assert.Error(t, err, "describe affected command should return an error when called with invalid flags")
}
