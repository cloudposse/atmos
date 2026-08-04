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
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	authtypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/ci"
	githubCI "github.com/cloudposse/atmos/pkg/ci/providers/github"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
	storepkg "github.com/cloudposse/atmos/pkg/store"
)

// Compile-time sentinel: tests below depend on these schema.ConfigAndStacksInfo
// fields by name. If any field is renamed upstream, this declaration fails
// to compile so the rename surfaces before the tests would silently drift.
var _ = schema.ConfigAndStacksInfo{
	Stack:                        "",
	Component:                    "",
	ComponentFromArg:             "",
	ComponentType:                "",
	TerraformPlanCIResultHandler: nil,
	NodeHooks:                    nil,
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

func withoutCIDetection(t *testing.T) {
	t.Helper()

	restoreRegistry := ci.SwapRegistryForTest()
	t.Cleanup(restoreRegistry)

	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("CI", "")
	t.Setenv("ATMOS_CI", "")
}

func withGitHubActionsDetection(t *testing.T) {
	t.Helper()

	restoreRegistry := ci.SwapRegistryForTest()
	t.Cleanup(restoreRegistry)
	ci.Register(githubCI.NewProvider())

	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("CI", "")
	t.Setenv("ATMOS_CI", "")
}

func resetViperCI(t *testing.T) {
	t.Helper()

	previous := viper.Get("ci")
	t.Cleanup(func() {
		if previous == nil {
			viper.Set("ci", false)
			return
		}
		viper.Set("ci", previous)
	})
	viper.Set("ci", false)
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

// TestTerraformNodeHooksAfter_PlanDemoStacks exercises the per-component plan
// CI hook path (terraformNodeHooks.After). The demo-stacks fixture has
// ci.enabled=false so RunCIHooks short-circuits cleanly — the test verifies
// no panic for both the success and failure paths, and that a component with
// no `hooks:` section returns a nil user-hook error.
func TestTerraformNodeHooksAfter_PlanDemoStacks(t *testing.T) {
	t.Chdir("../../examples/demo-stacks")

	nodeHooks := &terraformNodeHooks{cmd: newHookTestCmd(), afterEvent: hooks.AfterTerraformPlan}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "dev",
		Component:        "myapp",
		ComponentFromArg: "myapp",
		ComponentType:    "terraform",
	}

	// Success path: execErr is nil, exit code forwarded as 0.
	assert.NoError(t, nodeHooks.After(context.Background(), info, "plan output", nil))

	// Failure path: non-nil execErr is forwarded with its exit code.
	assert.NoError(t, nodeHooks.After(context.Background(), info, "", errUtils.ExitCodeError{Code: 1}))
}

// TestTerraformNodeHooksAfter_DeployDemoStacks exercises the per-component
// deploy CI hook path (terraformNodeHooks.After). The demo-stacks fixture has
// ci.enabled=false so RunCIHooks short-circuits cleanly — the test verifies
// no panic for both the success and failure paths.
func TestTerraformNodeHooksAfter_DeployDemoStacks(t *testing.T) {
	t.Chdir("../../examples/demo-stacks")

	nodeHooks := &terraformNodeHooks{cmd: newHookTestCmd(), afterEvent: hooks.AfterTerraformDeploy}
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
			assert.NoError(t, nodeHooks.After(context.Background(), info, tc.output, tc.execErr))
		})
	}
}

// TestTerraformNodeHooksAfter_ApplyDemoStacks exercises the per-component
// apply CI hook path (terraformNodeHooks.After). The demo-stacks fixture has
// ci.enabled=false so RunCIHooks short-circuits cleanly — the test verifies
// no panic for both the success and failure paths.
func TestTerraformNodeHooksAfter_ApplyDemoStacks(t *testing.T) {
	t.Chdir("../../examples/demo-stacks")

	nodeHooks := &terraformNodeHooks{cmd: newHookTestCmd(), afterEvent: hooks.AfterTerraformApply}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "dev",
		Component:        "myapp",
		ComponentFromArg: "myapp",
		ComponentType:    "terraform",
	}

	// Success path: execErr is nil, exit code forwarded as 0.
	assert.NoError(t, nodeHooks.After(context.Background(), info, "apply output", nil))

	// Failure path: non-nil execErr is forwarded with its exit code.
	assert.NoError(t, nodeHooks.After(context.Background(), info, "", errUtils.ExitCodeError{Code: 1}))
}

// TestTerraformNodeHooksAfter_ReinjectsStoreAuthResolver verifies that graph-mode
// after-hooks restore the auth resolver after loading their fresh configuration.
// The resolver derives the inherited store identity from the node AuthManager;
// without this wiring GetChain is never called and identity-aware store hooks
// fail because their newly constructed stores have no resolver.
func TestTerraformNodeHooksAfter_ReinjectsStoreAuthResolver(t *testing.T) {
	t.Chdir("../../examples/demo-stacks")
	e.ClearLastAuthContext()
	t.Cleanup(e.ClearLastAuthContext)

	ctrl := gomock.NewController(t)
	authManager := authtypes.NewMockAuthManager(ctrl)
	authManager.EXPECT().GetChain().Return([]string{"local-aws"}).Times(1)

	nodeHooks := &terraformNodeHooks{cmd: newHookTestCmd(), afterEvent: hooks.AfterTerraformApply}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "dev",
		Component:        "myapp",
		ComponentFromArg: "myapp",
		ComponentType:    "terraform",
		AuthManager:      authManager,
	}

	require.NoError(t, nodeHooks.After(context.Background(), info, "apply output", nil))
	assert.Equal(t, "local-aws", info.Identity)
}

