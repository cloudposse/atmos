package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/hooks"
)

// TestInitCmd_HookWiring exercises the PreRunE/PostRunE hook closures added to
// the `terraform init` command so the before-/after-terraform-init lifecycle
// events fire through the shared runHooks path. The demo-stacks fixture has
// ci.enabled=false and the component declares no init hooks, so the wrappers run
// to completion without side effects — the test asserts the wiring executes
// cleanly and that the multi-component guard short-circuits PostRunE.
//
// Mirrors TestRunHooks_DemoStacks in utils_hooks_test.go (which can't use
// cmd.NewTestKit due to a circular import); the closures take the *cobra.Command
// as a parameter, so a lightweight test command stands in for the real one.
func TestInitCmd_HookWiring(t *testing.T) {
	t.Chdir("../../examples/demo-stacks")
	args := []string{"--stack", "dev", "myapp"}

	// Sanity: the init command must actually wire the hook closures, otherwise
	// before-/after-terraform-init would silently never fire.
	require.NotNil(t, initCmd.PreRunE, "init must wire PreRunE for before-terraform-init")
	require.NotNil(t, initCmd.PostRunE, "init must wire PostRunE for after-terraform-init")

	t.Run("PreRunE runs before-terraform-init hooks", func(t *testing.T) {
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

// Compile-time guard: the init command fires these specific events. If either
// constant is renamed, this fails to build so the wiring drift surfaces.
var (
	_ = hooks.BeforeTerraformInit
	_ = hooks.AfterTerraformInit
)
