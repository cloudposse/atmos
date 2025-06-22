package cmd

import (
	"fmt"
	"testing"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/golang/mock/gomock"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
)

func TestDescribeAffected(t *testing.T) {
	t.Chdir("../tests/fixtures/scenarios/basic")
	ctrl := gomock.NewController(t)
	describeAffectedMock := exec.NewMockDescribeAffectedExec(ctrl)
	describeAffectedMock.EXPECT().Execute(gomock.Any()).Return(nil)
	run := getRunnableDescribeAffectedCmd(func(opts ...AtmosValidateOption) {
	}, parseDescribeAffectedCliArgs, func(atmosConfig *schema.AtmosConfiguration) exec.DescribeAffectedExec {
		return describeAffectedMock
	})
	run(describeAffectedCmd, []string{})
}

func TestSetFlagValueInCliArgs(t *testing.T) {
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
				Ref:               "main",
				SHA:               "abc123",
				IncludeDependents: true,
				Format:            "yaml",
			},
		},
		{
			name: "Set Upload flag to true",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Set("upload", "true")
			},
			expected: &exec.DescribeAffectedCmdArgs{
				Upload:            true,
				IncludeDependents: true,
				IncludeSettings:   true,
				Format:            "json",
			},
		},
		{
			name: "No flags changed, set default format",
			setFlags: func(fs *pflag.FlagSet) {
				// No flags set
			},
			expected: &exec.DescribeAffectedCmdArgs{
				Format: "json",
			},
		},
		{
			name: "Set format explicitly, no override",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Set("format", "json")
			},
			expected: &exec.DescribeAffectedCmdArgs{
				Format: "json",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
			setDescribeAffectedFlagValueInCliArgs(fs, gotDescribe)
			tt.expected.CLIConfig = &schema.AtmosConfiguration{}

			// Assert the describe struct matches the expected values
			assert.Equal(t, tt.expected, gotDescribe, "Describe struct does not match expected")
		})
	}
}

func TestSetDescribeAffectedFlagValueInCliArgs(t *testing.T) {
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
				Ref:               "main",
				SHA:               "abc123",
				IncludeDependents: true,
				Format:            "yaml",
			},
		},
		{
			name: "Set Upload flag to true",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Set("upload", "true")
			},
			expected: &exec.DescribeAffectedCmdArgs{
				Upload:            true,
				IncludeDependents: true,
				IncludeSettings:   true,
				Format:            "json",
			},
		},
		{
			name: "Set selector flag",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Set("selector", "env=prod")
			},
			expected: &exec.DescribeAffectedCmdArgs{
				Selector: "env=prod",
				Format:   "json",
			},
		},
		{
			name: "No flags changed, set default format",
			setFlags: func(fs *pflag.FlagSet) {
				// No flags set
			},
			expected: &exec.DescribeAffectedCmdArgs{
				Format: "json",
			},
		},
		{
			name: "Set format explicitly, no override",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Set("format", "json")
			},
			expected: &exec.DescribeAffectedCmdArgs{
				Format: "json",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new flag set and add the flags
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			fs.String("ref", "", "ref flag")
			fs.String("sha", "", "sha flag")
			fs.String("repo-path", "", "repo-path flag")
			fs.String("ssh-key", "", "ssh-key flag")
			fs.String("ssh-key-password", "", "ssh-key-password flag")
			fs.Bool("include-spacelift-admin-stacks", false, "include-spacelift-admin-stacks flag")
			fs.Bool("include-dependents", false, "include-dependents flag")
			fs.Bool("include-settings", false, "include-settings flag")
			fs.Bool("upload", false, "upload flag")
			fs.Bool("clone-target-ref", false, "clone-target-ref flag")
			fs.Bool("process-templates", true, "process-templates flag")
			fs.Bool("process-functions", true, "process-functions flag")
			fs.StringSlice("skip", nil, "skip flag")
			fs.String("pager", "", "pager flag")
			fs.String("stack", "", "stack flag")
			fs.String("format", "", "format flag")
			fs.String("file", "", "file flag")
			fs.String("query", "", "query flag")
			fs.String("selector", "", "selector flag")

			// Set the flags as specified in the test
			tt.setFlags(fs)

			// Create the args struct
			args := &exec.DescribeAffectedCmdArgs{
				CLIConfig: &schema.AtmosConfiguration{
					Settings: schema.AtmosSettings{
						Terminal: schema.Terminal{
							Pager: "",
						},
					},
				},
			}

			if tt.expectedPanic {
				assert.Panics(t, func() {
					setDescribeAffectedFlagValueInCliArgs(fs, args)
				}, tt.panicMessage)
			} else {
				setDescribeAffectedFlagValueInCliArgs(fs, args)
				assert.Equal(t, tt.expected.Ref, args.Ref)
				assert.Equal(t, tt.expected.SHA, args.SHA)
				assert.Equal(t, tt.expected.IncludeDependents, args.IncludeDependents)
				assert.Equal(t, tt.expected.IncludeSettings, args.IncludeSettings)
				assert.Equal(t, tt.expected.Upload, args.Upload)
				assert.Equal(t, tt.expected.Format, args.Format)
				assert.Equal(t, tt.expected.Selector, args.Selector)
			}
		})
	}
}