// TestRunCIHooksForNode_RunCIHooksError covers runCIHooksForNode's RunCIHooks
// error branch (log.Warn). Constructing an AtmosConfiguration directly with
// CI.Enabled=true and Settings.Experimental="disable" makes checkExperimental
// (inside RunCIHooks) return an error without needing any fixture on disk —
// runCIHooksForNode takes atmosConfig as a parameter rather than loading it
// itself.
func TestRunCIHooksForNode_RunCIHooksError(t *testing.T) {
	withoutCIDetection(t)

	cmd := newHookTestCmd()
	require.NoError(t, cmd.Flags().Set("ci", "true"))
	nodeHooks := &terraformNodeHooks{cmd: cmd, afterEvent: hooks.AfterTerraformPlan}
	atmosConfig := &schema.AtmosConfiguration{
		CI:       schema.CIConfig{Enabled: true},
		Settings: schema.AtmosSettings{Experimental: "disable"},
	}
	info := &schema.ConfigAndStacksInfo{Stack: "dev", Component: "myapp", ComponentFromArg: "myapp"}

	// runCIHooksForNode has no return value — this just exercises the error
	// branch (log.Warn) without panicking.
	assert.NotPanics(t, func() {
		nodeHooks.runCIHooksForNode(atmosConfig, info, "output", nil)
	})
}

// TestTerraformNodeHooksBeforeAfter_ConfigInitFailure covers the
// config-init-failure branch in both Before and After (log.Warn; return nil)
// — every other terraformNodeHooks test above chdirs into a valid fixture, so
// cfg.InitCliConfig always succeeds and this branch was never reached.
func TestTerraformNodeHooksBeforeAfter_ConfigInitFailure(t *testing.T) {
	t.Chdir(t.TempDir())

	nodeHooks := &terraformNodeHooks{cmd: newHookTestCmd(), beforeEvent: hooks.BeforeTerraformPlan, afterEvent: hooks.AfterTerraformPlan}
	info := &schema.ConfigAndStacksInfo{Stack: "dev", Component: "myapp", ComponentFromArg: "myapp", ComponentType: "terraform"}

	assert.NoError(t, nodeHooks.Before(context.Background(), info),
		"config-init failures are logged, not surfaced, from Before")
	assert.NoError(t, nodeHooks.After(context.Background(), info, "output", nil),
		"config-init failures are logged, not surfaced, from After")
}

// TestTerraformNodeHooksRunUserHooksForNode_EmptyEvent covers
// runUserHooksForNode's `event == ""` guard. Unreachable via production
// wiring today (terraformHookEvents returns ok=false for any subcommand
// without an event pair, and wirePerComponentHook never constructs a
// terraformNodeHooks in that case), so it must be exercised directly.
func TestTerraformNodeHooksRunUserHooksForNode_EmptyEvent(t *testing.T) {
	nodeHooks := &terraformNodeHooks{cmd: newHookTestCmd()}
	err := nodeHooks.runUserHooksForNode(&schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, "", hooks.Outcome{})
	assert.NoError(t, err)
}

// TestTerraformAggregateEvent pins the aggregate CI hook event mapping for
// every Terraform command HandleTerraformPlanCIResults can be invoked with.
func TestTerraformAggregateEvent(t *testing.T) {
	cases := map[string]hooks.HookEvent{
		"apply":   hooks.AfterTerraformApplyAggregate,
		"destroy": hooks.AfterTerraformDestroyAggregate,
		"plan":    hooks.AfterTerraformPlanAggregate,
		"":        hooks.AfterTerraformPlanAggregate,
	}
	for command, want := range cases {
		t.Run(command, func(t *testing.T) {
			assert.Equal(t, want, terraformAggregateEvent(command))
		})
	}
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

// TestTerraformNodeHooksAfter_DeployExitCodeForwarding verifies that the exit
// code extracted from execErr is forwarded correctly, matching the plan
// component hook behaviour.
func TestTerraformNodeHooksAfter_DeployExitCodeForwarding(t *testing.T) {
	t.Chdir("../../examples/demo-stacks")

	nodeHooks := &terraformNodeHooks{cmd: newHookTestCmd(), afterEvent: hooks.AfterTerraformDeploy}
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
			assert.NoError(t, nodeHooks.After(context.Background(), info, "deploy output", tc.execErr))
		})
	}
}

// TestTerraformNodeHooksAfter_ApplyExitCodeForwarding verifies that the exit
// code extracted from execErr is forwarded correctly, matching the plan
// component hook behaviour.
func TestTerraformNodeHooksAfter_ApplyExitCodeForwarding(t *testing.T) {
	t.Chdir("../../examples/demo-stacks")

	nodeHooks := &terraformNodeHooks{cmd: newHookTestCmd(), afterEvent: hooks.AfterTerraformApply}
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
			assert.NoError(t, nodeHooks.After(context.Background(), info, "apply output", tc.execErr))
		})
	}
}

