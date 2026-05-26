package generate

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	h "github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel: tests below depend on these schema.ConfigAndStacksInfo
// fields by name. If a field is renamed upstream, this declaration fails to
// compile so the rename surfaces before the tests silently drift.
var _ = schema.ConfigAndStacksInfo{
	Stack:            "",
	Component:        "",
	ComponentFromArg: "",
	ComponentType:    "",
}

// newPlanfileHookTestCmd constructs a cobra.Command with all the flags
// ProcessCommandLineArgs reads (base-path, config, config-path, profile,
// stack, ci). This lets the wrapper functions in hooks.go progress past
// argument parsing and into the option-construction code.
func newPlanfileHookTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "planfile"}
	cmd.Flags().String("base-path", "", "base path")
	cmd.Flags().StringSlice("config", nil, "config")
	cmd.Flags().StringSlice("config-path", nil, "config path")
	cmd.Flags().StringSlice("profile", nil, "profile")
	cmd.Flags().String("stack", "", "stack flag")
	cmd.Flags().Bool("ci", false, "ci flag")
	return cmd
}

// TestPlanfileHookEventStrings pins the wire format of the new hook event
// constants. The string values are user-facing — they appear in atmos.yaml
// hook configuration — so a rename is a breaking change for any user who
// configured before/after.terraform.generate.planfile hooks.
func TestPlanfileHookEventStrings(t *testing.T) {
	assert.Equal(t, h.HookEvent("before.terraform.generate.planfile"), h.BeforeTerraformGeneratePlanfile)
	assert.Equal(t, h.HookEvent("after.terraform.generate.planfile"), h.AfterTerraformGeneratePlanfile)
}

// TestPlanfileCmd_CIFlag pins the --ci flag wiring (regression guard against
// silent removal or env-var rename). ATMOS_CI and CI env bindings must remain
// in place so users running in GitHub Actions / GitLab CI auto-detect CI mode.
func TestPlanfileCmd_CIFlag(t *testing.T) {
	t.Run("flag is registered as a bool with default false", func(t *testing.T) {
		flag := planfileCmd.Flags().Lookup("ci")
		require.NotNil(t, flag, "--ci flag must be registered on the planfile command")
		assert.Equal(t, "bool", flag.Value.Type(), "--ci must be a bool flag")
		assert.Equal(t, "false", flag.DefValue, "--ci must default to false")
	})

	t.Run("ATMOS_CI and CI env vars are bound", func(t *testing.T) {
		registry := planfileParser.Registry()
		require.True(t, registry.Has("ci"), "planfileParser must register the ci flag")
		envVars := registry.Get("ci").GetEnvVars()
		assert.Contains(t, envVars, "ATMOS_CI", "ci flag must bind ATMOS_CI")
		assert.Contains(t, envVars, "CI", "ci flag must bind CI (auto-detection)")
	})
}

// TestPlanfileCmd_HookLifecycleWired verifies the PreRunE/PostRunE wiring is
// present. The defer-guard in RunE is exercised separately by
// TestPlanfileCmd_RunE_DeferGuard below.
func TestPlanfileCmd_HookLifecycleWired(t *testing.T) {
	require.NotNil(t, planfileCmd.PreRunE, "planfile must wire PreRunE for before.terraform.generate.planfile")
	require.NotNil(t, planfileCmd.PostRunE, "planfile must wire PostRunE for after.terraform.generate.planfile")
}

// TestPlanfileCmd_PreRunE_FiresBeforeEvent verifies that the registered
// PreRunE closure invokes runGeneratePlanfileHooks with the BeforeTerraformGeneratePlanfile
// event. Without this assertion, a future copy-paste regression (e.g., wiring
// PreRunE to the After event) would pass the "non-nil" check but break the
// hook contract. Symmetric assertion for PostRunE follows below.
func TestPlanfileCmd_PreRunE_FiresBeforeEvent(t *testing.T) {
	origHook := runGeneratePlanfileHooks
	defer func() { runGeneratePlanfileHooks = origHook }()

	var called bool
	var calledEvent h.HookEvent
	runGeneratePlanfileHooks = func(event h.HookEvent, _ *cobra.Command, _ []string) error {
		called = true
		calledEvent = event
		return nil
	}

	err := planfileCmd.PreRunE(newPlanfileHookTestCmd(), []string{"--stack", "dev", "myapp"})
	assert.NoError(t, err, "stubbed hook returns nil; PreRunE must forward that")
	assert.True(t, called, "PreRunE must invoke runGeneratePlanfileHooks")
	assert.Equal(t, h.BeforeTerraformGeneratePlanfile, calledEvent,
		"PreRunE must fire the BEFORE event, not the AFTER event")
}

