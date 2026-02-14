package exec

import (
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

// absTestPath returns a platform-appropriate absolute path for tests.
// On Windows, filepath.IsAbs requires a drive letter prefix (e.g., C:\),
// while on Unix systems a leading "/" is sufficient.
func absTestPath(name string) string {
	// Build an OS-appropriate absolute path root.
	// On Windows, "C:" alone is relative to CWD on drive C; "C:\" is absolute.
	root := string(filepath.Separator)
	if runtime.GOOS == "windows" {
		root = "C:" + root
	}
	return filepath.Join(root, filepath.FromSlash(name))
}

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
		// --cluster-name flag tests for EKS/Helmfile integration.
		{
			name:              "cluster-name flag with space separator",
			componentType:     "helmfile",
			inputArgsAndFlags: []string{"sync", "--cluster-name", "my-eks-cluster"},
			want: schema.ArgsAndFlagsInfo{
				SubCommand:  "sync",
				ClusterName: "my-eks-cluster",
			},
			wantErr: false,
		},
		{
			name:              "cluster-name flag with equals syntax",
			componentType:     "helmfile",
			inputArgsAndFlags: []string{"sync", "--cluster-name=prod-eks-cluster"},
			want: schema.ArgsAndFlagsInfo{
				SubCommand:  "sync",
				ClusterName: "prod-eks-cluster",
			},
			wantErr: false,
		},
		{
			name:              "cluster-name flag with hyphenated value",
			componentType:     "helmfile",
			inputArgsAndFlags: []string{"apply", "--cluster-name", "tenant1-dev-us-east-2-eks"},
			want: schema.ArgsAndFlagsInfo{
				SubCommand:  "apply",
				ClusterName: "tenant1-dev-us-east-2-eks",
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
		// --cluster-name flag error cases for EKS/Helmfile integration.
		{
			name:              "cluster-name flag without value",
			componentType:     "helmfile",
			inputArgsAndFlags: []string{"sync", "--cluster-name"},
			expectedError:     "--cluster-name",
		},
		{
			name:              "cluster-name with multiple equals",
			componentType:     "helmfile",
			inputArgsAndFlags: []string{"sync", "--cluster-name=cluster=extra"},
			expectedError:     "--cluster-name=cluster=extra",
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

// TestProcessCommandLineArgs_IdentityFromCobraParsedFlags verifies that the --identity
// flag value is preserved when Cobra pre-parses it before ProcessCommandLineArgs.
// This simulates the real terraform command flow:
// 1. Plan.go RunE receives full args including --identity.
// 2. Cobra parses flags (identity is consumed and stored in cmd.Flags())
// 3. RunE calls terraformRunWithOptions with only positional args (flags removed)
// 4. ProcessCommandLineArgs receives stripped args, reads identity from Cobra flag storage.
func TestProcessCommandLineArgs_IdentityFromCobraParsedFlags(t *testing.T) {
	tests := []struct {
		name           string
		fullArgs       []string // What Cobra initially sees
		strippedArgs   []string // What ProcessCommandLineArgs receives after Cobra consumed flags
		expectedResult string
	}{
		{
			name:           "identity flag with equals syntax consumed by Cobra",
			fullArgs:       []string{"plan", "eks", "--stack", "test", "--identity=staging-admin"},
			strippedArgs:   []string{"plan", "eks"},
			expectedResult: "staging-admin",
		},
		{
			name:           "identity without value consumed by Cobra",
			fullArgs:       []string{"plan", "eks", "--stack", "test", "--identity"},
			strippedArgs:   []string{"plan", "eks"},
			expectedResult: "__SELECT__", // NoOptDefVal
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create command with identity flag (matching terraform setup).
			cmd := newTestCommandWithGlobalFlags("terraform")
			cmd.Flags().String("stack", "", "stack name")
			identityFlag := cmd.Flags().String("identity", "", "identity name")
			cmd.Flags().Lookup("identity").NoOptDefVal = "__SELECT__"

			// Step 1: Cobra parses the full args (simulating plan.go RunE entry).
			err := cmd.ParseFlags(tt.fullArgs)
			require.NoError(t, err)

			// Verify Cobra parsed it correctly.
			require.Equal(t, tt.expectedResult, *identityFlag, "Cobra should have parsed identity")

			// Step 2: ProcessCommandLineArgs receives stripped args (simulating terraformRunWithOptions).
			result, err := ProcessCommandLineArgs("terraform", cmd, tt.strippedArgs, []string{})

			// Verify results.
			require.NoError(t, err)
			// Identity preserved from Cobra-parsed flags.
			assert.Equal(t, tt.expectedResult, result.Identity, "Identity should be preserved from Cobra's parsed flags")
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

// TestProcessArgsAndFlags_CompoundSubcommands tests terraform compound subcommands like "providers lock".
// This includes both the standard form (separate words) and the quoted form (single argument).
func TestProcessArgsAndFlags_CompoundSubcommands(t *testing.T) {
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
		{
			name:              "state pull - separate words",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"state", "pull", "my-component"},
			wantSubCommand:    "state pull",
			wantSubCommand2:   "",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		{
			name:              "state push - separate words",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"state", "push", "my-component"},
			wantSubCommand:    "state push",
			wantSubCommand2:   "",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		{
			name:              "state replace-provider - separate words",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"state", "replace-provider", "my-component"},
			wantSubCommand:    "state replace-provider",
			wantSubCommand2:   "",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		{
			name:              "state rm - separate words",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"state", "rm", "my-component"},
			wantSubCommand:    "state rm",
			wantSubCommand2:   "",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		{
			name:              "state show - separate words",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"state", "show", "my-component"},
			wantSubCommand:    "state show",
			wantSubCommand2:   "",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		{
			name:              "state list - separate words with flags",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"state", "list", "my-component", "-id=123"},
			wantSubCommand:    "state list",
			wantSubCommand2:   "",
			wantComponent:     "my-component",
			wantAdditional:    []string{"-id=123"},
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
		{
			name:              "state mv - quoted",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"state mv", "my-component"},
			wantSubCommand:    "state mv",
			wantSubCommand2:   "",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		{
			name:              "state pull - quoted",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"state pull", "my-component"},
			wantSubCommand:    "state pull",
			wantSubCommand2:   "",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		{
			name:              "state push - quoted",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"state push", "my-component"},
			wantSubCommand:    "state push",
			wantSubCommand2:   "",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		{
			name:              "state replace-provider - quoted",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"state replace-provider", "my-component"},
			wantSubCommand:    "state replace-provider",
			wantSubCommand2:   "",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		{
			name:              "state rm - quoted",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"state rm", "my-component"},
			wantSubCommand:    "state rm",
			wantSubCommand2:   "",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		{
			name:              "state show - quoted",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"state show", "my-component"},
			wantSubCommand:    "state show",
			wantSubCommand2:   "",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		{
			name:              "state list - quoted with flags",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"state list", "my-component", "-id=123"},
			wantSubCommand:    "state list",
			wantSubCommand2:   "",
			wantComponent:     "my-component",
			wantAdditional:    []string{"-id=123"},
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
		{
			name:              "workspace new - separate words",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"workspace", "new", "my-component"},
			wantSubCommand:    "workspace",
			wantSubCommand2:   "new",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		{
			name:              "workspace delete - separate words",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"workspace", "delete", "my-component"},
			wantSubCommand:    "workspace",
			wantSubCommand2:   "delete",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		{
			name:              "workspace show - separate words",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"workspace", "show", "my-component"},
			wantSubCommand:    "workspace",
			wantSubCommand2:   "show",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		{
			name:              "workspace select - separate words with flags",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"workspace", "select", "my-component", "-lock=false"},
			wantSubCommand:    "workspace",
			wantSubCommand2:   "select",
			wantComponent:     "my-component",
			wantAdditional:    []string{"-lock=false"},
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
		{
			name:              "workspace list - quoted",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"workspace list", "my-component"},
			wantSubCommand:    "workspace",
			wantSubCommand2:   "list",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		{
			name:              "workspace new - quoted",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"workspace new", "my-component"},
			wantSubCommand:    "workspace",
			wantSubCommand2:   "new",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		{
			name:              "workspace delete - quoted",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"workspace delete", "my-component"},
			wantSubCommand:    "workspace",
			wantSubCommand2:   "delete",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		{
			name:              "workspace show - quoted",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"workspace show", "my-component"},
			wantSubCommand:    "workspace",
			wantSubCommand2:   "show",
			wantComponent:     "my-component",
			wantErr:           false,
		},
		{
			name:              "workspace select - quoted with flags",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"workspace select", "my-component", "-lock=false"},
			wantSubCommand:    "workspace",
			wantSubCommand2:   "select",
			wantComponent:     "my-component",
			wantAdditional:    []string{"-lock=false"},
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
		// Providers - quoted schema.
		{
			name:              "providers schema - quoted",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"providers schema", "my-component"},
			wantSubCommand:    "providers schema",
			wantSubCommand2:   "",
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
		{
			name:              "state list - missing component",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"state", "list"},
			wantErr:           true,
		},
		{
			name:              "state list quoted - missing component",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"state list"},
			wantErr:           true,
		},
		{
			name:              "workspace select - missing component",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"workspace", "select"},
			wantErr:           true,
		},
		{
			name:              "workspace select quoted - missing component",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"workspace select"},
			wantErr:           true,
		},
		{
			name:              "write varfile - missing component",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"write", "varfile"},
			wantErr:           true,
		},
		{
			name:              "write varfile quoted - missing component",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"write varfile"},
			wantErr:           true,
		},
		// Non-terraform component type should not match compound subcommands.
		{
			name:              "helmfile state list - treated as single command",
			componentType:     "helmfile",
			inputArgsAndFlags: []string{"state", "list", "my-component"},
			wantSubCommand:    "state",
			wantSubCommand2:   "",
			wantComponent:     "list",
			wantAdditional:    []string{"my-component"},
			wantErr:           false,
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
			} else {
				assert.Empty(t, got.AdditionalArgsAndFlags, "AdditionalArgsAndFlags should be empty")
			}
		})
	}
}

