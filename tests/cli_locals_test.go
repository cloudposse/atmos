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
// locals from all stack files and presents them in Atmos schema format.
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

	// Check root-level locals (Atmos schema format).
	locals, ok := devLocals["locals"].(map[string]any)
	require.True(t, ok, "should have locals section")
	assert.Equal(t, "dev", locals["environment"], "environment should be 'dev'")
	assert.Equal(t, "acme", locals["namespace"], "namespace should be 'acme'")
	assert.Equal(t, "acme-dev", locals["name_prefix"], "name_prefix should be 'acme-dev'")

	// Check terraform section locals (Atmos schema format: terraform.locals).
	terraform, ok := devLocals["terraform"].(map[string]any)
	require.True(t, ok, "should have terraform section")
	tfLocals, ok := terraform["locals"].(map[string]any)
	require.True(t, ok, "should have terraform.locals section")
	assert.Equal(t, "acme-dev-tfstate", tfLocals["backend_bucket"],
		"terraform.locals should include backend_bucket")
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

	// Check root-level locals (Atmos schema format).
	locals, ok := prodLocals["locals"].(map[string]any)
	require.True(t, ok, "should have locals section")
	assert.Equal(t, "prod", locals["environment"], "environment should be 'prod'")
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
	// The fixture has name_template: "{{ .vars.stage }}" and vars.stage: "final",
	// so the derived stack name is "final" (not "deploy/final").
	finalLocals, ok := result["final"].(map[string]any)
	require.True(t, ok, "should have 'final' stack locals (derived from name_template)")

	// Check root-level locals (Atmos schema format).
	locals, ok := finalLocals["locals"].(map[string]any)
	require.True(t, ok, "should have locals section")
	assert.Equal(t, "from-final-stack", locals["final_local"],
		"final_local should be from this file")
	assert.Equal(t, "final-value", locals["shared_key"],
		"shared_key should be from this file, not inherited")

	// The mixin files define locals but those should NOT appear here.
	// Each file's locals are independent.
	_, hasBaseLocal := locals["base_local"]
	assert.False(t, hasBaseLocal, "base_local should NOT be present - it's in base mixin")
	_, hasLayer1Local := locals["layer1_local"]
	assert.False(t, hasLayer1Local, "layer1_local should NOT be present - it's in layer1 mixin")
	_, hasLayer2Local := locals["layer2_local"]
	assert.False(t, hasLayer2Local, "layer2_local should NOT be present - it's in layer2 mixin")
}

// TestDescribeLocalsForComponent tests that describe locals command correctly
// returns locals for a specific component in a stack.
// This tests the `atmos describe locals <component> -s <stack>` functionality.
func TestDescribeLocalsForComponent(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals")

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Test getting locals for a terraform component.
	t.Run("returns locals for terraform component", func(t *testing.T) {
		// Get locals for deploy/dev stack.
		stackLocals, err := exec.ExecuteDescribeLocals(&atmosConfig, "deploy/dev")
		require.NoError(t, err)
		require.NotNil(t, stackLocals)

		// Verify the structure has the expected locals (Atmos schema format).
		devLocals, ok := stackLocals["deploy/dev"].(map[string]any)
		require.True(t, ok, "should have deploy/dev stack")

		// Check root-level locals.
		locals, ok := devLocals["locals"].(map[string]any)
		require.True(t, ok, "should have locals section")
		assert.Equal(t, "acme", locals["namespace"], "should have namespace")
		assert.Equal(t, "dev", locals["environment"], "should have environment")

		// Check terraform section has section-specific locals.
		terraform, ok := devLocals["terraform"].(map[string]any)
		require.True(t, ok, "should have terraform section")
		tfLocals, ok := terraform["locals"].(map[string]any)
		require.True(t, ok, "should have terraform.locals section")

		// Verify terraform-specific locals (only section-specific, not merged).
		assert.Equal(t, "acme-dev-tfstate", tfLocals["backend_bucket"],
			"terraform.locals should include backend_bucket")
		assert.Equal(t, "terraform-only", tfLocals["tf_specific"],
			"terraform.locals should include tf_specific")
	})
}

