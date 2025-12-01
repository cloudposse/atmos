package exec

import (
	"reflect"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

// newTestCommandWithGlobalFlags creates a test command with all global flags registered
// using the flag registry pattern. This ensures test commands have the same flags as
// production commands that inherit from RootCmd.
func newTestCommandWithGlobalFlags(use string) *cobra.Command {
	cmd := &cobra.Command{Use: use}

	// Register global flags using the same pattern as cmd/root.go.
	globalParser := flags.NewGlobalOptionsBuilder().Build()
	globalParser.RegisterPersistentFlags(cmd)

	return cmd
}

func Test_processArgsAndFlags(t *testing.T) {
	inputArgsAndFlags := []string{
		"--deploy-run-init=true",
		"--init-pass-vars=true",
		"--skip-planfile=true",
		"--logs-level",
		"Debug",
	}

	info, err := processArgsAndFlags(
		"terraform",
		inputArgsAndFlags,
	)

	assert.NoError(t, err)
	assert.Equal(t, info.DeployRunInit, "true")
	assert.Equal(t, info.InitPassVars, "true")
	assert.Equal(t, info.PlanSkipPlanfile, "true")
	assert.Equal(t, info.LogsLevel, "Debug")
}

func Test_processArgsAndFlags2(t *testing.T) {
	tests := []struct {
		name              string
		componentType     string
		inputArgsAndFlags []string
		want              schema.ArgsAndFlagsInfo
		wantErr           bool
	}{
		{
			name:              "clean command",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"clean"},
			want: schema.ArgsAndFlagsInfo{
				SubCommand: "clean",
			},
			wantErr: false,
		},
		{
			name:              "version command",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"version"},
			want: schema.ArgsAndFlagsInfo{
				SubCommand: "version",
			},
			wantErr: false,
		},
		{
			name:              "help for single command",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan"},
			want: schema.ArgsAndFlagsInfo{
				SubCommand: "plan",
				NeedHelp:   true,
			},
			wantErr: false,
		},
		{
			name:              "terraform command flag",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"--terraform-command", "plan"},
			want: schema.ArgsAndFlagsInfo{
				TerraformCommand: "plan",
			},
			wantErr: false,
		},
		{
			name:              "terraform dir flag",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"--terraform-dir", "/path/to/terraform"},
			want: schema.ArgsAndFlagsInfo{
				TerraformDir: "/path/to/terraform",
			},
			wantErr: false,
		},
		{
			name:          "multiple flags",
			componentType: "terraform",
			inputArgsAndFlags: []string{
				"--terraform-command", "plan",
				"--terraform-dir", "/path/to/terraform",
				"--append-user-agent", "test-agent",
				"--skip-planfile", "true",
				"--init-pass-vars", "true",
			},
			want: schema.ArgsAndFlagsInfo{
				TerraformCommand: "plan",
				TerraformDir:     "/path/to/terraform",
				AppendUserAgent:  "test-agent",
				PlanSkipPlanfile: "true",
				InitPassVars:     "true",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := processArgsAndFlags(tt.componentType, tt.inputArgsAndFlags)
			if (err != nil) != tt.wantErr {
				t.Errorf("processArgsAndFlags() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("processArgsAndFlags() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_processArgsAndFlags_invalidFlag(t *testing.T) {
	inputArgsAndFlags := []string{
		"init",
		"--init-pass-vars=invalid=true",
	}

	_, err := processArgsAndFlags(
		"terraform",
		inputArgsAndFlags,
	)

	assert.Error(t, err)
	assert.ErrorContains(t, err, "invalid flag")
	assert.ErrorContains(t, err, "--init-pass-vars=invalid=true")
}

func Test_processArgsAndFlags_errorPaths(t *testing.T) {
	tests := []struct {
		name              string
		componentType     string
		inputArgsAndFlags []string
		expectedError     string
	}{
		// Missing flag values (need to include plan as subcommand to avoid early return).
		{
			name:              "terraform-command flag without value",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--terraform-command"},
			expectedError:     "--terraform-command",
		},
		{
			name:              "terraform-dir flag without value",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--terraform-dir"},
			expectedError:     "--terraform-dir",
		},
		{
			name:              "append-user-agent flag without value",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--append-user-agent"},
			expectedError:     "--append-user-agent",
		},
		{
			name:              "helmfile-command flag without value",
			componentType:     "helmfile",
			inputArgsAndFlags: []string{"sync", "--helmfile-command"},
			expectedError:     "--helmfile-command",
		},
		{
			name:              "helmfile-dir flag without value",
			componentType:     "helmfile",
			inputArgsAndFlags: []string{"sync", "--helmfile-dir"},
			expectedError:     "--helmfile-dir",
		},
		{
			name:              "config-dir flag without value",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--config-dir"},
			expectedError:     "--config-dir",
		},
		{
			name:              "base-path flag without value",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--base-path"},
			expectedError:     "--base-path",
		},
		{
			name:              "vendor-base-path flag without value",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--vendor-base-path"},
			expectedError:     "--vendor-base-path",
		},
		{
			name:              "deploy-run-init flag without value",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--deploy-run-init"},
			expectedError:     "--deploy-run-init",
		},
		{
			name:              "auto-generate-backend-file flag without value",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--auto-generate-backend-file"},
			expectedError:     "--auto-generate-backend-file",
		},
		{
			name:              "init-run-reconfigure flag without value",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--init-run-reconfigure"},
			expectedError:     "--init-run-reconfigure",
		},
		{
			name:              "logs-level flag without value",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--logs-level"},
			expectedError:     "--logs-level",
		},
		{
			name:              "logs-file flag without value",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--logs-file"},
			expectedError:     "--logs-file",
		},
		// Invalid flag formats with multiple equals signs.
		{
			name:              "terraform-command with multiple equals",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--terraform-command=plan=extra"},
			expectedError:     "--terraform-command=plan=extra",
		},
		{
			name:              "terraform-dir with multiple equals",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--terraform-dir=/path=extra"},
			expectedError:     "--terraform-dir=/path=extra",
		},
		{
			name:              "append-user-agent with multiple equals",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--append-user-agent=agent=extra"},
			expectedError:     "--append-user-agent=agent=extra",
		},
		{
			name:              "helmfile-command with multiple equals",
			componentType:     "helmfile",
			inputArgsAndFlags: []string{"sync", "--helmfile-command=sync=extra"},
			expectedError:     "--helmfile-command=sync=extra",
		},
		{
			name:              "helmfile-dir with multiple equals",
			componentType:     "helmfile",
			inputArgsAndFlags: []string{"sync", "--helmfile-dir=/path=extra"},
			expectedError:     "--helmfile-dir=/path=extra",
		},
		{
			name:              "config-dir with multiple equals",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--config-dir=/path=extra"},
			expectedError:     "--config-dir=/path=extra",
		},
		{
			name:              "base-path with multiple equals",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--base-path=/path=extra"},
			expectedError:     "--base-path=/path=extra",
		},
		{
			name:              "vendor-base-path with multiple equals",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--vendor-base-path=/path=extra"},
			expectedError:     "--vendor-base-path=/path=extra",
		},
		{
			name:              "deploy-run-init with multiple equals",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--deploy-run-init=true=extra"},
			expectedError:     "--deploy-run-init=true=extra",
		},
		{
			name:              "auto-generate-backend-file with multiple equals",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--auto-generate-backend-file=true=extra"},
			expectedError:     "--auto-generate-backend-file=true=extra",
		},
		{
			name:              "init-run-reconfigure with multiple equals",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--init-run-reconfigure=true=extra"},
			expectedError:     "--init-run-reconfigure=true=extra",
		},
		{
			name:              "logs-level with multiple equals",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--logs-level=Debug=extra"},
			expectedError:     "--logs-level=Debug=extra",
		},
		{
			name:              "logs-file with multiple equals",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--logs-file=/path=extra"},
			expectedError:     "--logs-file=/path=extra",
		},
		{
			name:              "init-pass-vars flag without value",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"init", "--init-pass-vars"},
			expectedError:     "--init-pass-vars",
		},
		{
			name:              "init-pass-vars with multiple equals",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"init", "--init-pass-vars=true=extra"},
			expectedError:     "--init-pass-vars=true=extra",
		},
		{
			name:              "skip-planfile flag without value",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--skip-planfile"},
			expectedError:     "--skip-planfile",
		},
		{
			name:              "skip-planfile with multiple equals",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--skip-planfile=true=extra"},
			expectedError:     "--skip-planfile=true=extra",
		},
		{
			name:              "schemas-atmos-manifest flag without value",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--schemas-atmos-manifest"},
			expectedError:     "--schemas-atmos-manifest",
		},
		{
			name:              "schemas-atmos-manifest with multiple equals",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--schemas-atmos-manifest=/path=extra"},
			expectedError:     "--schemas-atmos-manifest=/path=extra",
		},
		{
			name:              "redirect-stderr flag without value",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--redirect-stderr"},
			expectedError:     "--redirect-stderr",
		},
		{
			name:              "redirect-stderr with multiple equals",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--redirect-stderr=/path=extra"},
			expectedError:     "--redirect-stderr=/path=extra",
		},
		{
			name:              "planfile flag without value",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--planfile"},
			expectedError:     "--planfile",
		},
		{
			name:              "planfile with multiple equals",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--planfile=/path=extra"},
			expectedError:     "--planfile=/path=extra",
		},
		{
			name:              "schemas-jsonschema-dir flag without value",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--schemas-jsonschema-dir"},
			expectedError:     "--schemas-jsonschema-dir",
		},
		{
			name:              "schemas-jsonschema-dir with multiple equals",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--schemas-jsonschema-dir=/path=extra"},
			expectedError:     "--schemas-jsonschema-dir=/path=extra",
		},
		{
			name:              "schemas-opa-dir flag without value",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--schemas-opa-dir"},
			expectedError:     "--schemas-opa-dir",
		},
		{
			name:              "schemas-opa-dir with multiple equals",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--schemas-opa-dir=/path=extra"},
			expectedError:     "--schemas-opa-dir=/path=extra",
		},
		{
			name:              "schemas-cue-dir flag without value",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--schemas-cue-dir"},
			expectedError:     "--schemas-cue-dir",
		},
		{
			name:              "schemas-cue-dir with multiple equals",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--schemas-cue-dir=/path=extra"},
			expectedError:     "--schemas-cue-dir=/path=extra",
		},
		{
			name:              "settings-list-merge-strategy flag without value",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--settings-list-merge-strategy"},
			expectedError:     "--settings-list-merge-strategy",
		},
		{
			name:              "settings-list-merge-strategy with multiple equals",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--settings-list-merge-strategy=append=extra"},
			expectedError:     "--settings-list-merge-strategy=append=extra",
		},
		{
			name:              "query flag without value",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--query"},
			expectedError:     "--query",
		},
		{
			name:              "query with multiple equals",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--query=.foo=bar"},
			expectedError:     "--query=.foo=bar",
		},
		{
			name:              "stacks-dir flag without value",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--stacks-dir"},
			expectedError:     "--stacks-dir",
		},
		{
			name:              "stacks-dir with multiple equals",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--stacks-dir=/path=extra"},
			expectedError:     "--stacks-dir=/path=extra",
		},
		{
			name:              "workflows-dir flag without value",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--workflows-dir"},
			expectedError:     "--workflows-dir",
		},
		{
			name:              "workflows-dir with multiple equals",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "--workflows-dir=/path=extra"},
			expectedError:     "--workflows-dir=/path=extra",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := processArgsAndFlags(tt.componentType, tt.inputArgsAndFlags)

			assert.Error(t, err)
			assert.ErrorContains(t, err, "invalid flag")
			assert.ErrorContains(t, err, tt.expectedError)
		})
	}
}