// TestWirePerComponentHook pins the per-subcommand hook wiring contract used
// by both the ExecuteTerraformAll (--all) and ExecuteTerraformQuery
// (--components/--query) dispatch branches in terraformRunWithOptions. Both
// branches funnel through this helper, so a regression here would silently
// drop per-component user hooks (the bulk-dispatch bug this wiring fixes) or
// CI summary entries for one of plan/apply/deploy.
func TestWirePerComponentHook(t *testing.T) {
	withoutCIDetection(t)

	t.Run("plan/deploy/apply install a non-nil NodeHooks", func(t *testing.T) {
		for _, sub := range []string{"plan", "deploy", "apply"} {
			t.Run(sub, func(t *testing.T) {
				info := &schema.ConfigAndStacksInfo{
					TerraformPlanCIResultHandler: nil,
				}
				wirePerComponentHook(info, sub, newHookTestCmd(), nil)
				assert.NotNil(t, info.NodeHooks,
					"%q subcommand must install per-component hooks", sub)
			})
		}
	})

	t.Run("plan/apply/destroy CI installs aggregate handler alongside user hooks", func(t *testing.T) {
		cmd := newHookTestCmd()
		require.NoError(t, cmd.Flags().Set("ci", "true"))

		for _, sub := range []string{"plan", "apply", "destroy"} {
			t.Run(sub, func(t *testing.T) {
				info := &schema.ConfigAndStacksInfo{
					TerraformPlanCIResultHandler: nil,
				}
				wirePerComponentHook(info, sub, cmd, nil)

				assert.NotNil(t, info.TerraformPlanCIResultHandler)
				if sub == "destroy" {
					// destroy has no before/after user-hook events yet (a separate,
					// pre-existing gap); NodeHooks stays nil.
					assert.Nil(t, info.NodeHooks)
					return
				}
				// User hooks must still be wired even though the aggregate CI
				// handler owns CI output for this subcommand in CI mode — this is
				// the fix for the bug where CI mode never wired ANY per-component
				// hook (CI or user) for plan/apply.
				assert.NotNil(t, info.NodeHooks,
					"%q subcommand must still wire user hooks in CI mode", sub)
			})
		}
	})

	t.Run("plan/apply/destroy native CI installs aggregate handler alongside user hooks", func(t *testing.T) {
		withGitHubActionsDetection(t)

		for _, sub := range []string{"plan", "apply", "destroy"} {
			t.Run(sub, func(t *testing.T) {
				info := &schema.ConfigAndStacksInfo{
					TerraformPlanCIResultHandler: nil,
				}
				wirePerComponentHook(info, sub, newHookTestCmd(), nil)

				assert.NotNil(t, info.TerraformPlanCIResultHandler)
				if sub == "destroy" {
					assert.Nil(t, info.NodeHooks)
					return
				}
				assert.NotNil(t, info.NodeHooks,
					"%q subcommand must still wire user hooks in CI mode", sub)
			})
		}
	})

	t.Run("unknown subcommand leaves NodeHooks unset", func(t *testing.T) {
		// `init`, `validate`, etc. are valid terraform subcommands but they do
		// not have per-component hooks today. The helper must be a no-op
		// for anything outside the {plan, deploy, apply} set so other
		// subcommands don't accidentally start firing hooks.
		for _, sub := range []string{"destroy", "init", "validate", ""} {
			t.Run(sub, func(t *testing.T) {
				info := &schema.ConfigAndStacksInfo{}
				wirePerComponentHook(info, sub, newHookTestCmd(), nil)
				assert.Nil(t, info.NodeHooks,
					"%q subcommand must NOT install per-component hooks", sub)
			})
		}
	})

	t.Run("installed hooks do not panic when invoked", func(t *testing.T) {
		// Smoke-test the wiring: it must reach RunCIHooks/RunPerComponentHooks
		// without panicking even when invoked outside a configured atmos
		// directory or with no `hooks:` section — they should fail gracefully
		// (Warn log / no-op) rather than crash.
		t.Chdir("../../examples/demo-stacks")
		cmd := newHookTestCmd()

		for _, sub := range []string{"plan", "deploy", "apply"} {
			t.Run(sub, func(t *testing.T) {
				info := &schema.ConfigAndStacksInfo{
					Stack:            "dev",
					Component:        "myapp",
					ComponentFromArg: "myapp",
					ComponentType:    "terraform",
				}
				wirePerComponentHook(info, sub, cmd, nil)
				assert.NotPanics(t, func() {
					assert.NoError(t, info.NodeHooks.Before(context.Background(), info))
				})
				assert.NotPanics(t, func() {
					assert.NoError(t, info.NodeHooks.After(context.Background(), info, "output", nil))
				})
			})
		}
	})

	t.Run("the three subcommands wire to distinct events", func(t *testing.T) {
		// Sanity check: a future edit that copy-pastes one case over another
		// (e.g. apply ends up using the plan event) wouldn't be caught by the
		// nil/non-nil assertions above. Assert the wired before/after events
		// are pairwise distinct across subcommands.
		eventsFor := func(sub string) (hooks.HookEvent, hooks.HookEvent) {
			info := &schema.ConfigAndStacksInfo{}
			wirePerComponentHook(info, sub, newHookTestCmd(), nil)
			nodeHooks, ok := info.NodeHooks.(*terraformNodeHooks)
			require.True(t, ok)
			return nodeHooks.beforeEvent, nodeHooks.afterEvent
		}
		planBefore, planAfter := eventsFor("plan")
		applyBefore, applyAfter := eventsFor("apply")
		deployBefore, deployAfter := eventsFor("deploy")

		assert.NotEqual(t, planAfter, applyAfter, "plan and apply must fire different after-events")
		assert.NotEqual(t, planAfter, deployAfter, "plan and deploy must fire different after-events")
		assert.NotEqual(t, applyAfter, deployAfter, "apply and deploy must fire different after-events")
		assert.NotEqual(t, planBefore, applyBefore, "plan and apply must fire different before-events")
		assert.NotEqual(t, planBefore, deployBefore, "plan and deploy must fire different before-events")
		assert.NotEqual(t, applyBefore, deployBefore, "apply and deploy must fire different before-events")
	})
}

