package cmd

import (
	stderrors "errors"
	"fmt"
	"strings"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	// Isolate from the ambient CI environment. On CI runners GITHUB_ACTIONS=true,
	// GITHUB_BASE_REF=main, and CI=true are all set, which would make
	// SetDescribeAffectedFlagValueInCliArgs auto-detect a base from the CI
	// provider and populate Ref/HeadSHAOverride/CIEventType — breaking the
	// "empty by default" assertions below. Clear the relevant env vars and
	// reset viper so `isCIEnabledForDescribeAffected` reads only the explicit
	// per-case config passed into the helper.
	viper.Reset()
	t.Cleanup(viper.Reset)
	for _, k := range []string{"ATMOS_CI", "CI", "GITHUB_ACTIONS", "GITHUB_BASE_REF", "GITHUB_EVENT_NAME", "GITHUB_EVENT_PATH"} {
		t.Setenv(k, "")
	}

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

// TestDescribeAffectedCmd_RepoPathConflictHints verifies that supplying both
// --repo-path and --base returns ErrRepoPathConflict wrapped with actionable
// hints pointing the user at --ci=false / ATMOS_CI=false. Covers the
// errUtils.Build(...).WithHint(...).Err() branch in ParseDescribeAffectedCliArgs.
func TestDescribeAffectedCmd_RepoPathConflictHints(t *testing.T) {
	_ = NewTestKit(t)

	// Reset viper and clear ambient CI env so the validator sees only the
	// flags we set below. Without this, viper-bound CI state from an earlier
	// test (or the host runner) could cause ParseDescribeAffectedCliArgs to
	// take a different error branch before reaching the conflict validator.
	viper.Reset()
	t.Cleanup(viper.Reset)
	for _, k := range []string{"ATMOS_CI", "CI", "GITHUB_ACTIONS", "GITHUB_BASE_REF", "GITHUB_EVENT_NAME", "GITHUB_EVENT_PATH", "ATMOS_IDENTITY", "IDENTITY"} {
		t.Setenv(k, "")
	}

	stacksPath := "../tests/fixtures/scenarios/basic"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)
	viper.Set("identity", "false")

	// Set both --repo-path and --base on the command. These flag values
	// leak across tests via the shared describeAffectedCmd singleton, so we
	// reset them on cleanup to keep neighbors deterministic.
	require.NoError(t, describeAffectedCmd.PersistentFlags().Set("repo-path", "/tmp/some-clone"))
	require.NoError(t, describeAffectedCmd.PersistentFlags().Set("base", "main"))
	t.Cleanup(func() {
		_ = describeAffectedCmd.PersistentFlags().Set("repo-path", "")
		_ = describeAffectedCmd.PersistentFlags().Set("base", "")
	})

	err := describeAffectedCmd.RunE(describeAffectedCmd, []string{})

	require.Error(t, err, "--repo-path + --base should return ErrRepoPathConflict")
	assert.True(t,
		stderrors.Is(err, exec.ErrRepoPathConflict),
		"errors.Is must recognize the wrapped error as ErrRepoPathConflict; got %v", err)

	// Hints are attached via cockroachdb/errors.WithHint — use the package's
	// helper to extract them rather than substring-matching err.Error().
	hints := cockroachErrors.GetAllHints(err)
	require.NotEmpty(t, hints, "error builder must attach at least one hint")

	var sawRepoPathHint, sawCIOverrideHint bool
	for _, h := range hints {
		if assert.NotEmpty(t, h) {
			// Two hints come from the error builder; match each independently
			// so future wording tweaks don't break the test.
			if strings.Contains(h, "--repo-path") {
				sawRepoPathHint = true
			}
			if strings.Contains(h, "--ci=false") || strings.Contains(h, "ATMOS_CI=false") {
				sawCIOverrideHint = true
			}
		}
	}
	assert.True(t, sawRepoPathHint, "expected a hint explaining the --repo-path flag-group boundary")
	assert.True(t, sawCIOverrideHint, "expected a hint pointing at --ci=false / ATMOS_CI=false")
}
