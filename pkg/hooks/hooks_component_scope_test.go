package hooks

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e "github.com/cloudposse/atmos/internal/exec"
)

// TestHooksAreComponentScoped verifies that lifecycle hooks defined in component
// catalog files are properly scoped to their respective components and not merged
// globally across all components in a stack.
//
// This test ensures that when multiple components (vpc, rds, lambda) each define
// their own hooks in separate catalog files, each component only receives its own
// hooks and not hooks from other components.
func TestHooksAreComponentScoped(t *testing.T) {
	testDir := "../../tests/test-cases/hooks-component-scoped"

	absTestDir, err := filepath.Abs(testDir)
	require.NoError(t, err)

	// Change to test directory so atmos finds the config
	t.Chdir(absTestDir)

	// Test VPC component
	vpcComponent, err := e.ExecuteDescribeComponent("vpc", "acme-dev-test", true, true, []string{})
	require.NoError(t, err)

	vpcHooks, ok := vpcComponent["hooks"].(map[string]any)
	require.True(t, ok, "vpc should have hooks section")

	// Test RDS component
	rdsComponent, err := e.ExecuteDescribeComponent("rds", "acme-dev-test", true, true, []string{})
	require.NoError(t, err)

	rdsHooks, ok := rdsComponent["hooks"].(map[string]any)
	require.True(t, ok, "rds should have hooks section")

	// Test Lambda component
	lambdaComponent, err := e.ExecuteDescribeComponent("lambda", "acme-dev-test", true, true, []string{})
	require.NoError(t, err)

	lambdaHooks, ok := lambdaComponent["hooks"].(map[string]any)
	require.True(t, ok, "lambda should have hooks section")

	// Log hook counts for debugging
	t.Logf("vpc hooks count: %d, hooks: %v", len(vpcHooks), getHookNames(vpcHooks))
	t.Logf("rds hooks count: %d, hooks: %v", len(rdsHooks), getHookNames(rdsHooks))
	t.Logf("lambda hooks count: %d, hooks: %v", len(lambdaHooks), getHookNames(lambdaHooks))

	// Verify hooks are properly scoped to each component

	// VPC should only have its own hook
	assert.Equal(t, 1, len(vpcHooks), "vpc should have exactly 1 hook (vpc-store-outputs)")
	assert.Contains(t, vpcHooks, "vpc-store-outputs", "vpc should have vpc-store-outputs")
	assert.NotContains(t, vpcHooks, "rds-store-outputs", "vpc should NOT have rds-store-outputs")
	assert.NotContains(t, vpcHooks, "lambda-store-outputs", "vpc should NOT have lambda-store-outputs")

	// RDS should only have its own hook
	assert.Equal(t, 1, len(rdsHooks), "rds should have exactly 1 hook (rds-store-outputs)")
	assert.Contains(t, rdsHooks, "rds-store-outputs", "rds should have rds-store-outputs")
	assert.NotContains(t, rdsHooks, "vpc-store-outputs", "rds should NOT have vpc-store-outputs")
	assert.NotContains(t, rdsHooks, "lambda-store-outputs", "rds should NOT have lambda-store-outputs")

	// Lambda should only have its own hook
	assert.Equal(t, 1, len(lambdaHooks), "lambda should have exactly 1 hook (lambda-store-outputs)")
	assert.Contains(t, lambdaHooks, "lambda-store-outputs", "lambda should have lambda-store-outputs")
	assert.NotContains(t, lambdaHooks, "vpc-store-outputs", "lambda should NOT have vpc-store-outputs")
	assert.NotContains(t, lambdaHooks, "rds-store-outputs", "lambda should NOT have rds-store-outputs")
}