func Test_getCliVars(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		want      map[string]any
		wantErr   bool
		errString string
	}{
		{
			name: "basic var flag",
			args: []string{"-var", "name=value"},
			want: map[string]any{
				"name": "value",
			},
		},
		{
			name: "multiple var flags",
			args: []string{
				"-var", "name=value",
				"-var", "region=us-west-2",
			},
			want: map[string]any{
				"name":   "value",
				"region": "us-west-2",
			},
		},
		{
			name: "var flag with equals sign in value",
			args: []string{"-var", "connection_string=host=localhost;port=5432"},
			want: map[string]any{
				"connection_string": "host=localhost;port=5432",
			},
		},
		{
			name: "var flag with spaces in value",
			args: []string{"-var", "description=This is a test"},
			want: map[string]any{
				"description": "This is a test",
			},
		},
		{
			name: "var-file without value",
			args: []string{"-var-file"},
			want: map[string]any{}, // Should ignore invalid var-file
		},
		{
			name: "ignore non-var flags",
			args: []string{
				"-var", "name=value",
				"--other-flag", "something",
				"-var", "region=us-west-2",
			},
			want: map[string]any{
				"name":   "value",
				"region": "us-west-2",
			},
		},
		{
			name: "empty args",
			args: []string{},
			want: map[string]any{},
		},
		{
			name: "only non-var flags",
			args: []string{"--flag1", "value1", "--flag2", "value2"},
			want: map[string]any{},
		},
		{
			name: "duplicate var names",
			args: []string{
				"-var", "name=value1",
				"-var", "name=value2",
			},
			want: map[string]any{
				"name": "value2", // Last value should win
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getCliVars(tt.args)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errString != "" {
					assert.Contains(t, err.Error(), tt.errString)
				}
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestProcessCommandLineArgs_IdentityFromEnvironmentVariable(t *testing.T) {
	tests := []struct {
		name           string
		envValue       string
		flagValue      string
		args           []string
		expectedResult string
		description    string
	}{
		{
			name:           "uses environment variable when flag not provided",
			envValue:       "test-identity-from-env",
			flagValue:      "",
			args:           []string{"plan", "component", "--stack", "stack"},
			expectedResult: "test-identity-from-env",
			description:    "ATMOS_IDENTITY env var should be used when --identity flag is not provided",
		},
		{
			name:           "flag takes precedence over environment variable",
			envValue:       "test-identity-from-env",
			flagValue:      "test-identity-from-flag",
			args:           []string{"plan", "component", "--stack", "stack", "--identity", "test-identity-from-flag"},
			expectedResult: "test-identity-from-flag",
			description:    "--identity flag should override ATMOS_IDENTITY env var",
		},
		{
			name:           "empty when neither flag nor env var provided",
			envValue:       "",
			flagValue:      "",
			args:           []string{"plan", "component", "--stack", "stack"},
			expectedResult: "",
			description:    "Identity should be empty when neither flag nor env var is set",
		},
		{
			name:           "uses environment variable with identity flag equals syntax",
			envValue:       "test-identity-from-env",
			flagValue:      "",
			args:           []string{"plan", "component", "--stack", "stack"},
			expectedResult: "test-identity-from-env",
			description:    "ATMOS_IDENTITY should work regardless of flag syntax",
		},
		{
			name:           "flag with equals syntax takes precedence",
			envValue:       "test-identity-from-env",
			flagValue:      "test-identity-from-flag-equals",
			args:           []string{"plan", "component", "--stack", "stack", "--identity=test-identity-from-flag-equals"},
			expectedResult: "test-identity-from-flag-equals",
			description:    "--identity=value syntax should override ATMOS_IDENTITY env var",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variable.
			if tt.envValue != "" {
				t.Setenv("ATMOS_IDENTITY", tt.envValue)
			}

			// Create a test command with global flags registered via flag registry.
			cmd := newTestCommandWithGlobalFlags("terraform")
			cmd.Flags().String("stack", "", "stack name")
			cmd.Flags().String("identity", "", "identity name")

			// Process the command-line arguments.
			result, err := ProcessCommandLineArgs("terraform", cmd, tt.args, []string{})

			// Verify results.
			require.NoError(t, err, "ProcessCommandLineArgs should not return error")
			assert.Equal(t, tt.expectedResult, result.Identity, tt.description)
		})
	}
}

// TestProcessCommandLineArgs_IdentityFlagParsing verifies that the --identity flag
// is correctly parsed in both space-separated and equals syntax formats.
func TestProcessCommandLineArgs_IdentityFlagParsing(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedResult string
	}{
		{
			name:           "identity flag with space separator",
			args:           []string{"plan", "component", "--stack", "stack", "--identity", "my-identity"},
			expectedResult: "my-identity",
		},
		{
			name:           "identity flag with equals syntax",
			args:           []string{"plan", "component", "--stack", "stack", "--identity=my-identity"},
			expectedResult: "my-identity",
		},
		{
			name:           "identity flag with hyphenated name",
			args:           []string{"plan", "component", "--stack", "stack", "--identity", "core-identity/managers"},
			expectedResult: "core-identity/managers",
		},
		{
			name:           "identity flag at different position",
			args:           []string{"--identity", "early-identity", "plan", "component", "--stack", "stack"},
			expectedResult: "early-identity",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test command with global flags registered via flag registry.
			cmd := newTestCommandWithGlobalFlags("terraform")
			cmd.Flags().String("stack", "", "stack name")
			cmd.Flags().String("identity", "", "identity name")

			// Process the command-line arguments.
			result, err := ProcessCommandLineArgs("terraform", cmd, tt.args, []string{})

			// Verify results.
			require.NoError(t, err, "ProcessCommandLineArgs should not return error")
			assert.Equal(t, tt.expectedResult, result.Identity, "Identity should match expected value")
		})
	}
}

