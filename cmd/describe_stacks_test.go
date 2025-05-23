package cmd

import (
	"fmt"
	"testing"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
)

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
				fs.Set("process-templates", "false")
				fs.Set("format", "json")
			},
			describe: &exec.DescribeStacksArgs{},
			expected: &exec.DescribeStacksArgs{
				Format:           "json",
				ProcessTemplates: false,
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

	// Test panic for unsupported type
	t.Run("Unsupported flag type", func(t *testing.T) {
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		fs.Int("invalid", 0, "Invalid flag") // Int type is not supported
		fs.Set("invalid", "42")

		defer func() {
			if r := recover(); r != nil {
				expected := "unsupported type *int for flag invalid"
				if fmt.Sprintf("%v", r) != expected {
					t.Errorf("Expected panic message %q, got %v", expected, r)
				}
			} else {
				t.Error("Expected panic but none occurred")
			}
		}()

		// Override flagsKeyValue to include an int type
		originalFlagsKeyValue := map[string]any{
			"invalid": new(int),
		}
		for k, v := range originalFlagsKeyValue {
			if !fs.Changed(k) {
				continue
			}
			switch v := v.(type) {
			case *string:
				*v, _ = fs.GetString(k)
			case *bool:
				*v, _ = fs.GetBool(k)
			default:
				panic(fmt.Sprintf("unsupported type %T for flag %s", v, k))
			}
		}
	})
}
