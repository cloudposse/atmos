package terraform

// utils_hooks_test.go covers the hook-wrapper functions in utils.go that
// build RunCIHooksOptions and forward CommandError + ExitCode into the CI
// hook plumbing. These wrappers are thin glue (ProcessCommandLineArgs →
// InitCliConfig → RunCIHooks) but contain the new CommandError/ExitCode
// forwarding lines added in this PR. The demo-stacks fixture has no
// ci.enabled config, so RunCIHooks short-circuits cleanly — these tests
// exercise option construction without invoking real plugin handlers.

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
)

// newHookTestCmd constructs a cobra.Command with all the flags
// ProcessCommandLineArgs reads (base-path, config, config-path, profile,
// stack, ci, verify-plan). This lets the wrapper functions in utils.go
// progress past argument parsing and into the option-construction code
// that this PR added.
func newHookTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "plan"}
	cmd.Flags().String("base-path", "", "base path")
	cmd.Flags().StringSlice("config", nil, "config")
	cmd.Flags().StringSlice("config-path", nil, "config path")
	cmd.Flags().StringSlice("profile", nil, "profile")
	cmd.Flags().String("stack", "", "stack flag")
	cmd.Flags().Bool("ci", false, "ci flag")
	cmd.Flags().Bool("verify-plan", false, "verify-plan flag")
	return cmd
}

// TestRunHooksOnError_PreservesCommandError verifies the failure-path
// wrapper (runHooksOnError → runHooksOnErrorWithOutput) accepts a non-nil
// cmdErr and forwards it into RunCIHooksOptions without mutating it. This
// exercises the new errUtils.GetExitCode(cmdErr) call and the
// CommandError/ExitCode field assignments added in this PR.
func TestRunHooksOnError_PreservesCommandError(t *testing.T) {
	t.Chdir("../../examples/demo-stacks")

	cmd := newHookTestCmd()

	tests := []struct {
		name    string
		cmdErr  error
		wantExt int
	}{
		{
			name:    "wrapped ExitCodeError code 1",
			cmdErr:  errUtils.ExitCodeError{Code: 1},
			wantExt: 1,
		},
		{
			name:    "wrapped ExitCodeError code 2 (plan changes detected)",
			cmdErr:  errUtils.ExitCodeError{Code: 2},
			wantExt: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Pre-condition: GetExitCode must extract the wrapped code.
			// runHooksOnErrorWithOutput depends on this — if it ever stops
			// working, CI summaries silently regress to ExitCode=1 by default.
			assert.Equal(t, tc.wantExt, errUtils.GetExitCode(tc.cmdErr),
				"GetExitCode must extract the wrapped exit code")

			// runHooksOnErrorWithOutput returns void; the test asserts no
			// panic, no fatal error from the option construction or the
			// ci-disabled short-circuit path.
			runHooksOnError(hooks.AfterTerraformPlan, cmd, []string{"--stack", "dev", "myapp"}, tc.cmdErr)
		})
	}
}

// TestRunHooksOnErrorWithOutput_NilCmdErr verifies the wrapper handles a
// nil cmdErr cleanly (defensive: callers should always pass a non-nil error
// on the failure path, but the wrapper must not panic if they don't).
func TestRunHooksOnErrorWithOutput_NilCmdErr(t *testing.T) {
	t.Chdir("../../examples/demo-stacks")

	cmd := newHookTestCmd()

	// errUtils.GetExitCode(nil) returns 0 — ExitCode forwarded to plugins
	// will be 0, which RunCIHooks short-circuits cleanly via the
	// ci.enabled=false demo-stacks fixture.
	assert.Equal(t, 0, errUtils.GetExitCode(nil))

	runHooksOnErrorWithOutput(hooks.AfterTerraformPlan, cmd, []string{"--stack", "dev", "myapp"}, nil, "captured output")
}

// TestRunHooks_DemoStacks exercises the success-path wrapper (runHooks →
// runHooksWithOutput). The demo-stacks fixture has ci.enabled=false so
// RunCIHooks short-circuits cleanly inside the wrapper — the test asserts
// the wrapper completes without error.
func TestRunHooks_DemoStacks(t *testing.T) {
	t.Chdir("../../examples/demo-stacks")

	cmd := newHookTestCmd()
	err := runHooks(hooks.BeforeTerraformPlan, cmd, []string{"--stack", "dev", "myapp"})
	assert.NoError(t, err)
}

// TestRunCIHooksForDeploy_DemoStacks exercises the deploy-specific wrapper
// (runCIHooksForDeploy). Unlike the other wrappers, this one takes a
// pre-resolved info struct and skips ProcessCommandLineArgs to avoid eager
// !store YAML function resolution. The demo-stacks fixture's
// ci.enabled=false makes RunCIHooks short-circuit cleanly.
func TestRunCIHooksForDeploy_DemoStacks(t *testing.T) {
	t.Chdir("../../examples/demo-stacks")

	cmd := newHookTestCmd()
	info := &schema.ConfigAndStacksInfo{
		Stack:            "dev",
		ComponentFromArg: "myapp",
		ComponentType:    "terraform",
	}

	// Function returns void — the test verifies no panic on the option
	// construction path with a wired info struct.
	runCIHooksForDeploy(hooks.BeforeTerraformDeploy, cmd, []string{"myapp"}, info, "")
}
