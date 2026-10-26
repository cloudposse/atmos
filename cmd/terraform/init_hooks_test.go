package terraform

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/hooks"
)

// TestInitCmd_HookWiring checks CLI preparation and the after-hook wrappers.
// TestTerraformLifecycleWorkdir exercises before-hooks at the executor boundary.
func TestInitCmd_HookWiring(t *testing.T) {
	isolateInitHookSelection(t)
	t.Chdir("../../examples/demo-stacks")
	args := []string{"--stack", "dev", "myapp"}

	// Init resolves CLI selection before execution and reports completion afterward.
	require.NotNil(t, initCmd.PreRunE, "init must prepare CLI selection")
	require.NotNil(t, initCmd.PostRunE, "init must wire PostRunE for after-terraform-init")

	t.Run("PreRunE prepares CLI selection", func(t *testing.T) {
		assert.NoError(t, initCmd.PreRunE(newHookTestCmd(), args))
	})

	t.Run("PostRunE runs after-terraform-init hooks in single-component mode", func(t *testing.T) {
		wasMultiComponentExecution = false
		assert.NoError(t, initCmd.PostRunE(newHookTestCmd(), args))
	})

	t.Run("PostRunE suppresses the global hook in multi-component mode", func(t *testing.T) {
		wasMultiComponentExecution = true
		t.Cleanup(func() { wasMultiComponentExecution = false })
		// Returns nil early without invoking runHooksWithOutput.
		assert.NoError(t, initCmd.PostRunE(newHookTestCmd(), args))
	})
}

// TestInitCmd_RunE_FiresAfterInitErrorHookOnFailure covers the init RunE body
// and its deferred error hook: when RunE fails (here, no atmos stacks directory
// in an empty working dir, so ValidateAtmosConfig errors), the deferred
// after-terraform-init hook must fire with the run error so CI check runs are
// updated to failure. The wrapper is stubbed to record the invocation without
// performing real hook work.
func TestInitCmd_RunE_FiresAfterInitErrorHookOnFailure(t *testing.T) {
	isolateInitHookSelection(t)
	// Empty dir with no atmos.yaml/stacks — RunE fails fast in
	// terraformRunWithOptions before any terraform binary is needed.
	t.Chdir(t.TempDir())

	var (
		fired     bool
		gotEvent  hooks.HookEvent
		gotCmdErr error
	)
	orig := runHooksOnErrorWithOutput
	runHooksOnErrorWithOutput = func(event hooks.HookEvent, _ *cobra.Command, _ []string, cmdErr error, _ string) {
		fired = true
		gotEvent = event
		gotCmdErr = cmdErr
	}
	t.Cleanup(func() { runHooksOnErrorWithOutput = orig })

	wasMultiComponentExecution = false
	err := initCmd.RunE(newHookTestCmd(), []string{"--stack", "dev", "myapp"})

	require.Error(t, err, "RunE must fail when there is no stacks directory")
	assert.True(t, fired, "deferred after-terraform-init error hook must fire on RunE failure")
	assert.Equal(t, hooks.AfterTerraformInit, gotEvent)
	require.Error(t, gotCmdErr, "the run error must be forwarded to the error hook")
}

// isolateInitHookSelection prevents CLI selection from leaking across hook tests.
func isolateInitHookSelection(t *testing.T) {
	t.Helper()

	previousComponent := preResolvedComponent
	previousMultiComponent := wasMultiComponentExecution
	preResolvedComponent = ""
	wasMultiComponentExecution = false
	t.Cleanup(func() {
		preResolvedComponent = previousComponent
		wasMultiComponentExecution = previousMultiComponent
	})
}

// Compile-time guard: the init command fires these specific events. If either
// constant is renamed, this fails to build so the wiring drift surfaces.
var (
	_ = hooks.BeforeTerraformInit
	_ = hooks.AfterTerraformInit
)