// TestParseCompoundSubcommand tests the parseCompoundSubcommand helper function.
func TestParseCompoundSubcommand(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    *compoundSubcommandResult
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
			want: &compoundSubcommandResult{
				subCommand: "providers lock",
				argCount:   1,
			},
		},
		{
			name: "providers lock - separate",
			args: []string{"providers", "lock", "component"},
			want: &compoundSubcommandResult{
				subCommand: "providers lock",
				argCount:   2,
			},
		},
		{
			name: "state list - quoted",
			args: []string{"state list", "component"},
			want: &compoundSubcommandResult{
				subCommand: "state list",
				argCount:   1,
			},
		},
		{
			name: "workspace select - quoted",
			args: []string{"workspace select", "component"},
			want: &compoundSubcommandResult{
				subCommand:  "workspace",
				subCommand2: "select",
				argCount:    1,
			},
		},
		{
			name: "write varfile - separate",
			args: []string{"write", "varfile", "component"},
			want: &compoundSubcommandResult{
				subCommand:  "write",
				subCommand2: "varfile",
				argCount:    2,
			},
		},
		{
			name:    "unknown compound subcommand",
			args:    []string{"unknown command", "component"},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCompoundSubcommand(tt.args)
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

// TestParseQuotedCompoundSubcommand tests the parseQuotedCompoundSubcommand helper function.
func TestParseQuotedCompoundSubcommand(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		want    *compoundSubcommandResult
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
			want: &compoundSubcommandResult{
				subCommand: "providers lock",
				argCount:   1,
			},
		},
		{
			name: "providers mirror",
			arg:  "providers mirror",
			want: &compoundSubcommandResult{
				subCommand: "providers mirror",
				argCount:   1,
			},
		},
		{
			name: "providers schema",
			arg:  "providers schema",
			want: &compoundSubcommandResult{
				subCommand: "providers schema",
				argCount:   1,
			},
		},
		{
			name: "state list",
			arg:  "state list",
			want: &compoundSubcommandResult{
				subCommand: "state list",
				argCount:   1,
			},
		},
		{
			name: "state mv",
			arg:  "state mv",
			want: &compoundSubcommandResult{
				subCommand: "state mv",
				argCount:   1,
			},
		},
		{
			name: "state pull",
			arg:  "state pull",
			want: &compoundSubcommandResult{
				subCommand: "state pull",
				argCount:   1,
			},
		},
		{
			name: "state push",
			arg:  "state push",
			want: &compoundSubcommandResult{
				subCommand: "state push",
				argCount:   1,
			},
		},
		{
			name: "state replace-provider",
			arg:  "state replace-provider",
			want: &compoundSubcommandResult{
				subCommand: "state replace-provider",
				argCount:   1,
			},
		},
		{
			name: "state rm",
			arg:  "state rm",
			want: &compoundSubcommandResult{
				subCommand: "state rm",
				argCount:   1,
			},
		},
		{
			name: "state show",
			arg:  "state show",
			want: &compoundSubcommandResult{
				subCommand: "state show",
				argCount:   1,
			},
		},
		{
			name: "workspace select",
			arg:  "workspace select",
			want: &compoundSubcommandResult{
				subCommand:  "workspace",
				subCommand2: "select",
				argCount:    1,
			},
		},
		{
			name: "workspace list",
			arg:  "workspace list",
			want: &compoundSubcommandResult{
				subCommand:  "workspace",
				subCommand2: "list",
				argCount:    1,
			},
		},
		{
			name: "workspace new",
			arg:  "workspace new",
			want: &compoundSubcommandResult{
				subCommand:  "workspace",
				subCommand2: "new",
				argCount:    1,
			},
		},
		{
			name: "workspace delete",
			arg:  "workspace delete",
			want: &compoundSubcommandResult{
				subCommand:  "workspace",
				subCommand2: "delete",
				argCount:    1,
			},
		},
		{
			name: "workspace show",
			arg:  "workspace show",
			want: &compoundSubcommandResult{
				subCommand:  "workspace",
				subCommand2: "show",
				argCount:    1,
			},
		},
		{
			name: "write varfile",
			arg:  "write varfile",
			want: &compoundSubcommandResult{
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
		{
			name:    "state with unknown subcommand",
			arg:     "state unknown",
			wantNil: true,
		},
		{
			name:    "workspace with unknown subcommand",
			arg:     "workspace unknown",
			wantNil: true,
		},
		{
			name:    "write with unknown subcommand",
			arg:     "write unknown",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseQuotedCompoundSubcommand(tt.arg)
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

// TestParseSeparateCompoundSubcommand tests the parseSeparateCompoundSubcommand helper function.
func TestParseSeparateCompoundSubcommand(t *testing.T) {
	tests := []struct {
		name    string
		first   string
		second  string
		want    *compoundSubcommandResult
		wantNil bool
	}{
		{
			name:   "providers lock",
			first:  "providers",
			second: "lock",
			want: &compoundSubcommandResult{
				subCommand: "providers lock",
				argCount:   2,
			},
		},
		{
			name:   "state list",
			first:  "state",
			second: "list",
			want: &compoundSubcommandResult{
				subCommand: "state list",
				argCount:   2,
			},
		},
		{
			name:   "state mv",
			first:  "state",
			second: "mv",
			want: &compoundSubcommandResult{
				subCommand: "state mv",
				argCount:   2,
			},
		},
		{
			name:   "state pull",
			first:  "state",
			second: "pull",
			want: &compoundSubcommandResult{
				subCommand: "state pull",
				argCount:   2,
			},
		},
		{
			name:   "state push",
			first:  "state",
			second: "push",
			want: &compoundSubcommandResult{
				subCommand: "state push",
				argCount:   2,
			},
		},
		{
			name:   "state replace-provider",
			first:  "state",
			second: "replace-provider",
			want: &compoundSubcommandResult{
				subCommand: "state replace-provider",
				argCount:   2,
			},
		},
		{
			name:   "state rm",
			first:  "state",
			second: "rm",
			want: &compoundSubcommandResult{
				subCommand: "state rm",
				argCount:   2,
			},
		},
		{
			name:   "state show",
			first:  "state",
			second: "show",
			want: &compoundSubcommandResult{
				subCommand: "state show",
				argCount:   2,
			},
		},
		{
			name:   "providers mirror",
			first:  "providers",
			second: "mirror",
			want: &compoundSubcommandResult{
				subCommand: "providers mirror",
				argCount:   2,
			},
		},
		{
			name:   "providers schema",
			first:  "providers",
			second: "schema",
			want: &compoundSubcommandResult{
				subCommand: "providers schema",
				argCount:   2,
			},
		},
		{
			name:   "workspace new",
			first:  "workspace",
			second: "new",
			want: &compoundSubcommandResult{
				subCommand:  "workspace",
				subCommand2: "new",
				argCount:    2,
			},
		},
		{
			name:   "workspace select",
			first:  "workspace",
			second: "select",
			want: &compoundSubcommandResult{
				subCommand:  "workspace",
				subCommand2: "select",
				argCount:    2,
			},
		},
		{
			name:   "workspace list",
			first:  "workspace",
			second: "list",
			want: &compoundSubcommandResult{
				subCommand:  "workspace",
				subCommand2: "list",
				argCount:    2,
			},
		},
		{
			name:   "workspace delete",
			first:  "workspace",
			second: "delete",
			want: &compoundSubcommandResult{
				subCommand:  "workspace",
				subCommand2: "delete",
				argCount:    2,
			},
		},
		{
			name:   "workspace show",
			first:  "workspace",
			second: "show",
			want: &compoundSubcommandResult{
				subCommand:  "workspace",
				subCommand2: "show",
				argCount:    2,
			},
		},
		{
			name:   "write varfile",
			first:  "write",
			second: "varfile",
			want: &compoundSubcommandResult{
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
		{
			name:    "providers with unknown subcommand",
			first:   "providers",
			second:  "unknown",
			wantNil: true,
		},
		{
			name:    "workspace with unknown subcommand",
			first:   "workspace",
			second:  "unknown",
			wantNil: true,
		},
		{
			name:    "write with unknown subcommand",
			first:   "write",
			second:  "unknown",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSeparateCompoundSubcommand(tt.first, tt.second)
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

// TestProcessTerraformCompoundSubcommand tests the processTerraformCompoundSubcommand helper function.
func TestProcessTerraformCompoundSubcommand(t *testing.T) {
	tests := []struct {
		name               string
		args               []string
		wantProcessed      bool
		wantErr            bool
		wantSubCommand     string
		wantSubCommand2    string
		wantComponent      string
		wantAdditional     []string
		wantPathResolution bool
	}{
		{
			name:          "not a compound subcommand",
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
		{
			name:               "providers lock with path component sets NeedsPathResolution",
			args:               []string{"providers", "lock", "./my-component"},
			wantProcessed:      true,
			wantErr:            false,
			wantSubCommand:     "providers lock",
			wantComponent:      "./my-component",
			wantPathResolution: true,
		},
		{
			name:           "providers lock with component and additional flags",
			args:           []string{"providers", "lock", "my-component", "-platform=linux_amd64", "-platform=darwin_amd64"},
			wantProcessed:  true,
			wantErr:        false,
			wantSubCommand: "providers lock",
			wantComponent:  "my-component",
			wantAdditional: []string{"-platform=linux_amd64", "-platform=darwin_amd64"},
		},
		{
			name:           "state list with component",
			args:           []string{"state", "list", "my-component"},
			wantProcessed:  true,
			wantErr:        false,
			wantSubCommand: "state list",
			wantComponent:  "my-component",
		},
		{
			name:           "state list quoted with component",
			args:           []string{"state list", "my-component"},
			wantProcessed:  true,
			wantErr:        false,
			wantSubCommand: "state list",
			wantComponent:  "my-component",
		},
		{
			name:          "state mv without component",
			args:          []string{"state", "mv"},
			wantProcessed: true,
			wantErr:       true,
		},
		{
			name:               "state show with path component sets NeedsPathResolution",
			args:               []string{"state", "show", "../my-component"},
			wantProcessed:      true,
			wantErr:            false,
			wantSubCommand:     "state show",
			wantComponent:      "../my-component",
			wantPathResolution: true,
		},
		{
			name:            "workspace select with component",
			args:            []string{"workspace", "select", "my-component"},
			wantProcessed:   true,
			wantErr:         false,
			wantSubCommand:  "workspace",
			wantSubCommand2: "select",
			wantComponent:   "my-component",
		},
		{
			name:          "workspace select without component",
			args:          []string{"workspace", "select"},
			wantProcessed: true,
			wantErr:       true,
		},
		{
			name:               "workspace list with path component sets NeedsPathResolution",
			args:               []string{"workspace", "list", absTestPath("abs/path/component")},
			wantProcessed:      true,
			wantErr:            false,
			wantSubCommand:     "workspace",
			wantSubCommand2:    "list",
			wantComponent:      absTestPath("abs/path/component"),
			wantPathResolution: true,
		},
		{
			name:            "workspace quoted select with component",
			args:            []string{"workspace select", "my-component"},
			wantProcessed:   true,
			wantErr:         false,
			wantSubCommand:  "workspace",
			wantSubCommand2: "select",
			wantComponent:   "my-component",
		},
		{
			name:            "write varfile with component",
			args:            []string{"write", "varfile", "my-component"},
			wantProcessed:   true,
			wantErr:         false,
			wantSubCommand:  "write",
			wantSubCommand2: "varfile",
			wantComponent:   "my-component",
		},
		{
			name:            "write varfile quoted with component",
			args:            []string{"write varfile", "my-component"},
			wantProcessed:   true,
			wantErr:         false,
			wantSubCommand:  "write",
			wantSubCommand2: "varfile",
			wantComponent:   "my-component",
		},
		{
			name:          "write varfile without component",
			args:          []string{"write", "varfile"},
			wantProcessed: true,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &schema.ArgsAndFlagsInfo{}
			processed, err := processTerraformCompoundSubcommand(info, tt.args)

			assert.Equal(t, tt.wantProcessed, processed)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			if tt.wantProcessed {
				assert.Equal(t, tt.wantSubCommand, info.SubCommand)
				assert.Equal(t, tt.wantSubCommand2, info.SubCommand2)
				assert.Equal(t, tt.wantComponent, info.ComponentFromArg)
				assert.Equal(t, tt.wantPathResolution, info.NeedsPathResolution)
				if tt.wantAdditional != nil {
					assert.Equal(t, tt.wantAdditional, info.AdditionalArgsAndFlags)
				} else {
					assert.Empty(t, info.AdditionalArgsAndFlags, "AdditionalArgsAndFlags should be empty")
				}
			}
		})
	}
}

// TestProcessSingleCommand tests the processSingleCommand helper function.
func TestProcessSingleCommand(t *testing.T) {
	tests := []struct {
		name               string
		args               []string
		wantErr            bool
		wantSubCommand     string
		wantComponent      string
		wantAdditional     []string
		wantPathResolution bool
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
			name:           "flag with additional args preserves all flags",
			args:           []string{"plan", "--help", "-var=foo"},
			wantErr:        false,
			wantSubCommand: "plan",
			wantAdditional: []string{"--help", "-var=foo"},
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
		{
			name:               "component with dot prefix sets NeedsPathResolution",
			args:               []string{"plan", "."},
			wantErr:            false,
			wantSubCommand:     "plan",
			wantComponent:      ".",
			wantPathResolution: true,
		},
		{
			name:               "component with relative path prefix sets NeedsPathResolution",
			args:               []string{"plan", "./vpc"},
			wantErr:            false,
			wantSubCommand:     "plan",
			wantComponent:      "./vpc",
			wantPathResolution: true,
		},
		{
			name:               "component with parent path prefix sets NeedsPathResolution",
			args:               []string{"plan", "../vpc"},
			wantErr:            false,
			wantSubCommand:     "plan",
			wantComponent:      "../vpc",
			wantPathResolution: true,
		},
		{
			// Windows-style relative path with backslash.
			name:               "component with backslash relative path sets NeedsPathResolution",
			args:               []string{"plan", `.\\vpc`},
			wantErr:            false,
			wantSubCommand:     "plan",
			wantComponent:      `.\\vpc`,
			wantPathResolution: true,
		},
		{
			// Windows-style parent path with backslash.
			name:               "component with backslash parent path sets NeedsPathResolution",
			args:               []string{"plan", `..\\vpc`},
			wantErr:            false,
			wantSubCommand:     "plan",
			wantComponent:      `..\\vpc`,
			wantPathResolution: true,
		},
		{
			name:               "component with absolute path sets NeedsPathResolution",
			args:               []string{"plan", absTestPath("absolute/path/component")},
			wantErr:            false,
			wantSubCommand:     "plan",
			wantComponent:      absTestPath("absolute/path/component"),
			wantPathResolution: true,
		},
		{
			// Forward slashes in component names are namespace separators, not path indicators.
			name:               "component with slash but no path prefix does not set NeedsPathResolution",
			args:               []string{"plan", "infra/vpc"},
			wantErr:            false,
			wantSubCommand:     "plan",
			wantComponent:      "infra/vpc",
			wantPathResolution: false,
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
			assert.Equal(t, tt.wantPathResolution, info.NeedsPathResolution, "NeedsPathResolution mismatch")
			if tt.wantAdditional != nil {
				assert.Equal(t, tt.wantAdditional, info.AdditionalArgsAndFlags)
			} else {
				assert.Empty(t, info.AdditionalArgsAndFlags, "AdditionalArgsAndFlags should be empty")
			}
		})
	}
}