func TestProcessCommandLineArgs_ProfileFromEnvironmentVariable(t *testing.T) {
	tests := []struct {
		name           string
		envValue       string
		expectedResult []string
	}{
		{
			name:           "single profile from environment variable",
			envValue:       "production",
			expectedResult: []string{"production"},
		},
		{
			name:           "multiple profiles from environment variable",
			envValue:       "dev,staging,prod",
			expectedResult: []string{"dev", "staging", "prod"},
		},
		{
			name:           "profiles with empty entries",
			envValue:       "dev,,prod",
			expectedResult: []string{"dev", "prod"},
		},
		{
			name:           "profiles with spaces",
			envValue:       "dev, staging , prod",
			expectedResult: []string{"dev", "staging", "prod"},
		},
		{
			name:           "profiles with leading/trailing commas",
			envValue:       ",dev,staging,",
			expectedResult: []string{"dev", "staging"},
		},
		{
			name:           "only whitespace and commas",
			envValue:       " , , ",
			expectedResult: []string{},
		},
		{
			name:           "empty environment variable",
			envValue:       "",
			expectedResult: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable.
			if tt.envValue != "" {
				t.Setenv("ATMOS_PROFILE", tt.envValue)
			}

			// Create test command with global flags (including profile flag).
			cmd := newTestCommandWithGlobalFlags("test")
			cmd.Flags().String("stack", "", "stack name")

			// Process with no --profile flag in args (should use env var).
			result, err := ProcessCommandLineArgs("terraform", cmd, []string{}, []string{})

			// Verify results.
			require.NoError(t, err, "ProcessCommandLineArgs should not return error")
			assert.Equal(t, tt.expectedResult, result.ProfilesFromArg, "Profiles should match expected value")
		})
	}
}

func TestProcessCommandLineArgs_ProfileFlagTakesPrecedenceOverEnv(t *testing.T) {
	// Set environment variable.
	t.Setenv("ATMOS_PROFILE", "env-profile")

	// Create test command with global flags.
	cmd := newTestCommandWithGlobalFlags("test")
	cmd.Flags().String("stack", "", "stack name")

	// Set profile flag directly (simulating --profile flag).
	// Profile is a persistent flag, so use PersistentFlags().
	err := cmd.PersistentFlags().Set("profile", "flag-profile")
	require.NoError(t, err)

	// Process command line args.
	result, err := ProcessCommandLineArgs("terraform", cmd, []string{}, []string{})

	// Verify that flag takes precedence.
	require.NoError(t, err)
	assert.Equal(t, []string{"flag-profile"}, result.ProfilesFromArg, "Flag should take precedence over env var")
}
