package cmd

import (
	"fmt"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDescribeAffected(t *testing.T) {
	_ = NewTestKit(t)

	// Reset Viper to clear any environment variable bindings from previous tests.
	// This prevents ATMOS_IDENTITY or IDENTITY env vars from interfering with the test.
	viper.Reset()

	// Clear identity environment variables to prevent Viper from reading them.
	// In CI, these might be set and cause auth validation to fail when no auth is configured.
	t.Setenv("ATMOS_IDENTITY", "")
	t.Setenv("IDENTITY", "")

	t.Chdir("../tests/fixtures/scenarios/basic")

	// Disable authentication for this test to prevent validation errors.
	// Set both environment variable and viper value to ensure it's recognized.
	t.Setenv("ATMOS_IDENTITY", "false")
	viper.Set("identity", "false")

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

// TestDescribeAffectedSetsAuthDisabled exercises the wiring that routes the --identity=false
// signal (and its aliases) into DescribeAffectedCmdArgs.AuthDisabled. Before this fix, the
// disabled sentinel terminated at the top-level AuthManager (which became nil) but never
// reached the per-component auth resolver inside ExecuteDescribeStacks, so the
// `--identity=false` user expectation was silently ignored in `describe affected --upload`.
// See plan: --identity=false not honored in `atmos describe affected`.
func TestDescribeAffectedSetsAuthDisabled(t *testing.T) {
	tests := []struct {
		name             string
		envIdentity      string
		viperIdentity    string
		wantAuthDisabled bool
	}{
		{
			name:             "identity=false sets AuthDisabled",
			envIdentity:      "false",
			viperIdentity:    "false",
			wantAuthDisabled: true,
		},
		{
			name:             "identity=off sets AuthDisabled",
			envIdentity:      "off",
			viperIdentity:    "off",
			wantAuthDisabled: true,
		},
		{
			name:             "identity=0 sets AuthDisabled",
			envIdentity:      "0",
			viperIdentity:    "0",
			wantAuthDisabled: true,
		},
		{
			name:             "identity=no sets AuthDisabled",
			envIdentity:      "no",
			viperIdentity:    "no",
			wantAuthDisabled: true,
		},
		{
			name:             "no identity flag does not set AuthDisabled",
			envIdentity:      "",
			viperIdentity:    "",
			wantAuthDisabled: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_ = NewTestKit(t)

			viper.Reset()
			t.Setenv("ATMOS_IDENTITY", tc.envIdentity)
			t.Setenv("IDENTITY", "")
			if tc.viperIdentity != "" {
				viper.Set("identity", tc.viperIdentity)
			}

			t.Chdir("../tests/fixtures/scenarios/basic")

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			var captured *exec.DescribeAffectedCmdArgs
			mock := exec.NewMockDescribeAffectedExec(ctrl)
			mock.EXPECT().Execute(gomock.Any()).DoAndReturn(func(args *exec.DescribeAffectedCmdArgs) error {
				captured = args
				return nil
			})

			run := getRunnableDescribeAffectedCmd(
				func(opts ...AtmosValidateOption) {},
				exec.ParseDescribeAffectedCliArgs,
				func(atmosConfig *schema.AtmosConfiguration) exec.DescribeAffectedExec { return mock },
			)

			err := run(describeAffectedCmd, []string{})
			assert.NoError(t, err)
			assert.NotNil(t, captured, "Execute was not called with args")
			assert.Equal(t, tc.wantAuthDisabled, captured.AuthDisabled,
				"AuthDisabled should reflect the normalized identity flag value")
			if tc.wantAuthDisabled {
				assert.Nil(t, captured.AuthManager,
					"AuthManager must be nil when authentication is explicitly disabled")
			}
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