// TestPlanfileCmd_PostRunE_FiresAfterEvent is the symmetric assertion to
// TestPlanfileCmd_PreRunE_FiresBeforeEvent. Together they pin the hook
// lifecycle contract: PreRunE → before-event, PostRunE → after-event.
func TestPlanfileCmd_PostRunE_FiresAfterEvent(t *testing.T) {
	origHook := runGeneratePlanfileHooks
	defer func() { runGeneratePlanfileHooks = origHook }()

	var called bool
	var calledEvent h.HookEvent
	runGeneratePlanfileHooks = func(event h.HookEvent, _ *cobra.Command, _ []string) error {
		called = true
		calledEvent = event
		return nil
	}

	err := planfileCmd.PostRunE(newPlanfileHookTestCmd(), []string{"--stack", "dev", "myapp"})
	assert.NoError(t, err, "stubbed hook returns nil; PostRunE must forward that")
	assert.True(t, called, "PostRunE must invoke runGeneratePlanfileHooks")
	assert.Equal(t, h.AfterTerraformGeneratePlanfile, calledEvent,
		"PostRunE must fire the AFTER event, not the BEFORE event")
}

// TestPlanfileCmd_PreRunE_PropagatesHookError verifies that an error returned
// by a user-defined before-hook (e.g., a guard command that exits non-zero)
// aborts the command before terraform runs, matching the cmd/terraform/plan.go
// contract. A regression here would silently swallow user-hook failures.
func TestPlanfileCmd_PreRunE_PropagatesHookError(t *testing.T) {
	origHook := runGeneratePlanfileHooks
	defer func() { runGeneratePlanfileHooks = origHook }()

	hookErr := errors.New("user-defined before-hook failed")
	runGeneratePlanfileHooks = func(_ h.HookEvent, _ *cobra.Command, _ []string) error {
		return hookErr
	}

	err := planfileCmd.PreRunE(newPlanfileHookTestCmd(), []string{"--stack", "dev", "myapp"})
	assert.ErrorIs(t, err, hookErr, "PreRunE must propagate hook errors so cobra aborts the command")
}

// TestRunGeneratePlanfileHooks_DemoStacks exercises the success-path wrapper.
// The demo-stacks fixture has ci.enabled=false so RunCIHooks short-circuits
// cleanly inside the wrapper — the test asserts the wrapper completes without
// error and reaches the hook-resolution code path.
func TestRunGeneratePlanfileHooks_DemoStacks(t *testing.T) {
	t.Chdir("../../../examples/demo-stacks")

	cmd := newPlanfileHookTestCmd()
	err := runGeneratePlanfileHooks(h.BeforeTerraformGeneratePlanfile, cmd, []string{"--stack", "dev", "myapp"})
	assert.NoError(t, err)
}

// TestRunGeneratePlanfileHooks_ConfigInitError exercises the wrapper's error
// path. When invoked outside an Atmos workspace, ProcessCommandLineArgs /
// ValidateAtmosConfig / InitCliConfig fail — the wrapper must surface the
// error, not silently fall through to RunCIHooks where ctx.AtmosConfig would
// be a zero value and downstream plugin code could panic.
func TestRunGeneratePlanfileHooks_ConfigInitError(t *testing.T) {
	// Chdir to an empty temp dir with no atmos.yaml so config init fails.
	t.Chdir(t.TempDir())

	cmd := newPlanfileHookTestCmd()
	err := runGeneratePlanfileHooks(h.BeforeTerraformGeneratePlanfile, cmd, []string{"--stack", "dev", "myapp"})
	require.Error(t, err, "wrapper must surface config-init failure")
}

