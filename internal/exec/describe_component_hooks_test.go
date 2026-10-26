package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDescribeComponent_TopLevelTerraformHooksInherited is the end-to-end guard
// for the DRY hook pattern reported in the field: hooks defined once at the
// top-level `terraform.hooks` scope in an imported `_defaults` stack must be
// inherited by a component that declares no hooks of its own.
//
// It exercises ExecuteDescribeComponent — the exact call hooks.GetHooks uses at
// runtime — so it validates the real path (import resolution → deep merge →
// hooks section on the component), not just the in-memory merge unit.
func TestDescribeComponent_TopLevelTerraformHooksInherited(t *testing.T) {
	t.Chdir("../../tests/fixtures/scenarios/hooks-terraform-scope")

	componentSection, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component: "vpc",
		Stack:     "test",
		// Match GetHooks: hook discovery reads static metadata only, no template
		// or YAML-function processing.
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		Skip:                 []string{},
		AuthManager:          nil,
	})
	require.NoError(t, err)
	require.NotNil(t, componentSection)

	hooks, ok := componentSection["hooks"].(map[string]any)
	require.True(t, ok, "vpc must have an inherited hooks section, got: %v", componentSection["hooks"])

	policy, ok := hooks["policy"].(map[string]any)
	require.True(t, ok, "vpc must inherit the 'policy' hook from top-level terraform.hooks, got: %v", hooks)
	assert.Equal(t, "checkov", policy["kind"])
	assert.Equal(t, []any{"before-terraform-plan"}, policy["events"])

	cost, ok := hooks["cost"].(map[string]any)
	require.True(t, ok, "vpc must inherit the 'cost' hook from top-level terraform.hooks, got: %v", hooks)
	assert.Equal(t, "infracost", cost["kind"])
	assert.Equal(t, []any{"after-terraform-plan"}, cost["events"])
}
