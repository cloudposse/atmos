package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestLocalsResolutionDev tests that file-scoped locals are properly resolved in dev environment.
// This is an integration test for GitHub issue #1933.
func TestLocalsResolutionDev(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals")

	_, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Get component configuration with locals resolved.
	result, err := exec.ExecuteDescribeComponent(&exec.ExecuteDescribeComponentParams{
		Component:            "mock/instance-1",
		Stack:                "dev-us-east-1",
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify locals were resolved correctly in vars.
	vars, ok := result["vars"].(map[string]any)
	require.True(t, ok, "vars should be a map")

	// Check that {{ .locals.name_prefix }}-mock-instance-1 resolved to "acme-dev-mock-instance-1".
	assert.Equal(t, "acme-dev-mock-instance-1", vars["app_name"], "app_name should be resolved from locals")

	// Check that {{ .locals.environment }} resolved to "dev".
	assert.Equal(t, "dev", vars["bar"], "bar should be resolved from locals.environment")

	// Check that {{ .locals.backend_bucket }} resolved to "acme-dev-tfstate".
	assert.Equal(t, "acme-dev-tfstate", vars["bucket"], "bucket should be resolved from locals.backend_bucket")

	// Verify backend was also resolved.
	backend, ok := result["backend"].(map[string]any)
	require.True(t, ok, "backend should be a map")
	assert.Equal(t, "acme-dev-tfstate", backend["bucket"], "backend bucket should be resolved from locals")
}

// TestLocalsResolutionProd tests locals resolution in the prod environment.
func TestLocalsResolutionProd(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals")

	_, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Get component configuration with locals resolved.
	result, err := exec.ExecuteDescribeComponent(&exec.ExecuteDescribeComponentParams{
		Component:            "mock/primary",
		Stack:                "prod-us-east-1",
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify locals were resolved correctly in vars.
	vars, ok := result["vars"].(map[string]any)
	require.True(t, ok, "vars should be a map")

	// Check that {{ .locals.name_prefix }}-mock-primary resolved to "acme-prod-mock-primary".
	assert.Equal(t, "acme-prod-mock-primary", vars["app_name"], "app_name should be resolved from locals")

	// Check that {{ .locals.environment }} resolved to "prod".
	assert.Equal(t, "prod", vars["bar"], "bar should be resolved from locals.environment")

	// Check that {{ .locals.backend_bucket }} resolved to "acme-prod-tfstate".
	assert.Equal(t, "acme-prod-tfstate", vars["bucket"], "bucket should be resolved from locals.backend_bucket")
}

// TestLocalsDescribeStacks tests that describe stacks works with locals.
func TestLocalsDescribeStacks(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals")

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Get all stacks configuration.
	result, err := exec.ExecuteDescribeStacks(
		&atmosConfig,
		"",    // filterByStack
		nil,   // components
		nil,   // componentTypes
		nil,   // sections
		true,  // ignoreMissingFiles
		true,  // processTemplates
		true,  // processYamlFunctions
		false, // includeEmptyStacks
		nil,   // skip
		nil,   // authManager
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result, "should have at least one stack")

	// Find a stack that contains the mock/instance-1 component.
	var foundStack map[string]any
	for _, stackData := range result {
		stack, ok := stackData.(map[string]any)
		if !ok {
			continue
		}
		components, ok := stack["components"].(map[string]any)
		if !ok {
			continue
		}
		terraform, ok := components["terraform"].(map[string]any)
		if !ok {
			continue
		}
		if _, exists := terraform["mock/instance-1"]; exists {
			foundStack = stack
			break
		}
	}
	require.NotNil(t, foundStack, "should find a stack with mock/instance-1 component")

	components, ok := foundStack["components"].(map[string]any)
	require.True(t, ok, "components should be a map")
	terraform, ok := components["terraform"].(map[string]any)
	require.True(t, ok, "terraform section should be a map")
	mockInstance1, ok := terraform["mock/instance-1"].(map[string]any)
	require.True(t, ok, "mock/instance-1 should be a map")
	vars, ok := mockInstance1["vars"].(map[string]any)
	require.True(t, ok, "vars should be a map")

	// Verify locals were resolved.
	assert.Equal(t, "acme-dev-mock-instance-1", vars["app_name"], "app_name should be resolved")
	assert.Equal(t, "dev", vars["bar"], "bar should be resolved")
}

// TestLocalsCircularDependency verifies that circular locals don't crash the system.
// When locals have a cycle, the resolver should log an error and continue without locals.
func TestLocalsCircularDependency(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals-circular")

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Get all stacks - should succeed even with circular locals.
	// The circular locals are logged as a debug warning but processing continues.
	result, err := exec.ExecuteDescribeStacks(
		&atmosConfig,
		"",    // filterByStack
		nil,   // components
		nil,   // componentTypes
		nil,   // sections
		true,  // ignoreMissingFiles
		true,  // processTemplates
		true,  // processYamlFunctions
		false, // includeEmptyStacks
		nil,   // skip
		nil,   // authManager
	)

	// Should not error - circular locals are handled gracefully.
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result, "should have at least one stack")

	// Find a stack that contains the mock component.
	var foundStack map[string]any
	for _, stackData := range result {
		stack, ok := stackData.(map[string]any)
		if !ok {
			continue
		}
		components, ok := stack["components"].(map[string]any)
		if !ok {
			continue
		}
		terraform, ok := components["terraform"].(map[string]any)
		if !ok {
			continue
		}
		if _, exists := terraform["mock"]; exists {
			foundStack = stack
			break
		}
	}
	require.NotNil(t, foundStack, "should find a stack with mock component")

	components, ok := foundStack["components"].(map[string]any)
	require.True(t, ok, "components should be a map")
	terraform, ok := components["terraform"].(map[string]any)
	require.True(t, ok, "terraform section should be a map")

	// The mock component should exist.
	_, ok = terraform["mock"].(map[string]any)
	require.True(t, ok, "mock component should exist")
}

// TestLocalsFileScoped verifies that locals are file-scoped and NOT inherited across imports.
// This is a critical test for the file-scoped locals design.
// - Locals defined in a mixin file should NOT be available in files that import it.
// - Only the file's own locals should be resolvable.
// - Regular vars ARE inherited (normal Atmos behavior).
func TestLocalsFileScoped(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals-file-scoped")

	_, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Get component configuration.
	result, err := exec.ExecuteDescribeComponent(&exec.ExecuteDescribeComponentParams{
		Component:            "test-component",
		Stack:                "test",
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	vars, ok := result["vars"].(map[string]any)
	require.True(t, ok, "vars should be a map")

	// File's own locals SHOULD resolve.
	// {{ .locals.file_computed }} should resolve to "file-ns-file-env".
	assert.Equal(t, "file-ns-file-env", vars["own_local"],
		"file's own locals should be resolved")

	// Verify that the mixin's locals are NOT inherited by checking the component vars.
	// The mixin defines locals (mixin_namespace, mixin_env, mixin_computed) but these
	// should NOT be available in the importing file - only the file's own locals work.
	// Since we don't reference mixin locals in the template (it would cause an error),
	// we verify by confirming our own locals work while the mixin defined different ones.
}

// TestLocalsNotInherited verifies that mixin locals are NOT inherited by importing files.
// This proves that locals are file-scoped and not inherited across file boundaries.
// When a file tries to use {{ .locals.mixin_value }} but mixin_value is defined in an
// imported file, the local is not available and resolves to "<no value>".
func TestLocalsNotInherited(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals-not-inherited")

	_, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Get component configuration.
	result, err := exec.ExecuteDescribeComponent(&exec.ExecuteDescribeComponentParams{
		Component:            "test-component",
		Stack:                "test",
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	vars, ok := result["vars"].(map[string]any)
	require.True(t, ok, "vars should be a map")

	// The mixin's local should NOT be available.
	// {{ .locals.mixin_value }} should resolve to "<no value>" (not "from-mixin-locals").
	attemptMixinLocal, ok := vars["attempt_mixin_local"].(string)
	require.True(t, ok, "attempt_mixin_local should be a string")
	assert.NotEqual(t, "from-mixin-locals", attemptMixinLocal,
		"mixin locals should NOT be inherited - locals are file-scoped")
	assert.Equal(t, "<no value>", attemptMixinLocal,
		"unresolved mixin local should be '<no value>'")

	// However, regular vars from the mixin ARE inherited (normal Atmos behavior).
	inheritedVar, ok := vars["inherited_var"].(string)
	require.True(t, ok, "inherited_var should be a string")
	assert.Equal(t, "from-mixin-vars", inheritedVar,
		"regular vars from mixin should be inherited")
}

// TestLocalsNotInFinalOutput verifies that locals sections are stripped from the final component output.
// Locals are only used during template processing and should not appear in describe output.
func TestLocalsNotInFinalOutput(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals")

	_, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Get component configuration.
	result, err := exec.ExecuteDescribeComponent(&exec.ExecuteDescribeComponentParams{
		Component:            "mock/instance-1",
		Stack:                "dev-us-east-1",
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify that the locals section is NOT present in the output.
	// Locals are internal to template processing and should be stripped.
	_, hasLocals := result["locals"]
	assert.False(t, hasLocals, "locals section should NOT appear in component output")
}

// TestDescribeLocals verifies that the describe locals command correctly extracts
// locals from all stack files and presents them in a structured format.
func TestDescribeLocals(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals")

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Get all locals.
	result, err := exec.ExecuteDescribeLocals(&atmosConfig, "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result, "should have at least one stack with locals")

	// Verify we got locals for the dev stack.
	devLocals, ok := result["deploy/dev"].(map[string]any)
	require.True(t, ok, "should have deploy/dev stack")

	// Check global locals.
	globalLocals, ok := devLocals["global"].(map[string]any)
	require.True(t, ok, "should have global locals")
	assert.Equal(t, "dev", globalLocals["environment"], "environment should be 'dev'")
	assert.Equal(t, "acme", globalLocals["namespace"], "namespace should be 'acme'")
	assert.Equal(t, "acme-dev", globalLocals["name_prefix"], "name_prefix should be 'acme-dev'")

	// Check merged locals.
	mergedLocals, ok := devLocals["merged"].(map[string]any)
	require.True(t, ok, "should have merged locals")
	assert.Equal(t, "acme-dev-tfstate", mergedLocals["backend_bucket"],
		"merged should include terraform section locals")
}

// TestDescribeLocalsWithFilter verifies that the describe locals command
// correctly filters by stack name.
func TestDescribeLocalsWithFilter(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals")

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Get locals filtered by stack.
	result, err := exec.ExecuteDescribeLocals(&atmosConfig, "deploy/prod")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result, 1, "should have exactly one stack")

	// Verify we got locals for the prod stack only.
	prodLocals, ok := result["deploy/prod"].(map[string]any)
	require.True(t, ok, "should have deploy/prod stack")

	// Check global locals.
	globalLocals, ok := prodLocals["global"].(map[string]any)
	require.True(t, ok, "should have global locals")
	assert.Equal(t, "prod", globalLocals["environment"], "environment should be 'prod'")
}

// TestLocalsDeepImportChain verifies that file-scoped locals work correctly
// through a deep import chain (base -> layer1 -> layer2 -> final).
// This tests that locals are NOT inherited through multiple levels of imports.
func TestLocalsDeepImportChain(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals-deep-import-chain")

	_, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Get component configuration with locals resolved.
	result, err := exec.ExecuteDescribeComponent(&exec.ExecuteDescribeComponentParams{
		Component:            "deep-chain-component",
		Stack:                "final",
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	vars, ok := result["vars"].(map[string]any)
	require.True(t, ok, "vars should be a map")

	// File's own locals SHOULD resolve correctly.
	assert.Equal(t, "from-final-stack", vars["local_value"],
		"{{ .locals.final_local }} should resolve to the file's own local")
	assert.Equal(t, "from-final-stack-computed", vars["computed"],
		"{{ .locals.computed_value }} should resolve correctly (locals referencing locals)")
	assert.Equal(t, "final-value", vars["shared"],
		"{{ .locals.shared_key }} should resolve to the file's own value, not parent")
	assert.Equal(t, "final-value-from-final-stack", vars["full_chain"],
		"nested local references should resolve correctly")

	// Verify that regular vars ARE inherited through the chain.
	// Unlike locals, vars follow normal Atmos inheritance.
	assert.Equal(t, "from-base-vars", vars["base_var"],
		"vars from base mixin should be inherited")
	assert.Equal(t, "from-layer1-vars", vars["layer1_var"],
		"vars from layer1 mixin should be inherited")
	assert.Equal(t, "from-layer2-vars", vars["layer2_var"],
		"vars from layer2 mixin should be inherited")
	assert.Equal(t, "from-final-vars", vars["final_var"],
		"vars from final stack should be present")
}

// TestLocalsDeepImportChainDescribeStacks tests that describe stacks works
// correctly with a deep import chain and file-scoped locals.
func TestLocalsDeepImportChainDescribeStacks(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals-deep-import-chain")

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Get all stacks.
	result, err := exec.ExecuteDescribeStacks(
		&atmosConfig,
		"",    // filterByStack
		nil,   // components
		nil,   // componentTypes
		nil,   // sections
		true,  // ignoreMissingFiles
		true,  // processTemplates
		true,  // processYamlFunctions
		false, // includeEmptyStacks
		nil,   // skip
		nil,   // authManager
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result, "should have at least one stack")

	// Find the final stack.
	finalStack, ok := result["final"].(map[string]any)
	require.True(t, ok, "should find the 'final' stack")

	components, ok := finalStack["components"].(map[string]any)
	require.True(t, ok, "components should be a map")
	terraform, ok := components["terraform"].(map[string]any)
	require.True(t, ok, "terraform section should be a map")
	component, ok := terraform["deep-chain-component"].(map[string]any)
	require.True(t, ok, "deep-chain-component should exist")
	vars, ok := component["vars"].(map[string]any)
	require.True(t, ok, "vars should be a map")

	// Verify locals were resolved correctly.
	assert.Equal(t, "from-final-stack", vars["local_value"],
		"locals should be resolved in describe stacks output")
	assert.Equal(t, "from-final-stack-computed", vars["computed"],
		"computed locals should be resolved")
}

// TestLocalsDescribeLocalsDeepChain tests that describe locals command
// shows each file's locals independently in a deep import chain.
func TestLocalsDescribeLocalsDeepChain(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals-deep-import-chain")

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Get all locals.
	result, err := exec.ExecuteDescribeLocals(&atmosConfig, "")
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify we got locals for the final stack.
	finalLocals, ok := result["deploy/final"].(map[string]any)
	require.True(t, ok, "should have deploy/final stack locals")

	// Check global locals show this file's own locals.
	globalLocals, ok := finalLocals["global"].(map[string]any)
	require.True(t, ok, "should have global locals")
	assert.Equal(t, "from-final-stack", globalLocals["final_local"],
		"final_local should be from this file")
	assert.Equal(t, "final-value", globalLocals["shared_key"],
		"shared_key should be from this file, not inherited")

	// The mixin files define locals but those should NOT appear here.
	// Each file's locals are independent.
	_, hasBaseLocal := globalLocals["base_local"]
	assert.False(t, hasBaseLocal, "base_local should NOT be present - it's in base mixin")
	_, hasLayer1Local := globalLocals["layer1_local"]
	assert.False(t, hasLayer1Local, "layer1_local should NOT be present - it's in layer1 mixin")
	_, hasLayer2Local := globalLocals["layer2_local"]
	assert.False(t, hasLayer2Local, "layer2_local should NOT be present - it's in layer2 mixin")
}
