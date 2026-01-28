package exec

import (
	"reflect"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
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
			name:              "single subcommand allows interactive prompts",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan"},
			want: schema.ArgsAndFlagsInfo{
				SubCommand: "plan",
				NeedHelp:   false, // Don't auto-show help; allow interactive prompts.
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
		// Test cases for ATMOS_IDENTITY=false (issue #1931).
		{
			name:           "ATMOS_IDENTITY=false disables authentication",
			envValue:       "false",
			flagValue:      "",
			args:           []string{"plan", "component", "--stack", "stack"},
			expectedResult: cfg.IdentityFlagDisabledValue,
			description:    "ATMOS_IDENTITY=false should be normalized to __DISABLED__",
		},
		{
			name:           "ATMOS_IDENTITY=FALSE disables authentication",
			envValue:       "FALSE",
			flagValue:      "",
			args:           []string{"plan", "component", "--stack", "stack"},
			expectedResult: cfg.IdentityFlagDisabledValue,
			description:    "ATMOS_IDENTITY=FALSE should be normalized to __DISABLED__",
		},
		{
			name:           "ATMOS_IDENTITY=0 disables authentication",
			envValue:       "0",
			flagValue:      "",
			args:           []string{"plan", "component", "--stack", "stack"},
			expectedResult: cfg.IdentityFlagDisabledValue,
			description:    "ATMOS_IDENTITY=0 should be normalized to __DISABLED__",
		},
		{
			name:           "ATMOS_IDENTITY=no disables authentication",
			envValue:       "no",
			flagValue:      "",
			args:           []string{"plan", "component", "--stack", "stack"},
			expectedResult: cfg.IdentityFlagDisabledValue,
			description:    "ATMOS_IDENTITY=no should be normalized to __DISABLED__",
		},
		{
			name:           "ATMOS_IDENTITY=off disables authentication",
			envValue:       "off",
			flagValue:      "",
			args:           []string{"plan", "component", "--stack", "stack"},
			expectedResult: cfg.IdentityFlagDisabledValue,
			description:    "ATMOS_IDENTITY=off should be normalized to __DISABLED__",
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

// TestProcessCommandLineArgs_FromPlanBeforeComponent verifies that placing --from-plan
// before the component name results in an error rather than silent misinterpretation.
// This tests the edge case where --from-plan is misplaced in the argument order.
//
// Correct usage: atmos terraform apply <component> -s <stack> --from-plan.
// Incorrect usage: atmos terraform apply --from-plan <component> -s <stack>.
func TestProcessCommandLineArgs_FromPlanBeforeComponent(t *testing.T) {
	// Create a test command simulating terraform apply with global flags.
	cmd := newTestCommandWithGlobalFlags("terraform")
	cmd.Flags().String("stack", "", "stack name")

	// Test case: --from-plan before component name should result in an error.
	// The flag parser sees --from-plan as an unknown flag because it's processed
	// before the flag handling code recognizes it in the proper position.
	args := []string{"apply", "--from-plan", "component-name", "-s", "test-stack"}

	_, err := ProcessCommandLineArgs("terraform", cmd, args, []string{})

	// The function should return an error - this prevents silent misinterpretation
	// where the component name could be mistakenly used as the plan file path.
	require.Error(t, err, "ProcessCommandLineArgs should return error when --from-plan precedes component name")
}

// TestProcessArgsAndFlags_TwoWordCommands tests two-word terraform commands like "providers lock".
// This includes both the standard form (separate words) and the quoted form (single argument).
func TestProcessArgsAndFlags_TwoWordCommands(t *testing.T) {
	tests := []struct {
		name              string
		componentType     string
		inputArgsAndFlags []string
		wantSubCommand    string
		wantSubCommand2   string
		wantComponent     string
		wantAdditional    []string
		wantErr           bool
	}{
		// Providers commands - separate words.
		{
			name:              "providers lock - separate words",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"providers", "lock", "my-component"},
			wantSubCommand:    "providers lock",
			wantSubCommand2:   "",
			wantComponent:     "my-component",
			wantAdditional:    nil,
			wantErr:           false,
		},
		{
			name:              "providers lock - separate words with flags",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"providers", "lock", "my-component", "-platform=linux_amd64"},
			wantSubCommand:    "providers lock",
			wantSubCommand2:   "",
			wantComponent:     "my-component",
			wantAdditional:    []string{"-platform=linux_amd64"},
			wantErr:           false,
		},
		{
			name:              "providers mirror - separate words",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"providers", "mirror", "my-component"},
			wantSubCommand:    "providers mirror",
			wantSubCommand2:   "",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		{
			name:              "providers schema - separate words",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"providers", "schema", "my-component"},
			wantSubCommand:    "providers schema",
			wantSubCommand2:   "",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		// Providers commands - quoted (single argument).
		{
			name:              "providers lock - quoted",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"providers lock", "my-component"},
			wantSubCommand:    "providers lock",
			wantSubCommand2:   "",
			wantComponent:     "my-component",
			wantAdditional:    nil,
			wantErr:           false,
		},
		{
			name:              "providers lock - quoted with flags",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"providers lock", "my-component", "-platform=darwin_amd64"},
			wantSubCommand:    "providers lock",
			wantSubCommand2:   "",
			wantComponent:     "my-component",
			wantAdditional:    []string{"-platform=darwin_amd64"},
			wantErr:           false,
		},
		{
			name:              "providers mirror - quoted",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"providers mirror", "my-component"},
			wantSubCommand:    "providers mirror",
			wantSubCommand2:   "",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		// State commands - separate words.
		{
			name:              "state list - separate words",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"state", "list", "my-component"},
			wantSubCommand:    "state list",
			wantSubCommand2:   "",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		{
			name:              "state mv - separate words",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"state", "mv", "my-component"},
			wantSubCommand:    "state mv",
			wantSubCommand2:   "",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		// State commands - quoted.
		{
			name:              "state list - quoted",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"state list", "my-component"},
			wantSubCommand:    "state list",
			wantSubCommand2:   "",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		// Workspace commands - separate words.
		{
			name:              "workspace select - separate words",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"workspace", "select", "my-component"},
			wantSubCommand:    "workspace",
			wantSubCommand2:   "select",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		{
			name:              "workspace list - separate words",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"workspace", "list", "my-component"},
			wantSubCommand:    "workspace",
			wantSubCommand2:   "list",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		// Workspace commands - quoted.
		{
			name:              "workspace select - quoted",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"workspace select", "my-component"},
			wantSubCommand:    "workspace",
			wantSubCommand2:   "select",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		// Write varfile - separate words.
		{
			name:              "write varfile - separate words",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"write", "varfile", "my-component"},
			wantSubCommand:    "write",
			wantSubCommand2:   "varfile",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		// Write varfile - quoted.
		{
			name:              "write varfile - quoted",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"write varfile", "my-component"},
			wantSubCommand:    "write",
			wantSubCommand2:   "varfile",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		// Error cases.
		{
			name:              "providers lock - missing component",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"providers", "lock"},
			wantErr:           true,
		},
		{
			name:              "providers lock quoted - missing component",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"providers lock"},
			wantErr:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := processArgsAndFlags(tt.componentType, tt.inputArgsAndFlags)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantSubCommand, got.SubCommand, "SubCommand mismatch")
			assert.Equal(t, tt.wantSubCommand2, got.SubCommand2, "SubCommand2 mismatch")
			assert.Equal(t, tt.wantComponent, got.ComponentFromArg, "ComponentFromArg mismatch")
			if tt.wantAdditional != nil {
				assert.Equal(t, tt.wantAdditional, got.AdditionalArgsAndFlags, "AdditionalArgsAndFlags mismatch")
			}
		})
	}
}