// TestHooksWithDRYPattern verifies that when using a DRY pattern where the hook
// structure is defined globally and components only override the outputs, hooks
// are still properly scoped to their components.
//
// This test uses the pattern:
// - Global _defaults.yaml defines hook structure (events, command, name)
// - Each component only defines the outputs specific to that component.
func TestHooksWithDRYPattern(t *testing.T) {
	testDir := "../../tests/test-cases/hooks-component-scoped"

	absTestDir, err := filepath.Abs(testDir)
	require.NoError(t, err)

	// Change to test directory so atmos finds the config
	t.Chdir(absTestDir)

	// Test VPC component using DRY pattern
	vpcComponent, err := e.ExecuteDescribeComponent("vpc-dry", "acme-dev-dry", true, true, []string{})
	require.NoError(t, err)

	vpcHooks, ok := vpcComponent["hooks"].(map[string]any)
	require.True(t, ok, "vpc-dry should have hooks section")

	// Test RDS component using DRY pattern
	rdsComponent, err := e.ExecuteDescribeComponent("rds-dry", "acme-dev-dry", true, true, []string{})
	require.NoError(t, err)

	rdsHooks, ok := rdsComponent["hooks"].(map[string]any)
	require.True(t, ok, "rds-dry should have hooks section")

	// Test Lambda component using DRY pattern
	lambdaComponent, err := e.ExecuteDescribeComponent("lambda-dry", "acme-dev-dry", true, true, []string{})
	require.NoError(t, err)

	lambdaHooks, ok := lambdaComponent["hooks"].(map[string]any)
	require.True(t, ok, "lambda-dry should have hooks section")

	// Log hook counts for debugging
	t.Logf("vpc-dry hooks count: %d, hooks: %v", len(vpcHooks), getHookNames(vpcHooks))
	t.Logf("rds-dry hooks count: %d, hooks: %v", len(rdsHooks), getHookNames(rdsHooks))
	t.Logf("lambda-dry hooks count: %d, hooks: %v", len(lambdaHooks), getHookNames(lambdaHooks))

	// Verify that all components have the same hook name (store-outputs)
	// but with different outputs specific to each component
	assert.Equal(t, 1, len(vpcHooks), "vpc-dry should have exactly 1 hook (store-outputs)")
	assert.Equal(t, 1, len(rdsHooks), "rds-dry should have exactly 1 hook (store-outputs)")
	assert.Equal(t, 1, len(lambdaHooks), "lambda-dry should have exactly 1 hook (store-outputs)")

	// All should have the same hook name
	assert.Contains(t, vpcHooks, "store-outputs", "vpc-dry should have store-outputs hook")
	assert.Contains(t, rdsHooks, "store-outputs", "rds-dry should have store-outputs hook")
	assert.Contains(t, lambdaHooks, "store-outputs", "lambda-dry should have store-outputs hook")

	// Verify each hook has the correct outputs for its component
	vpcHook := vpcHooks["store-outputs"].(map[string]any)
	vpcOutputs := vpcHook["outputs"].(map[string]any)
	assert.Contains(t, vpcOutputs, "vpc_id", "vpc-dry should have vpc_id output")
	assert.Contains(t, vpcOutputs, "vpc_cidr_block", "vpc-dry should have vpc_cidr_block output")
	assert.NotContains(t, vpcOutputs, "cluster_endpoint", "vpc-dry should NOT have cluster_endpoint output")
	assert.NotContains(t, vpcOutputs, "lambda_function_arn", "vpc-dry should NOT have lambda_function_arn output")

	rdsHook := rdsHooks["store-outputs"].(map[string]any)
	rdsOutputs := rdsHook["outputs"].(map[string]any)
	assert.Contains(t, rdsOutputs, "cluster_endpoint", "rds-dry should have cluster_endpoint output")
	assert.Contains(t, rdsOutputs, "cluster_id", "rds-dry should have cluster_id output")
	assert.NotContains(t, rdsOutputs, "vpc_id", "rds-dry should NOT have vpc_id output")
	assert.NotContains(t, rdsOutputs, "lambda_function_arn", "rds-dry should NOT have lambda_function_arn output")

	lambdaHook := lambdaHooks["store-outputs"].(map[string]any)
	lambdaOutputs := lambdaHook["outputs"].(map[string]any)
	assert.Contains(t, lambdaOutputs, "lambda_function_arn", "lambda-dry should have lambda_function_arn output")
	assert.Contains(t, lambdaOutputs, "lambda_function_name", "lambda-dry should have lambda_function_name output")
	assert.NotContains(t, lambdaOutputs, "vpc_id", "lambda-dry should NOT have vpc_id output")
	assert.NotContains(t, lambdaOutputs, "cluster_endpoint", "lambda-dry should NOT have cluster_endpoint output")
}

func getHookNames(hooks map[string]any) []string {
	names := make([]string, 0, len(hooks))
	for name := range hooks {
		names = append(names, name)
	}
	return names
}