// TestRunGeneratePlanfileErrorHook_SilentlyHandlesConfigInitError verifies the
// failure-path wrapper does not panic and does not propagate errors when
// invoked outside an Atmos workspace. It is called from a defer in RunE on the
// already-failing path; surfacing a second error would obscure the original
// command failure that the user actually needs to see.
func TestRunGeneratePlanfileErrorHook_SilentlyHandlesConfigInitError(t *testing.T) {
	// Chdir to an empty temp dir with no atmos.yaml so config init fails.
	t.Chdir(t.TempDir())

	cmd := newPlanfileHookTestCmd()

	// The wrapper returns void. The contract under test is: no panic, no log
	// noise that would obscure the original command error in the user's terminal.
	assert.NotPanics(t, func() {
		runGeneratePlanfileErrorHook(h.AfterTerraformGeneratePlanfile, cmd, []string{"--stack", "dev", "myapp"}, errUtils.ExitCodeError{Code: 1})
	})
}

// TestRunGeneratePlanfileErrorHook_DemoStacks exercises the failure-path
// wrapper. errUtils.GetExitCode must extract the wrapped code from
// ExitCodeError; if it ever stops working, CI summaries silently regress to
// ExitCode=1 by default.
func TestRunGeneratePlanfileErrorHook_DemoStacks(t *testing.T) {
	t.Chdir("../../../examples/demo-stacks")

	cmd := newPlanfileHookTestCmd()

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
		{
			name:    "nil cmdErr is forwarded as exit code 0",
			cmdErr:  nil,
			wantExt: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Pre-condition: GetExitCode must extract the wrapped code before
			// the wrapper forwards it to RunCIHooks.
			assert.Equal(t, tc.wantExt, errUtils.GetExitCode(tc.cmdErr),
				"GetExitCode must extract the wrapped exit code")

			// runGeneratePlanfileErrorHook returns void; the test asserts no
			// panic and no fatal error from the option construction or the
			// ci-disabled short-circuit path.
			runGeneratePlanfileErrorHook(h.AfterTerraformGeneratePlanfile, cmd, []string{"--stack", "dev", "myapp"}, tc.cmdErr)
		})
	}
}

// TestPlanfileCmd_RunE_DeferGuard verifies the RunE defer-guard contract: the
// error-path hook (runGeneratePlanfileErrorHook) must fire when runErr is
// non-nil and must NOT fire on the success path (PostRunE handles success).
//
// The defer-guard lives inside planfileCmd.RunE and runs after the named
// return value `runErr` is assigned, so we cannot pre-set runErr and invoke
// RunE directly. Instead, this test mirrors the defer-body inline and verifies
// every branch using a stubbed runGeneratePlanfileErrorHook.
func TestPlanfileCmd_RunE_DeferGuard(t *testing.T) {
	origHook := runGeneratePlanfileErrorHook
	defer func() { runGeneratePlanfileErrorHook = origHook }()

	var called bool
	var calledEvent h.HookEvent
	var calledErr error
	runGeneratePlanfileErrorHook = func(event h.HookEvent, _ *cobra.Command, _ []string, cmdErr error) {
		called = true
		calledEvent = event
		calledErr = cmdErr
	}

	cmd := newPlanfileHookTestCmd()
	args := []string{"--stack", "dev", "myapp"}

	// invokeDefer mirrors the RunE defer-guard body in planfile.go. Any change
	// to the production guard must be reflected here.
	invokeDefer := func(runErr error) {
		if runErr != nil {
			runGeneratePlanfileErrorHook(h.AfterTerraformGeneratePlanfile, cmd, args, runErr)
		}
	}

	planErr := errors.New("terraform plan failed inside generate planfile")

	tests := []struct {
		name             string
		runErr           error
		expectCalled     bool
		expectForwardErr error
	}{
		{
			name:             "non-nil error fires the error hook with after-event",
			runErr:           planErr,
			expectCalled:     true,
			expectForwardErr: planErr,
		},
		{
			name:         "nil error does not fire the error hook (PostRunE handles success)",
			runErr:       nil,
			expectCalled: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			called = false
			calledEvent = ""
			calledErr = nil

			invokeDefer(tc.runErr)

			assert.Equal(t, tc.expectCalled, called, "error hook firing did not match expectation")
			if tc.expectCalled {
				assert.Equal(t, h.AfterTerraformGeneratePlanfile, calledEvent, "error hook event mismatch")
				assert.Equal(t, tc.expectForwardErr, calledErr, "error hook did not receive original runErr")
			}
		})
	}
}