// TestParseTwoWordCommand tests the parseTwoWordCommand helper function.
func TestParseTwoWordCommand(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    *twoWordCommandResult
		wantNil bool
	}{
		{
			name:    "empty args",
			args:    []string{},
			wantNil: true,
		},
		{
			name:    "single word command",
			args:    []string{"plan"},
			wantNil: true,
		},
		{
			name: "providers lock - quoted",
			args: []string{"providers lock", "component"},
			want: &twoWordCommandResult{
				subCommand: "providers lock",
				argCount:   1,
			},
		},
		{
			name: "providers lock - separate",
			args: []string{"providers", "lock", "component"},
			want: &twoWordCommandResult{
				subCommand: "providers lock",
				argCount:   2,
			},
		},
		{
			name: "state list - quoted",
			args: []string{"state list", "component"},
			want: &twoWordCommandResult{
				subCommand: "state list",
				argCount:   1,
			},
		},
		{
			name: "workspace select - quoted",
			args: []string{"workspace select", "component"},
			want: &twoWordCommandResult{
				subCommand:  "workspace",
				subCommand2: "select",
				argCount:    1,
			},
		},
		{
			name: "write varfile - separate",
			args: []string{"write", "varfile", "component"},
			want: &twoWordCommandResult{
				subCommand:  "write",
				subCommand2: "varfile",
				argCount:    2,
			},
		},
		{
			name:    "unknown two-word command",
			args:    []string{"unknown command", "component"},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTwoWordCommand(tt.args)
			if tt.wantNil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, tt.want.subCommand, got.subCommand)
			assert.Equal(t, tt.want.subCommand2, got.subCommand2)
			assert.Equal(t, tt.want.argCount, got.argCount)
		})
	}
}

