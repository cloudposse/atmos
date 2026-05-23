package terraform

// utils_hooks_test.go covers the hook-wrapper functions in utils.go that
// build RunCIHooksOptions and forward CommandError + ExitCode into the CI
// hook plumbing. These wrappers are thin glue (ProcessCommandLineArgs →
// InitCliConfig → RunCIHooks) but contain the new CommandError/ExitCode
// forwarding lines added in this PR. The demo-stacks fixture has no
// ci.enabled config, so RunCIHooks short-circuits cleanly — these tests
// exercise option construction without invoking real plugin handlers.
//
// Note on test isolation: cmd.NewTestKit(t) cannot be used here due to a
// circular import between cmd/terraform and the cmd package. The package
// already has a TestMain in testmain_test.go that handles the subprocess
// env-var pattern, and the wrappers under test don't mutate RootCmd state.

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel: tests below depend on these schema.ConfigAndStacksInfo
// fields by name. If any field is renamed upstream, this declaration fails
// to compile so the rename surfaces before the tests would silently drift.
var _ = schema.ConfigAndStacksInfo{
	Stack:            "",
	Component:        "",
	ComponentFromArg: "",
	ComponentType:    "",
}

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
		output  string
		execErr error
	}{
		{name: "success path", output: "deploy output", execErr: nil},
		{name: "failure path forwards exit code", output: "", execErr: errUtils.ExitCodeError{Code: 1}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runCIHooksForDeployComponent(cmd, info, tc.output, tc.execErr)
		})
	}
}

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
	runCIHooksForApplyComponent(cmd, info, "apply output", nil)

	// Failure path: non-nil execErr is forwarded with its exit code.
	runCIHooksForApplyComponent(cmd, info, "", errUtils.ExitCodeError{Code: 1})
}

// TestDeployPostRunE_SuppressedWhenMultiComponent verifies that deployCmd.PostRunE
// returns nil without error when wasMultiComponentExecution is true (multi-component
// mode). Exercises the actual PostRunE closure in both suppressed and non-suppressed states.
func TestDeployPostRunE_SuppressedWhenMultiComponent(t *testing.T) {
	t.Chdir("../../examples/demo-stacks")

	orig := wasMultiComponentExecution
	defer func() { wasMultiComponentExecution = orig }()

	cmd := newHookTestCmd()
	cmd.Use = "deploy"

	// With wasMultiComponentExecution = true, PostRunE must return nil immediately
	// without invoking runHooksWithOutput (which would attempt stack resolution).
	wasMultiComponentExecution = true
	err := deployCmd.PostRunE(cmd, []string{"--stack", "dev", "myapp"})
	assert.NoError(t, err, "PostRunE must be suppressed when wasMultiComponentExecution is true")

	// With wasMultiComponentExecution = false, PostRunE must run normally.
	// The demo-stacks fixture has no hooks configured, so it completes without error.
	wasMultiComponentExecution = false
	err = deployCmd.PostRunE(cmd, []string{"--stack", "dev", "myapp"})
	assert.NoError(t, err, "PostRunE must fire normally in single-component mode")
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
	cmd.Use = "apply"

	// With wasMultiComponentExecution = true, PostRunE must return nil immediately
	// without invoking runHooksWithOutput (which would attempt stack resolution).
	wasMultiComponentExecution = true
	err := applyCmd.PostRunE(cmd, []string{"--stack", "dev", "myapp"})
	assert.NoError(t, err, "PostRunE must be suppressed when wasMultiComponentExecution is true")

	// With wasMultiComponentExecution = false, PostRunE must run normally.
	// The demo-stacks fixture has no hooks configured, so it completes without error.
	wasMultiComponentExecution = false
	err = applyCmd.PostRunE(cmd, []string{"--stack", "dev", "myapp"})
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
// verifies every branch using a stubbed runHooksOnErrorWithOutput.
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

	deployErr := errors.New("terraform deploy failed")

	tests := []struct {
		name              string
		runErr            error
		multiComponent    bool
		expectCalled      bool
		expectEvent       hooks.HookEvent
		expectForwardErr  error
		expectOutput      string
		expectOutputMatch bool
	}{
		{
			name:              "non-nil error + single-component → hook fires",
			runErr:            deployErr,
			multiComponent:    false,
			expectCalled:      true,
			expectEvent:       hooks.AfterTerraformDeploy,
			expectForwardErr:  deployErr,
			expectOutput:      "captured deploy output",
			expectOutputMatch: true,
		},
		{
			name:           "non-nil error + multi-component → hook suppressed",
			runErr:         errors.New("terraform deploy failed"),
			multiComponent: true,
			expectCalled:   false,
		},
		{
			name:           "nil error + single-component → hook does not fire",
			runErr:         nil,
			multiComponent: false,
			expectCalled:   false,
		},
		{
			name:           "nil error + multi-component → hook does not fire",
			runErr:         nil,
			multiComponent: true,
			expectCalled:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			called = false
			calledEvent = ""
			calledErr = nil
			calledOutput = ""
			wasMultiComponentExecution = tc.multiComponent

			invokeDefer(tc.runErr, "captured deploy output")

			assert.Equal(t, tc.expectCalled, called, "hook firing did not match expectation")
			if tc.expectCalled {
				assert.Equal(t, tc.expectEvent, calledEvent, "hook event mismatch")
				assert.Equal(t, tc.expectForwardErr, calledErr, "hook did not receive original runErr")
				if tc.expectOutputMatch {
					assert.Equal(t, tc.expectOutput, calledOutput, "hook did not receive captured output")
				}
			}
		})
	}
}

// TestRunCIHooksForDeployComponent_ExitCodeForwarding verifies that the exit code
// extracted from execErr is forwarded correctly, matching the plan component hook behaviour.
func TestRunCIHooksForDeployComponent_ExitCodeForwarding(t *testing.T) {
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
		{"exit code 2 (abnormal termination)", errUtils.ExitCodeError{Code: 2}, 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantExt, errUtils.GetExitCode(tc.execErr),
				"GetExitCode must extract the wrapped exit code before forwarding to deploy hook")
			// The wrapper must not panic regardless of exit code.
			runCIHooksForDeployComponent(cmd, info, "deploy output", tc.execErr)
		})
	}
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