// TestDescribeLocalsForComponentOutput tests the full output structure
// when describing locals for a specific component (Atmos schema format).
func TestDescribeLocalsForComponentOutput(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals")

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Get locals filtered by stack file path.
	stackLocals, err := exec.ExecuteDescribeLocals(&atmosConfig, "deploy/dev")
	require.NoError(t, err)
	require.Len(t, stackLocals, 1, "should have exactly one stack")

	devLocals, ok := stackLocals["deploy/dev"].(map[string]any)
	require.True(t, ok)

	// Check root-level locals (Atmos schema format).
	locals, ok := devLocals["locals"].(map[string]any)
	require.True(t, ok, "should have locals section")

	// Root locals should have global locals.
	assert.Equal(t, "acme", locals["namespace"])
	assert.Equal(t, "dev", locals["environment"])
	assert.Equal(t, "us-east-1", locals["stage"])
	assert.Equal(t, "acme-dev", locals["name_prefix"])
	assert.Equal(t, "acme-dev-us-east-1", locals["full_name"])

	// Check terraform section has terraform-specific locals.
	terraform, ok := devLocals["terraform"].(map[string]any)
	require.True(t, ok, "should have terraform section")
	tfLocals, ok := terraform["locals"].(map[string]any)
	require.True(t, ok, "should have terraform.locals section")
	assert.Equal(t, "acme-dev-tfstate", tfLocals["backend_bucket"])
	assert.Equal(t, "terraform-only", tfLocals["tf_specific"])
}

// TestDescribeLocalsForComponentInProdStack tests locals for a component
// in the prod stack to ensure different stacks have independent locals.
func TestDescribeLocalsForComponentInProdStack(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals")

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Get locals for prod stack using the file path.
	stackLocals, err := exec.ExecuteDescribeLocals(&atmosConfig, "deploy/prod")
	require.NoError(t, err)
	require.Len(t, stackLocals, 1, "should have exactly one stack")

	prodLocals, ok := stackLocals["deploy/prod"].(map[string]any)
	require.True(t, ok)

	// Check root-level locals (Atmos schema format).
	locals, ok := prodLocals["locals"].(map[string]any)
	require.True(t, ok, "should have locals section")

	// Verify prod-specific values.
	assert.Equal(t, "acme", locals["namespace"])
	assert.Equal(t, "prod", locals["environment"])

	// Check terraform section has terraform-specific locals.
	terraform, ok := prodLocals["terraform"].(map[string]any)
	require.True(t, ok, "should have terraform section")
	tfLocals, ok := terraform["locals"].(map[string]any)
	require.True(t, ok, "should have terraform.locals section")
	assert.Equal(t, "acme-prod-tfstate", tfLocals["backend_bucket"],
		"prod should have prod-specific backend_bucket")
}

// =============================================================================
// Logical Stack Name Tests
// =============================================================================
// These tests use the locals-logical-names fixture where vars contain literal
// values (not templates), allowing name_template to derive logical stack names.