// TestParseQuotedTwoWordCommand tests the parseQuotedTwoWordCommand helper function.
func TestParseQuotedTwoWordCommand(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		want    *twoWordCommandResult
		wantNil bool
	}{
		{
			name:    "single word",
			arg:     "plan",
			wantNil: true,
		},
		{
			name: "providers lock",
			arg:  "providers lock",
			want: &twoWordCommandResult{
				subCommand: "providers lock",
				argCount:   1,
			},
		},
		{
			name: "providers mirror",
			arg:  "providers mirror",
			want: &twoWordCommandResult{
				subCommand: "providers mirror",
				argCount:   1,
			},
		},
		{
			name: "providers schema",
			arg:  "providers schema",
			want: &twoWordCommandResult{
				subCommand: "providers schema",
				argCount:   1,
			},
		},
		{
			name: "state list",
			arg:  "state list",
			want: &twoWordCommandResult{
				subCommand: "state list",
				argCount:   1,
			},
		},
		{
			name: "state mv",
			arg:  "state mv",
			want: &twoWordCommandResult{
				subCommand: "state mv",
				argCount:   1,
			},
		},
		{
			name: "workspace select",
			arg:  "workspace select",
			want: &twoWordCommandResult{
				subCommand:  "workspace",
				subCommand2: "select",
				argCount:    1,
			},
		},
		{
			name: "write varfile",
			arg:  "write varfile",
			want: &twoWordCommandResult{
				subCommand:  "write",
				subCommand2: "varfile",
				argCount:    1,
			},
		},
		{
			name:    "unknown command",
			arg:     "foo bar",
			wantNil: true,
		},
		{
			name:    "providers with unknown subcommand",
			arg:     "providers unknown",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseQuotedTwoWordCommand(tt.arg)
			if tt.wantNil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, tt.want.subCommand, got.subCommand)
			assert.Equal(t, tt.want.subCommand2, got.subCommand2)
			assert.Equal(t, tt.want.argCount, got.argCount)
		})
	}
}

