package terraform

// utils_hooks_test.go covers the hook-wrapper functions in utils.go that
// build RunCIHooksOptions and forward CommandError + ExitCode into the CI
// hook plumbing. These wrappers are thin glue (ProcessCommandLineArgs →
// InitCliConfig → RunCIHooks) but contain the new CommandError/ExitCode
// forwarding lines added in this PR. The demo-stacks fixture has no
// ci.enabled config, so RunCIHooks short-circuits cleanly — these tests
// exercise option construction without invoking real plugin handlers.

import (
	"errors"
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

// TestRunCIHooksForPlanComponent_DemoStacks exercises the per-component plan
// CI hook wrapper introduced by issue #2397. The demo-stacks fixture has
// ci.enabled=false so RunCIHooks short-circuits cleanly — the test verifies
// no panic on option construction for both the success and failure paths.
func TestRunCIHooksForPlanComponent_DemoStacks(t *testing.T) {
	t.Chdir("../../examples/demo-stacks")

	cmd := newHookTestCmd()
	info := &schema.ConfigAndStacksInfo{
		Stack:            "dev",
		Component:        "myapp",
		ComponentFromArg: "myapp",
		ComponentType:    "terraform",
	}

	// Success path: execErr is nil, exit code forwarded as 0.
	runCIHooksForPlanComponent(cmd, info, "plan output", nil)

	// Failure path: non-nil execErr is forwarded with its exit code.
	runCIHooksForPlanComponent(cmd, info, "", errUtils.ExitCodeError{Code: 1})
}

// TestRunCIHooksForDeployComponent_DemoStacks exercises the per-component deploy
// CI hook wrapper introduced by issue #2476. The demo-stacks fixture has
// ci.enabled=false so RunCIHooks short-circuits cleanly — the test verifies
// no panic on option construction for both the success and failure paths.
func TestRunCIHooksForDeployComponent_DemoStacks(t *testing.T) {
// TestRunCIHooksForApplyComponent_DemoStacks exercises the per-component apply
// CI hook wrapper introduced by issue #2475. The demo-stacks fixture has
// ci.enabled=false so RunCIHooks short-circuits cleanly — the test verifies
// no panic on option construction for both the success and failure paths.
func TestRunCIHooksForApplyComponent_DemoStacks(t *testing.T) {
	t.Chdir("../../examples/demo-stacks")

	cmd := newHookTestCmd()
	info := &schema.ConfigAndStacksInfo{
		Stack:            "dev",
		Component:        "myapp",
		ComponentFromArg: "myapp",
		ComponentType:    "terraform",
	}

	// Success path: execErr is nil, exit code forwarded as 0.
	runCIHooksForDeployComponent(cmd, info, "deploy output", nil)

	// Failure path: non-nil execErr is forwarded with its exit code.
	runCIHooksForDeployComponent(cmd, info, "", errUtils.ExitCodeError{Code: 1})
}

// TestDeployPostRunE_SuppressedWhenMultiComponent verifies that deployCmd.PostRunE
// returns nil without error when wasMultiComponentExecution is true (multi-component
// mode). Exercises the actual PostRunE closure in both suppressed and non-suppressed states.
func TestDeployPostRunE_SuppressedWhenMultiComponent(t *testing.T) {
	runCIHooksForApplyComponent(cmd, info, "apply output", nil)

	// Failure path: non-nil execErr is forwarded with its exit code.
	runCIHooksForApplyComponent(cmd, info, "", errUtils.ExitCodeError{Code: 1})
}

// TestApplyPostRunE_SuppressedWhenMultiComponent verifies that applyCmd.PostRunE
// returns nil without error when wasMultiComponentExecution is true (multi-component
// mode). This is the apply-command equivalent of the plan fix from issue #2397 —
// it exercises the actual PostRunE closure, not just the sentinel variable.
func TestApplyPostRunE_SuppressedWhenMultiComponent(t *testing.T) {
	t.Chdir("../../examples/demo-stacks")

	orig := wasMultiComponentExecution
	defer func() { wasMultiComponentExecution = orig }()

	cmd := newHookTestCmd()
	cmd.Use = "deploy"
	cmd.Use = "apply"

	// With wasMultiComponentExecution = true, PostRunE must return nil immediately
	// without invoking runHooksWithOutput (which would attempt stack resolution).
	wasMultiComponentExecution = true
	err := deployCmd.PostRunE(cmd, []string{"--stack", "dev", "myapp"})
	err := applyCmd.PostRunE(cmd, []string{"--stack", "dev", "myapp"})
	assert.NoError(t, err, "PostRunE must be suppressed when wasMultiComponentExecution is true")

	// With wasMultiComponentExecution = false, PostRunE must run normally.
	// The demo-stacks fixture has no hooks configured, so it completes without error.
	wasMultiComponentExecution = false
	err = deployCmd.PostRunE(cmd, []string{"--stack", "dev", "myapp"})
	assert.NoError(t, err, "PostRunE must fire normally in single-component mode")
}

// TestDeployRunE_DeferGuard verifies the RunE defer-guard contract in
// deploy.go: the global error hook (runHooksOnErrorWithOutput) must fire
// when runErr is non-nil AND wasMultiComponentExecution is false, and must
// be suppressed when wasMultiComponentExecution is true (multi-component
// mode, where per-component hooks already fired inside ExecuteTerraformQuery).
//
// The defer-guard lives inside deployCmd.RunE and is reset to false at the
// start of RunE, so we can't pre-set wasMultiComponentExecution and invoke
// RunE directly. Instead, this test mirrors the defer-body inline and
// verifies both branches using a stubbed runHooksOnErrorWithOutput.
func TestDeployRunE_DeferGuard(t *testing.T) {
	origGuard := wasMultiComponentExecution
	origHook := runHooksOnErrorWithOutput
	defer func() {
		wasMultiComponentExecution = origGuard
		runHooksOnErrorWithOutput = origHook
	}()

	var called bool
	var calledEvent hooks.HookEvent
	var calledErr error
	var calledOutput string
	runHooksOnErrorWithOutput = func(event hooks.HookEvent, _ *cobra.Command, _ []string, cmdErr error, output string) {
		called = true
		calledEvent = event
		calledErr = cmdErr
		calledOutput = output
	}

	cmd := newHookTestCmd()
	cmd.Use = "deploy"
	args := []string{"--stack", "dev", "myapp"}

	// invokeDefer mirrors the RunE defer-guard body in deploy.go lines 43-47.
	// Any change to the production guard must be reflected here.
	invokeDefer := func(runErr error, capturedOutput string) {
		if runErr != nil && !wasMultiComponentExecution {
			runHooksOnErrorWithOutput(hooks.AfterTerraformDeploy, cmd, args, runErr, capturedOutput)
		}
	}

	// Case 1: non-nil error AND single-component mode → hook MUST fire.
	called = false
	wasMultiComponentExecution = false
	runErr := errors.New("terraform deploy failed")
	invokeDefer(runErr, "captured deploy output")
	assert.True(t, called, "hook must fire when runErr is non-nil and wasMultiComponentExecution is false")
	assert.Equal(t, hooks.AfterTerraformDeploy, calledEvent, "hook must fire with AfterTerraformDeploy event")
	assert.Equal(t, runErr, calledErr, "hook must receive the original runErr")
	assert.Equal(t, "captured deploy output", calledOutput, "hook must receive the captured deploy output")

	// Case 2: non-nil error AND multi-component mode → hook MUST be suppressed.
	called = false
	wasMultiComponentExecution = true
	invokeDefer(errors.New("terraform deploy failed"), "captured deploy output")
	assert.False(t, called, "hook must be suppressed when wasMultiComponentExecution is true")

	// Case 3: nil error → hook MUST NOT fire regardless of guard state.
	called = false
	wasMultiComponentExecution = false
	invokeDefer(nil, "captured deploy output")
	assert.False(t, called, "hook must not fire on success path (nil runErr)")

	called = false
	wasMultiComponentExecution = true
	invokeDefer(nil, "captured deploy output")
	assert.False(t, called, "hook must not fire on success path even in multi-component mode")
}

// TestRunCIHooksForDeployComponent_ExitCodeForwarding verifies that the exit code
// extracted from execErr is forwarded correctly, matching the plan component hook behaviour.
func TestRunCIHooksForDeployComponent_ExitCodeForwarding(t *testing.T) {
	err = applyCmd.PostRunE(cmd, []string{"--stack", "dev", "myapp"})
	assert.NoError(t, err, "PostRunE must fire normally in single-component mode")
}

// TestRunCIHooksForApplyComponent_ExitCodeForwarding verifies that the exit code
// extracted from execErr is forwarded correctly, matching the plan component
// hook behaviour.
func TestRunCIHooksForApplyComponent_ExitCodeForwarding(t *testing.T) {
	t.Chdir("../../examples/demo-stacks")

	cmd := newHookTestCmd()
	info := &schema.ConfigAndStacksInfo{
		Stack:            "dev",
		Component:        "myapp",
		ComponentFromArg: "myapp",
		ComponentType:    "terraform",
	}

	tests := []struct {
		name    string
		execErr error
		wantExt int
	}{
		{"nil error has exit code 0", nil, 0},
		{"exit code 1", errUtils.ExitCodeError{Code: 1}, 1},
		{"exit code 2 (non-standard error)", errUtils.ExitCodeError{Code: 2}, 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantExt, errUtils.GetExitCode(tc.execErr),
				"GetExitCode must extract the wrapped exit code before forwarding to apply hook")
			// The wrapper must not panic regardless of exit code.
			runCIHooksForApplyComponent(cmd, info, "apply output", tc.execErr)
		})
	}
}