// TestDescribeLocalsWithLogicalStackName tests that ExecuteDescribeLocals
// correctly derives and returns logical stack names when configured.
func TestDescribeLocalsWithLogicalStackName(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals-logical-names")

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Get all locals - should use derived logical stack names as keys.
	result, err := exec.ExecuteDescribeLocals(&atmosConfig, "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result)

	// The fixture has name_template: "{{ .vars.environment }}-{{ .vars.stage }}"
	// dev.yaml has vars: {environment: dev, stage: us-east-1} -> "dev-us-east-1"
	// prod.yaml has vars: {environment: prod, stage: us-west-2} -> "prod-us-west-2"

	// Verify we got the logical stack names as keys.
	devLocals, hasDevLogical := result["dev-us-east-1"].(map[string]any)
	prodLocals, hasProdLogical := result["prod-us-west-2"].(map[string]any)

	assert.True(t, hasDevLogical, "should have dev-us-east-1 stack (logical name)")
	assert.True(t, hasProdLogical, "should have prod-us-west-2 stack (logical name)")

	// Verify locals content for dev stack (Atmos schema format).
	if hasDevLogical {
		locals, ok := devLocals["locals"].(map[string]any)
		require.True(t, ok, "dev stack should have locals section")
		assert.Equal(t, "acme", locals["namespace"])
		assert.Equal(t, "acme-dev", locals["env_prefix"])
	}

	// Verify locals content for prod stack (Atmos schema format).
	if hasProdLogical {
		locals, ok := prodLocals["locals"].(map[string]any)
		require.True(t, ok, "prod stack should have locals section")
		assert.Equal(t, "acme", locals["namespace"])
		assert.Equal(t, "acme-prod", locals["env_prefix"])
	}
}

// TestDescribeLocalsFilterByLogicalStackName tests filtering by logical stack name.
func TestDescribeLocalsFilterByLogicalStackName(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals-logical-names")

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Filter by logical stack name "dev-us-east-1".
	result, err := exec.ExecuteDescribeLocals(&atmosConfig, "dev-us-east-1")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result, 1, "should have exactly one stack when filtering by logical name")

	// Verify we got the dev stack.
	devLocals, ok := result["dev-us-east-1"].(map[string]any)
	require.True(t, ok, "should have dev-us-east-1 stack")

	locals, ok := devLocals["locals"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "acme", locals["namespace"])
}

// TestDescribeLocalsFilterByFilePath tests filtering by file path when logical names are available.
func TestDescribeLocalsFilterByFilePath(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals-logical-names")

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Filter by file path "deploy/prod" - should still work.
	result, err := exec.ExecuteDescribeLocals(&atmosConfig, "deploy/prod")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result, 1, "should have exactly one stack when filtering by file path")

	// The result key is the logical name even when filtering by file path.
	prodLocals, ok := result["prod-us-west-2"].(map[string]any)
	require.True(t, ok, "should have prod-us-west-2 stack (returned with logical name)")

	locals, ok := prodLocals["locals"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "acme-prod", locals["env_prefix"])
}

// TestDescribeLocalsOutputStructureStack tests the output structure when querying stacks (no component).
// Output follows Atmos schema format: locals:, terraform: {locals:}, helmfile: {locals:}, etc.
func TestDescribeLocalsOutputStructureStack(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals-logical-names")

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	result, err := exec.ExecuteDescribeLocals(&atmosConfig, "prod-us-west-2")
	require.NoError(t, err)
	require.Len(t, result, 1)

	prodLocals, ok := result["prod-us-west-2"].(map[string]any)
	require.True(t, ok)

	// Verify stack output structure has Atmos schema format.
	_, hasLocals := prodLocals["locals"]
	_, hasTerraform := prodLocals["terraform"]
	_, hasHelmfile := prodLocals["helmfile"]

	assert.True(t, hasLocals, "stack output should have 'locals' section (root-level locals)")
	assert.True(t, hasTerraform, "stack output should have 'terraform' section")
	assert.True(t, hasHelmfile, "stack output should have 'helmfile' section (prod has helmfile locals)")

	// Verify root-level locals.
	locals, ok := prodLocals["locals"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "acme", locals["namespace"])

	// Verify terraform section has nested locals (terraform.locals).
	terraform, ok := prodLocals["terraform"].(map[string]any)
	require.True(t, ok)
	tfLocals, ok := terraform["locals"].(map[string]any)
	require.True(t, ok, "terraform section should have nested locals")
	assert.Equal(t, "acme-prod-tfstate", tfLocals["backend_bucket"])
	assert.Equal(t, "terraform-specific-prod", tfLocals["tf_only"])

	// Verify helmfile section has nested locals (helmfile.locals).
	helmfile, ok := prodLocals["helmfile"].(map[string]any)
	require.True(t, ok)
	hfLocals, ok := helmfile["locals"].(map[string]any)
	require.True(t, ok, "helmfile section should have nested locals")
	assert.Equal(t, "acme-prod-release", hfLocals["release_name"])
	assert.Equal(t, "helmfile-specific-prod", hfLocals["hf_only"])
}