// TestParseSeparateTwoWordCommand tests the parseSeparateTwoWordCommand helper function.
func TestParseSeparateTwoWordCommand(t *testing.T) {
	tests := []struct {
		name    string
		first   string
		second  string
		want    *twoWordCommandResult
		wantNil bool
	}{
		{
			name:   "providers lock",
			first:  "providers",
			second: "lock",
			want: &twoWordCommandResult{
				subCommand: "providers lock",
				argCount:   2,
			},
		},
		{
			name:   "state list",
			first:  "state",
			second: "list",
			want: &twoWordCommandResult{
				subCommand: "state list",
				argCount:   2,
			},
		},
		{
			name:   "workspace new",
			first:  "workspace",
			second: "new",
			want: &twoWordCommandResult{
				subCommand:  "workspace",
				subCommand2: "new",
				argCount:    2,
			},
		},
		{
			name:   "write varfile",
			first:  "write",
			second: "varfile",
			want: &twoWordCommandResult{
				subCommand:  "write",
				subCommand2: "varfile",
				argCount:    2,
			},
		},
		{
			name:    "unknown command",
			first:   "foo",
			second:  "bar",
			wantNil: true,
		},
		{
			name:    "state with unknown subcommand",
			first:   "state",
			second:  "unknown",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSeparateTwoWordCommand(tt.first, tt.second)
			if tt.wantNil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, tt.want.subCommand, got.subCommand)
			assert.Equal(t, tt.want.subCommand2, got.subCommand2)
			assert.Equal(t, tt.want.argCount, got.argCount)
		})
	}
}

// TestProcessTerraformTwoWordCommand tests the processTerraformTwoWordCommand helper function.
func TestProcessTerraformTwoWordCommand(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantProcessed  bool
		wantErr        bool
		wantSubCommand string
		wantComponent  string
	}{
		{
			name:          "not a two-word command",
			args:          []string{"plan", "component"},
			wantProcessed: false,
			wantErr:       false,
		},
		{
			name:           "providers lock with component",
			args:           []string{"providers", "lock", "my-component"},
			wantProcessed:  true,
			wantErr:        false,
			wantSubCommand: "providers lock",
			wantComponent:  "my-component",
		},
		{
			name:           "providers lock quoted with component",
			args:           []string{"providers lock", "my-component"},
			wantProcessed:  true,
			wantErr:        false,
			wantSubCommand: "providers lock",
			wantComponent:  "my-component",
		},
		{
			name:          "providers lock without component",
			args:          []string{"providers", "lock"},
			wantProcessed: true,
			wantErr:       true,
		},
		{
			name:          "providers lock quoted without component",
			args:          []string{"providers lock"},
			wantProcessed: true,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &schema.ArgsAndFlagsInfo{}
			processed, err := processTerraformTwoWordCommand(info, tt.args)

			assert.Equal(t, tt.wantProcessed, processed)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			if tt.wantProcessed {
				assert.Equal(t, tt.wantSubCommand, info.SubCommand)
				assert.Equal(t, tt.wantComponent, info.ComponentFromArg)
			}
		})
	}
}

// TestProcessSingleCommand tests the processSingleCommand helper function.
func TestProcessSingleCommand(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantErr        bool
		wantSubCommand string
		wantComponent  string
		wantAdditional []string
	}{
		{
			name:           "single arg - subcommand only",
			args:           []string{"plan"},
			wantErr:        false,
			wantSubCommand: "plan",
		},
		{
			name:           "two args - subcommand and component",
			args:           []string{"plan", "my-component"},
			wantErr:        false,
			wantSubCommand: "plan",
			wantComponent:  "my-component",
		},
		{
			name:           "three args - with additional",
			args:           []string{"plan", "my-component", "-var=foo=bar"},
			wantErr:        false,
			wantSubCommand: "plan",
			wantComponent:  "my-component",
			wantAdditional: []string{"-var=foo=bar"},
		},
		{
			name:           "subcommand with flag instead of component",
			args:           []string{"plan", "--help"},
			wantErr:        false,
			wantSubCommand: "plan",
			wantAdditional: []string{"--help"},
		},
		{
			name:    "empty second argument",
			args:    []string{"plan", ""},
			wantErr: true,
		},
		{
			name:    "invalid flag format",
			args:    []string{"plan", "--"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &schema.ArgsAndFlagsInfo{}
			err := processSingleCommand(info, tt.args)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.wantSubCommand, info.SubCommand)
			assert.Equal(t, tt.wantComponent, info.ComponentFromArg)
			if tt.wantAdditional != nil {
				assert.Equal(t, tt.wantAdditional, info.AdditionalArgsAndFlags)
			}
		})
	}
}