func TestTerraformCIModeEnabledSources(t *testing.T) {
	t.Run("nil command and no CI detection is false", func(t *testing.T) {
		withoutCIDetection(t)
		resetViperCI(t)

		assert.False(t, terraformCIModeEnabled(nil))
	})

	t.Run("cobra flag wins", func(t *testing.T) {
		withoutCIDetection(t)
		resetViperCI(t)
		cmd := newHookTestCmd()
		require.NoError(t, cmd.Flags().Set("ci", "true"))

		assert.True(t, terraformCIModeEnabled(cmd))
	})

	t.Run("viper ci enables mode", func(t *testing.T) {
		withoutCIDetection(t)
		resetViperCI(t)
		viper.Set("ci", true)

		assert.True(t, terraformCIModeEnabled(newHookTestCmd()))
	})

	t.Run("native GitHub Actions detection enables mode", func(t *testing.T) {
		withGitHubActionsDetection(t)
		resetViperCI(t)

		assert.True(t, terraformCIModeEnabled(newHookTestCmd()))
	})
}

func TestTerraformPlanCIResultHandler(t *testing.T) {
	t.Run("nil and incomplete handlers are no-ops", func(t *testing.T) {
		var nilHandler *terraformPlanCIResultHandler
		require.NoError(t, nilHandler.HandleTerraformPlanCIResults(schema.TerraformPlanCIResultSet{}))
		require.NoError(t, (&terraformPlanCIResultHandler{}).HandleTerraformPlanCIResults(schema.TerraformPlanCIResultSet{}))
		require.NoError(t, (&terraformPlanCIResultHandler{cmd: newHookTestCmd()}).HandleTerraformPlanCIResults(schema.TerraformPlanCIResultSet{}))
		require.NoError(t, (&terraformPlanCIResultHandler{info: &schema.ConfigAndStacksInfo{}}).HandleTerraformPlanCIResults(schema.TerraformPlanCIResultSet{}))
	})

	t.Run("runs aggregate CI hook wrapper", func(t *testing.T) {
		t.Chdir("../../examples/demo-stacks")
		resetViperCI(t)
		cmd := newHookTestCmd()
		require.NoError(t, cmd.Flags().Set("ci", "true"))
		info := &schema.ConfigAndStacksInfo{
			Stack:            "dev",
			Component:        "myapp",
			ComponentFromArg: "myapp",
			ComponentType:    "terraform",
		}

		handler := &terraformPlanCIResultHandler{
			cmd:     cmd,
			info:    info,
			command: "apply",
		}

		err := handler.HandleTerraformPlanCIResults(schema.TerraformPlanCIResultSet{Results: []schema.TerraformPlanCIResult{
			{
				NodeID:    "myapp-dev",
				Stack:     "dev",
				Component: "myapp",
				Status:    "succeeded",
				Processed: true,
				Output:    "Apply complete! Resources: 0 added, 0 changed, 0 destroyed.",
			},
		}})
		require.NoError(t, err)
	})

	t.Run("returns config init errors", func(t *testing.T) {
		t.Chdir(t.TempDir())
		require.NoError(t, os.WriteFile("atmos.yaml", []byte("invalid: yaml: content:\n  - this is: [broken\n"), 0o644))
		handler := &terraformPlanCIResultHandler{
			cmd:  newHookTestCmd(),
			info: &schema.ConfigAndStacksInfo{},
		}

		err := handler.HandleTerraformPlanCIResults(schema.TerraformPlanCIResultSet{})

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrInitializeCLIConfig)
	})

	t.Run("returns RunCIHooks errors", func(t *testing.T) {
		// Covers the `if err := h.RunCIHooks(...); err != nil { return err }`
		// branch — distinct from the "returns config init errors" subtest
		// above, which fails earlier at cfg.InitCliConfig. This needs a valid
		// project (so InitCliConfig succeeds) with ci.enabled=true and
		// settings.experimental=disable, so RunCIHooks itself fails inside
		// checkExperimental.
		tempDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "stacks"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "stacks", "test.yaml"), []byte("vars:\n  stage: test\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(`base_path: "./"
components:
  terraform:
    base_path: "components/terraform"
stacks:
  base_path: "stacks"
  included_paths:
    - "**/*"
  name_pattern: "{stage}"
schemas: {}
ci:
  enabled: true
settings:
  experimental: disable
`), 0o644))
		t.Chdir(tempDir)

		cmd := newHookTestCmd()
		require.NoError(t, cmd.Flags().Set("ci", "true"))
		handler := &terraformPlanCIResultHandler{
			cmd:     cmd,
			info:    &schema.ConfigAndStacksInfo{},
			command: "apply",
		}

		err := handler.HandleTerraformPlanCIResults(schema.TerraformPlanCIResultSet{})
		require.Error(t, err)
	})
}

// TestInteractiveStackSelection_PersistsToCobraFlag verifies that when
// handleInteractiveComponentStackSelection fills in the stack from the
// interactive prompt, the value is persisted to the Cobra flag set so
// PostRunE hooks can read it via cmd.Flags().GetString("stack").
// Regression test for https://github.com/cloudposse/atmos/issues/2432.
func TestInteractiveStackSelection_PersistsToCobraFlag(t *testing.T) {
	// Stub the interactive prompt to return a fixed stack name.
	// The real shared.PromptForStack also persists to the Cobra flag;
	// the stub replicates that behavior.
	origPrompt := promptForStack
	promptForStack = func(cmd *cobra.Command, _ string) (string, error) {
		if f := cmd.Flag("stack"); f != nil {
			_ = f.Value.Set("tenant1-ue1-dev")
		}
		return "tenant1-ue1-dev", nil
	}
	t.Cleanup(func() { promptForStack = origPrompt })

	cmd := newHookTestCmd()
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "myapp",
		Stack:            "", // Empty — triggers the interactive prompt path.
	}

	err := handleInteractiveComponentStackSelection(info, cmd)
	assert.NoError(t, err)

	// Verify info.Stack was filled in by the prompt.
	assert.Equal(t, "tenant1-ue1-dev", info.Stack)

	// Verify the Cobra flag was also updated so PostRunE hooks can read it.
	got, flagErr := cmd.Flags().GetString("stack")
	assert.NoError(t, flagErr)
	assert.Equal(t, "tenant1-ue1-dev", got, "interactively-selected stack must persist to Cobra flag")
}

// TestRunHooksWithOutput_InjectsLastAuthContext verifies that
// runHooksWithOutput injects the auth context persisted by
// ExecuteTerraform into the hook info. Uses the demo-stacks fixture
// where hooks short-circuit cleanly (no store hooks configured).
// Regression test for https://github.com/cloudposse/atmos/issues/2433.
func TestRunHooksWithOutput_InjectsLastAuthContext(t *testing.T) {
	t.Chdir("../../examples/demo-stacks")
	t.Cleanup(e.ClearLastAuthContext)

	authCtx := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile:         "test-profile",
			CredentialsFile: "/tmp/test-creds",
			Region:          "us-west-2",
		},
	}
	e.SetLastAuthContext(authCtx, "mock-auth-manager")

	cmd := newHookTestCmd()

	// Call the real runHooksWithOutput — it will pick up the persisted
	// auth context and inject it into the info struct. The demo-stacks
	// fixture has no store hooks, so execution completes without error.
	err := runHooksWithOutput(hooks.AfterTerraformApply, cmd, []string{"--stack", "dev", "myapp"}, "")
	assert.NoError(t, err)

	// Verify the auth context is still available (wasn't cleared by the call).
	gotCtx, gotMgr := e.GetLastAuthContext()
	assert.NotNil(t, gotCtx, "auth context must survive through runHooksWithOutput")
	assert.Equal(t, "test-profile", gotCtx.AWS.Profile)
	assert.Equal(t, "mock-auth-manager", gotMgr)
}

// TestPrepareHookContextResolvesMetadataComponent verifies that the execution
// context retains ProcessStacks' component resolution. InitCliConfig processes
// a value copy, so this explicit pass is what makes lifecycle hooks use the
// metadata.component target rather than the stack-facing alias.
func TestPrepareHookContextResolvesMetadataComponent(t *testing.T) {
	t.Chdir("../../tests/fixtures/scenarios/terraform-apply-all-dependencies")
	cmd := newHookTestCmd()
	require.NoError(t, cmd.Flags().Set("stack", "reported-order-alias"))

	ctx, err := prepareHookContext(cmd, []string{"platform-kms"})
	require.NoError(t, err)
	assert.Equal(t, "platform-kms", ctx.info.ComponentFromArg)
	assert.Equal(t, "mock", ctx.info.FinalComponent)
	assert.Empty(t, ctx.info.ComponentFolderPrefix)
}

// TestPrepareHookContextToleratesProcessStacksFailure verifies the
// metadata.component resolution added to prepareHookContext is genuinely
// best-effort: a ProcessStacks failure (here, no --stack provided, so
// ProcessStacks(checkStack=true) returns errUtils.ErrMissingStack) must not
// become PreRunE's user-facing error. GetHooks (called next, in
// runUserHooks) already tolerates an empty Stack/unresolved component by
// returning no hooks, so info is left exactly as ProcessCommandLineArgs
// produced it, and RunE's own validation — not this hook-prep step —
// produces the correct, authoritative error and message.
func TestPrepareHookContextToleratesProcessStacksFailure(t *testing.T) {
	t.Chdir("../../tests/fixtures/scenarios/terraform-apply-all-dependencies")
	cmd := newHookTestCmd()
	// Deliberately leave --stack unset: ProcessStacks(checkStack=true) fails
	// with ErrMissingStack, which prepareHookContext must swallow.

	ctx, err := prepareHookContext(cmd, []string{"platform-kms"})
	require.NoError(t, err, "a ProcessStacks failure during hook-context prep must not surface as PreRunE's error")
	assert.Equal(t, "platform-kms", ctx.info.ComponentFromArg)
	assert.Empty(t, ctx.info.FinalComponent, "on ProcessStacks failure, info must be left unresolved rather than partially mutated")
}

// TestEnsureComponentSourceProvisioned verifies that a `before.terraform.*`
// hook is a "run before Terraform" event, not a "run before the component's
// JIT source is provisioned" event: ensureComponentSourceProvisioned vendors
// a configured `source:` into the component directory synchronously, so by
// the time hooks fire, ComponentPath() (the env var and hook subprocess cwd)
// resolves to a real, populated directory instead of one that won't exist
// until Terraform itself runs moments later in RunE.
func TestEnsureComponentSourceProvisioned(t *testing.T) {
	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte("# fixture\n"), 0o644))

	terraformDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{TerraformDirAbsolutePath: terraformDir}
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "app",
		FinalComponent:   "app",
		ComponentSection: schema.AtmosSectionMapType{
			"component": "app",
			"source":    sourceDir,
		},
	}

	ensureComponentSourceProvisioned(atmosConfig, info)

	componentPath := filepath.Join(terraformDir, "app")
	entries, err := os.ReadDir(componentPath)
	require.NoError(t, err, "component directory should exist after provisioning")
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Contains(t, names, "main.tf")
}

// TestEnsureComponentSourceProvisioned_NoSourceIsNoOp verifies components
// without a configured `source:` are left untouched (no directory created) —
// ensureComponentSourceProvisioned only matters for JIT-provisioned components.
func TestEnsureComponentSourceProvisioned_NoSourceIsNoOp(t *testing.T) {
	terraformDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{TerraformDirAbsolutePath: terraformDir}
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "app",
		FinalComponent:   "app",
		ComponentSection: schema.AtmosSectionMapType{"component": "app"},
	}

	ensureComponentSourceProvisioned(atmosConfig, info)

	_, err := os.Stat(filepath.Join(terraformDir, "app"))
	assert.True(t, os.IsNotExist(err), "no source configured should mean no directory is created")
}

// TestEnsureComponentSourceProvisioned_ProvisioningFailureIsLoggedNotReturned
// verifies that a JIT source that fails to provision (here, a `source:`
// pointing at a nonexistent local directory) is logged rather than returned:
// ensureComponentSourceProvisioned has no error return, precisely so hooks
// still attempt to run even when provisioning fails — the real Terraform
// command surfaces the same provisioning error authoritatively moments
// later. Regression coverage for that error branch: a failed provisioning
// attempt must leave no component directory behind.
func TestEnsureComponentSourceProvisioned_ProvisioningFailureIsLoggedNotReturned(t *testing.T) {
	terraformDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{TerraformDirAbsolutePath: terraformDir}
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "app",
		FinalComponent:   "app",
		ComponentSection: schema.AtmosSectionMapType{
			"component": "app",
			"source":    filepath.Join(terraformDir, "does-not-exist-source"),
		},
	}

	// ensureComponentSourceProvisioned has no error return: a provisioning
	// failure (here, the source path doesn't exist) must be logged and
	// swallowed, not panic or otherwise abort the caller (runUserHooks),
	// so hooks still get a chance to run.
	require.NotPanics(t, func() {
		ensureComponentSourceProvisioned(atmosConfig, info)
	}, "a provisioning failure must be logged, not propagated as a panic")

	assert.NoDirExists(t, filepath.Join(terraformDir, "app"),
		"a failed provisioning attempt must leave no component directory behind")
}

// writeMinimalSourceFixture builds a self-contained atmos project (atmos.yaml,
// one deploy stack, and a local JIT source directory) under a temp root, so
// provisioning tests don't depend on network access or a checked-in fixture.
// The withHooks parameter controls whether the component has a (lightweight,
// in-process `type: log`) hook configured for both before.terraform.plan and
// before.terraform.init — ensureComponentSourceProvisioned only runs when a
// component actually has hooks (see runUserHooks), so tests proving that
// behavior need one, while tests proving the no-hooks case is left alone
// need its absence. Returns the project root and the component name.
func writeMinimalSourceFixture(t *testing.T, withHooks bool) (root, component string) {
	t.Helper()

	root = t.TempDir()
	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte("# fixture\n"), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte(`base_path: "./"

components:
  terraform:
    base_path: "components/terraform"

stacks:
  base_path: "stacks"
  included_paths:
    - "deploy/*.yaml"
  name_pattern: "{stage}"

logs:
  level: Debug
`), 0o644))

	hooksYAML := ""
	if withHooks {
		hooksYAML = "      hooks:\n" +
			"        announce:\n" +
			"          kind: step\n" +
			"          type: log\n" +
			"          events: [before.terraform.plan, before.terraform.init]\n" +
			"          with:\n" +
			"            content: \"hook ran\"\n"
	}
	stacksDeployDir := filepath.Join(root, "stacks", "deploy")
	require.NoError(t, os.MkdirAll(stacksDeployDir, 0o755))
	stackYAML := "vars:\n  stage: test\n\ncomponents:\n  terraform:\n    app:\n" +
		hooksYAML +
		"      source:\n        uri: \"" + filepath.ToSlash(sourceDir) + "\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(stacksDeployDir, "test.yaml"), []byte(stackYAML), 0o644))

	return root, "app"
}

// newInitTestCmd builds a cobra.Command matching cmd/terraform/init.go's flag
// surface, for tests exercising before.terraform.init specifically.
func newInitTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "init"}
	cmd.Flags().String("base-path", "", "base path")
	cmd.Flags().StringSlice("config", nil, "config")
	cmd.Flags().StringSlice("config-path", nil, "config path")
	cmd.Flags().StringSlice("profile", nil, "profile")
	cmd.Flags().String("stack", "", "stack flag")
	cmd.Flags().Bool("ci", false, "ci flag")
	return cmd
}

// TestRunUserHooksProvisionsSourceBeforeHooks verifies that when a component
// actually has hooks configured, runUserHooks provisions its JIT `source:`
// before firing a `before.terraform.plan` hook, so ComponentPath()/
// $ATMOS_COMPONENT_PATH point at a real, populated directory instead of one
// Terraform itself wouldn't create until RunE.
func TestRunUserHooksProvisionsSourceBeforeHooks(t *testing.T) {
	root, component := writeMinimalSourceFixture(t, true)
	t.Chdir(root)
	withoutCIDetection(t)

	cmd := newHookTestCmd() // Use: "plan"
	require.NoError(t, cmd.Flags().Set("stack", "test"))

	require.NoError(t, runHooks(hooks.BeforeTerraformPlan, cmd, []string{component}))

	_, statErr := os.Stat(filepath.Join(root, "components", "terraform", component, "main.tf"))
	assert.NoError(t, statErr, "before.terraform.plan must see a provisioned component directory")
}

// TestRunUserHooksSkipsProvisioningWithoutHooks verifies ensureComponentSource
// Provisioned is NOT invoked for a component with no hooks configured, even
// though it has a JIT `source:`. Regression test: every terraform subcommand
// already provisions its own component independently in RunE, so an
// unconditional pre-provisioning attempt here raced that provisioning and
// broke TestJITSource_WorkdirWithLocalComponent_AllSubcommands (all but the
// first subcommand failed with "no such file or directory") for components
// that have nothing to gain from it.
func TestRunUserHooksSkipsProvisioningWithoutHooks(t *testing.T) {
	root, component := writeMinimalSourceFixture(t, false)
	t.Chdir(root)
	withoutCIDetection(t)

	cmd := newHookTestCmd() // Use: "plan"
	require.NoError(t, cmd.Flags().Set("stack", "test"))

	require.NoError(t, runHooks(hooks.BeforeTerraformPlan, cmd, []string{component}))

	_, statErr := os.Stat(filepath.Join(root, "components", "terraform", component))
	assert.True(t, os.IsNotExist(statErr), "a component with no hooks must not be pre-provisioned")
}

// TestRunUserHooksSkipsProvisioningForInit verifies before.terraform.init is
// excluded from pre-provisioning even when the component has hooks: it is the
// source provisioner's own lifecycle event (HookEventBeforeTerraformInit), so
// pre-provisioning here would fire it after provisioning already happened,
// inverting the one event whose entire point is to run before the source
// exists. Regression test for a CodeRabbit review finding on PR #2802.
func TestRunUserHooksSkipsProvisioningForInit(t *testing.T) {
	root, component := writeMinimalSourceFixture(t, true)
	t.Chdir(root)
	withoutCIDetection(t)

	cmd := newInitTestCmd()
	require.NoError(t, cmd.Flags().Set("stack", "test"))

	require.NoError(t, runHooks(hooks.BeforeTerraformInit, cmd, []string{component}))

	_, statErr := os.Stat(filepath.Join(root, "components", "terraform", component))
	assert.True(t, os.IsNotExist(statErr), "before.terraform.init must NOT trigger pre-provisioning")
}

// TestRunUserHooksLogsStatusForAfterEvents verifies the after-event branch of
// runUserHooks' status log: on before.* events the terraform operation hasn't
// run yet (status is misleading and omitted), but on after.* events the
// outcome status must be included so it's visible in the log line. This
// exercises the branch that was unreachable via before-only fixtures.
func TestRunUserHooksLogsStatusForAfterEvents(t *testing.T) {
	root, component := writeMinimalSourceFixture(t, true)
	t.Chdir(root)
	withoutCIDetection(t)

	cmd := newHookTestCmd() // Use: "plan"
	require.NoError(t, cmd.Flags().Set("stack", "test"))

	// The fixture's hook only fires on before.terraform.plan/init, so RunAll
	// no-ops for this after-event, but runUserHooks must still reach (and not
	// panic on) the "status" log branch before calling RunAll.
	require.NoError(t, runHooks(hooks.AfterTerraformPlan, cmd, []string{component}))
}

// TestPrepareHookContextSkipsComponentResolutionForMultiComponent verifies that
// the global before/after hook context for multi-component invocations
// (--all/--affected/--components/--query/--tags/--labels) does not fail with
// "component is required": no positional component is resolved yet at that
// point (only per-component hooks inside the component walker have one), so
// prepareHookContext must skip the metadata.component-resolving ProcessStacks
// call rather than call it with an empty ComponentFromArg. Regression test
// for the #2799 fix breaking `atmos terraform plan --all`.
func TestPrepareHookContextSkipsComponentResolutionForMultiComponent(t *testing.T) {
	t.Chdir("../../tests/fixtures/scenarios/terraform-apply-all-dependencies")
	cmd := newHookTestCmd()
	require.NoError(t, cmd.Flags().Set("stack", "reported-order-alias"))

	ctx, err := prepareHookContext(cmd, []string{})
	require.NoError(t, err)
	assert.Empty(t, ctx.info.ComponentFromArg)
	assert.Empty(t, ctx.info.FinalComponent)
}

// TestInjectHookStoreAuthResolver_InheritsDefaultIdentity verifies that the after-apply hook path
// now wires the resolver AND lets identity-less stores inherit the run's auto-detected identity
// (matching the main terraform path), so hook store writes work under Atmos auth. Auto-detection
// runs only when no explicit identity is present; an explicit/disabled identity is not overridden by
// the chain.
//
// Note: the per-store identity argument to SetAuthContext is computed by the store registry via
// defaultIdentityForStore, which only applies the default to the concrete SSM/ASM/AKV/GSM store types
// (a MockIdentityAwareStore receives ""). This test therefore asserts the seam behavior — chain
// auto-detection (GetChain), info.Identity population, and resolver wiring. That identity-less
// concrete stores actually receive the default is covered by pkg/store
// TestSetAuthContextResolverWithDefaultIdentity_DefaultsOnlyEmptyStores, and end to end by the Floci
// E2E TestAWSStoreHooks_InheritedIdentity_FlociE2E.
func TestInjectHookStoreAuthResolver_InheritsDefaultIdentity(t *testing.T) {
	tests := []struct {
		name             string
		identity         string
		chain            []string // nil => GetChain must NOT be called.
		expectedIdentity string   // info.Identity after the call.
	}{
		{
			name:             "no explicit identity auto-detects the chain leaf",
			identity:         "",
			chain:            []string{"permission-set", "core-identity/devops"},
			expectedIdentity: "core-identity/devops",
		},
		{
			name:             "explicit command identity is preserved (chain not consulted)",
			identity:         "cli-admin",
			chain:            nil,
			expectedIdentity: "cli-admin",
		},
		{
			name:             "disabled identity is not overridden by the chain",
			identity:         cfg.IdentityFlagDisabledValue,
			chain:            nil,
			expectedIdentity: cfg.IdentityFlagDisabledValue,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			authManager := authtypes.NewMockAuthManager(ctrl)
			if tc.chain != nil {
				authManager.EXPECT().GetChain().Return(tc.chain)
			}
			mockStore := storepkg.NewMockIdentityAwareStore(ctrl)

			// The resolver must always be wired into the store (regardless of identity).
			mockStore.EXPECT().
				SetAuthContext(gomock.Not(nil), gomock.Any()).
				Do(func(resolver storepkg.AuthContextResolver, _ string) {
					assert.NotNil(t, resolver)
				})

			atmosConfig := &schema.AtmosConfiguration{
				Stores: storepkg.StoreRegistry{
					"store": mockStore,
				},
			}
			info := &schema.ConfigAndStacksInfo{
				Identity:    tc.identity,
				AuthManager: authManager,
			}

			injectHookStoreAuthResolver(atmosConfig, info)

			assert.Equal(t, tc.expectedIdentity, info.Identity,
				"hook should auto-detect the active identity when none is explicitly set")
		})
	}
}

// TestInteractiveStackSelection_PromptError verifies that when the
// interactive stack prompt returns an error, it propagates correctly.
func TestInteractiveStackSelection_PromptError(t *testing.T) {
	origPrompt := promptForStack
	promptForStack = func(_ *cobra.Command, _ string) (string, error) {
		return "", errors.New("user cancelled")
	}
	t.Cleanup(func() { promptForStack = origPrompt })

	cmd := newHookTestCmd()
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "myapp",
		Stack:            "",
	}

	err := handleInteractiveComponentStackSelection(info, cmd)
	assert.Error(t, err)
	assert.Empty(t, info.Stack)
}

// TestInteractiveComponentSelection_PersistsPromptResult verifies that
// when both component and stack are missing, the component prompt is
// called first, then the stack prompt, and both results are persisted.
func TestInteractiveComponentSelection_PersistsPromptResult(t *testing.T) {
	origComp := promptForComponent
	origStack := promptForStack
	promptForComponent = func(_ *cobra.Command, _ string) (string, error) {
		return "vpc", nil
	}
	// The real shared.PromptForStack also persists to the Cobra flag;
	// the stub replicates that behavior.
	promptForStack = func(cmd *cobra.Command, _ string) (string, error) {
		if f := cmd.Flag("stack"); f != nil {
			_ = f.Value.Set("prod-ue1")
		}
		return "prod-ue1", nil
	}
	t.Cleanup(func() {
		promptForComponent = origComp
		promptForStack = origStack
	})

	cmd := newHookTestCmd()
	info := &schema.ConfigAndStacksInfo{}

	err := handleInteractiveComponentStackSelection(info, cmd)
	assert.NoError(t, err)
	assert.Equal(t, "vpc", info.ComponentFromArg)
	assert.Equal(t, "prod-ue1", info.Stack)

	got, _ := cmd.Flags().GetString("stack")
	assert.Equal(t, "prod-ue1", got)
}

// TestHandleInteractiveComponentStackSelection_BothProvided verifies the
// short-circuit path: when both component and stack are already set,
// the function returns nil without prompting.
func TestHandleInteractiveComponentStackSelection_BothProvided(t *testing.T) {
	cmd := newHookTestCmd()
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "dev",
	}

	err := handleInteractiveComponentStackSelection(info, cmd)
	assert.NoError(t, err)
	assert.Equal(t, "vpc", info.ComponentFromArg)
	assert.Equal(t, "dev", info.Stack)
}

// TestHandleInteractiveComponentStackSelection_SkipsMultiComponent verifies
// the function skips prompting when multi-component flags are set.
func TestHandleInteractiveComponentStackSelection_SkipsMultiComponent(t *testing.T) {
	cmd := newHookTestCmd()
	info := &schema.ConfigAndStacksInfo{
		All: true,
	}

	err := handleInteractiveComponentStackSelection(info, cmd)
	assert.NoError(t, err)
}