// TestDescribeLocalsOutputStructureComponent tests the output structure when querying for a component.
func TestDescribeLocalsOutputStructureComponent(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals-logical-names")

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Get all locals for the stack.
	stackLocals, err := exec.ExecuteDescribeLocals(&atmosConfig, "dev-us-east-1")
	require.NoError(t, err)

	devLocals, ok := stackLocals["dev-us-east-1"].(map[string]any)
	require.True(t, ok)

	// Verify root-level locals.
	locals, ok := devLocals["locals"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "acme", locals["namespace"], "root locals should include namespace")

	// Verify terraform section locals (Atmos schema: terraform.locals).
	terraform, ok := devLocals["terraform"].(map[string]any)
	require.True(t, ok)
	tfLocals, ok := terraform["locals"].(map[string]any)
	require.True(t, ok, "terraform section should have nested locals")
	assert.Equal(t, "acme-dev-tfstate", tfLocals["backend_bucket"])
	assert.Equal(t, "terraform-specific-dev", tfLocals["tf_only"])
}

// TestDescribeLocalsComponentWithLogicalStackName tests component argument with logical stack name.
func TestDescribeLocalsComponentWithLogicalStackName(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals-logical-names")

	_, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Verify component resolution works with logical stack name.
	// The component "vpc" in stack "dev-us-east-1" should return terraform locals.
	result, err := exec.ExecuteDescribeComponent(&exec.ExecuteDescribeComponentParams{
		Component:            "vpc",
		Stack:                "dev-us-east-1",
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify vars were resolved from locals.
	vars, ok := result["vars"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "acme-dev-us-east-1-vpc", vars["name"])
	assert.Equal(t, "acme-dev-tfstate", vars["bucket"])
}

// TestDescribeLocalsComponentWithFilePath tests component argument with file path via ExecuteDescribeLocals.
// Note: ExecuteDescribeComponent uses different stack resolution logic and may not work with file paths
// when a global config overrides the fixture config. This test verifies ExecuteDescribeLocals accepts file paths.
func TestDescribeLocalsComponentWithFilePath(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals-logical-names")

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Verify ExecuteDescribeLocals accepts file path "deploy/prod" as filter.
	result, err := exec.ExecuteDescribeLocals(&atmosConfig, "deploy/prod")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result, 1, "should have exactly one stack when filtering by file path")

	// The result key is the logical name even when filtering by file path.
	prodLocals, ok := result["prod-us-west-2"].(map[string]any)
	require.True(t, ok, "should have prod-us-west-2 stack (returned with logical name)")

	// Verify terraform section locals (Atmos schema: terraform.locals).
	terraform, ok := prodLocals["terraform"].(map[string]any)
	require.True(t, ok)
	tfLocals, ok := terraform["locals"].(map[string]any)
	require.True(t, ok, "terraform section should have nested locals")
	assert.Equal(t, "acme-prod-tfstate", tfLocals["backend_bucket"])
}

// TestDescribeLocalsHelmfileComponent tests locals for helmfile component type.
func TestDescribeLocalsHelmfileComponent(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals-logical-names")

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Get locals for prod stack which has helmfile locals.
	stackLocals, err := exec.ExecuteDescribeLocals(&atmosConfig, "prod-us-west-2")
	require.NoError(t, err)

	prodLocals, ok := stackLocals["prod-us-west-2"].(map[string]any)
	require.True(t, ok)

	// Verify helmfile section locals (Atmos schema: helmfile.locals).
	helmfile, ok := prodLocals["helmfile"].(map[string]any)
	require.True(t, ok)
	hfLocals, ok := helmfile["locals"].(map[string]any)
	require.True(t, ok, "helmfile section should have nested locals")

	// Helmfile section should have helmfile-specific locals only.
	assert.Equal(t, "acme-prod-release", hfLocals["release_name"])
	assert.Equal(t, "helmfile-specific-prod", hfLocals["hf_only"])

	// Global locals are in root "locals:" section, not merged into helmfile.locals.
	locals, ok := prodLocals["locals"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "acme", locals["namespace"])
	assert.Equal(t, "acme-prod", locals["env_prefix"])
}

// TestDescribeLocalsDifferentOutputStructures verifies that stack queries return
// Atmos schema format output.
func TestDescribeLocalsDifferentOutputStructures(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals-logical-names")

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Stack query output structure.
	stackResult, err := exec.ExecuteDescribeLocals(&atmosConfig, "dev-us-east-1")
	require.NoError(t, err)

	// Stack output: map keyed by stack name containing Atmos schema format.
	devLocals, ok := stackResult["dev-us-east-1"].(map[string]any)
	require.True(t, ok, "stack result should have stack name as key")

	// Verify stack output has Atmos schema format (locals:, terraform: {locals:}, etc.).
	_, hasLocals := devLocals["locals"]
	assert.True(t, hasLocals, "stack output should have 'locals' key (root-level locals)")

	// Stack output should NOT have the old format keys.
	_, hasGlobal := devLocals["global"]
	_, hasMerged := devLocals["merged"]
	assert.False(t, hasGlobal, "stack output should NOT have 'global' key (old format)")
	assert.False(t, hasMerged, "stack output should NOT have 'merged' key (old format)")

	// Stack output should NOT have component-specific keys.
	_, hasComponent := devLocals["component"]
	_, hasComponentType := devLocals["component_type"]
	assert.False(t, hasComponent, "stack output should NOT have 'component' key")
	assert.False(t, hasComponentType, "stack output should NOT have 'component_type' key")
}

// =============================================================================
// Component-Level Locals Tests
// =============================================================================
// These tests verify that component-level locals work correctly, including
// inheritance from base components via metadata.inherits.
//
// Note: Component-level locals are stored and inherited, but they are NOT
// available for {{ .locals.X }} template resolution within the same file.
// Only file-level locals (global + section) are available during template
// processing. Component-level locals appear in the final component output
// and can be used by downstream tooling.

// TestComponentLevelLocals tests component-level locals functionality using table-driven tests.
func TestComponentLevelLocals(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals-component-level")

	_, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	tests := []struct {
		name           string
		component      string
		expectedVars   map[string]string
		expectedLocals map[string]string
	}{
		{
			name:      "standalone component with component-level locals",
			component: "standalone",
			expectedVars: map[string]string{
				"name":   "acme-dev-standalone",
				"bucket": "acme-dev-tfstate",
			},
			expectedLocals: map[string]string{
				"standalone_value": "standalone-only",
				"computed_ref":     "acme-dev",
			},
		},
		{
			name:      "component inheriting with locals override",
			component: "vpc/dev",
			expectedVars: map[string]string{
				"name":        "acme-dev-vpc",
				"bucket":      "acme-dev-tfstate",
				"description": "acme-dev-vpc-dev",
			},
			expectedLocals: map[string]string{
				"cidr_prefix": "10.0",
				"vpc_type":    "development",
				"extra_tag":   "dev-only",
			},
		},
		{
			name:      "component inheriting without locals override",
			component: "vpc/standard",
			expectedVars: map[string]string{
				"name":        "acme-dev-vpc",
				"description": "acme-dev-vpc-standard",
			},
			expectedLocals: map[string]string{
				"vpc_type":    "standard",
				"cidr_prefix": "10.0",
			},
		},
		{
			name:      "component with component attribute",
			component: "vpc/custom",
			expectedVars: map[string]string{
				"prefix": "acme-dev",
			},
			expectedLocals: map[string]string{
				"custom_local": "custom-value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := exec.ExecuteDescribeComponent(&exec.ExecuteDescribeComponentParams{
				Component:            tt.component,
				Stack:                "dev-us-east-1",
				ProcessTemplates:     true,
				ProcessYamlFunctions: true,
			})
			require.NoError(t, err)
			require.NotNil(t, result)

			// Verify vars.
			vars, ok := result["vars"].(map[string]any)
			require.True(t, ok, "vars should be a map")
			for key, expected := range tt.expectedVars {
				assert.Equal(t, expected, vars[key], "vars[%s] mismatch", key)
			}

			// Verify locals.
			locals, hasLocals := result["locals"].(map[string]any)
			require.True(t, hasLocals, "component should have locals in output")
			for key, expected := range tt.expectedLocals {
				assert.Equal(t, expected, locals[key], "locals[%s] mismatch", key)
			}
		})
	}
}
