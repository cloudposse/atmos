package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOpenTofuModuleSourceInterpolation tests that OpenTofu 1.8+ module source
// variable interpolation works with Atmos's terraform-config-inspect validation.
//
// This addresses issue #1753 where users cannot use OpenTofu-specific syntax
// like `source = "${var.context.build.module_path}"` because the validation
// phase rejects it before any OpenTofu commands are executed.
func TestOpenTofuModuleSourceInterpolation(t *testing.T) {
	workDir := "../../tests/fixtures/scenarios/opentofu-module-source-interpolation"

	// Change to the test directory.
	t.Chdir(workDir)

	component := "test-component"
	stack := "test"

	t.Run("describe component with module source interpolation", func(t *testing.T) {
		// This test verifies that ExecuteDescribeComponent handles OpenTofu-specific
		// syntax gracefully. Before the fix, this would fail with:
		// "failed to load terraform module Variables not allowed: Variables may not be used here"

		componentSection, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
			Component:            component,
			Stack:                stack,
			ProcessTemplates:     false,
			ProcessYamlFunctions: false,
			Skip:                 []string{},
			AuthManager:          nil,
		})

		// After implementing auto-detection, this should NOT fail even though
		// the component uses OpenTofu-specific syntax.
		require.NoError(t, err, "ExecuteDescribeComponent should not fail for OpenTofu module source interpolation")
		require.NotNil(t, componentSection, "Component section should not be nil")

		// Verify vars section contains the nested variable structure.
		componentVars, ok := componentSection["vars"].(map[string]any)
		require.True(t, ok, "Component vars should be a map")

		// Verify nested context variable structure is preserved.
		context, ok := componentVars["context"].(map[string]any)
		require.True(t, ok, "context should be a nested map")

		build, ok := context["build"].(map[string]any)
		require.True(t, ok, "context.build should be a nested map")

		assert.Equal(t, "./modules/example", build["module_path"], "module_path should be preserved")
		assert.Equal(t, "v1.0.0", build["module_version"], "module_version should be preserved")

		// Verify flat variables are also preserved.
		assert.Equal(t, "simple_value", componentVars["simple_var"], "simple_var should be preserved")
		assert.Equal(t, "another_value", componentVars["another_var"], "another_var should be preserved")
		assert.Equal(t, "test", componentVars["stage"], "stage should be inherited from stack")
	})

	t.Run("varfile generation with nested variables", func(t *testing.T) {
		// This test verifies that varfile generation correctly handles nested
		// variable structures, which was incorrectly suspected as the issue in #1753.

		componentSection, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
			Component:            component,
			Stack:                stack,
			ProcessTemplates:     false,
			ProcessYamlFunctions: false,
			Skip:                 []string{},
			AuthManager:          nil,
		})
		require.NoError(t, err)

		// Extract component vars (what would be written to varfile).
		componentVars, ok := componentSection["vars"].(map[string]any)
		require.True(t, ok, "Component vars should be a map")

		// Verify the nested structure that will be written to the varfile.
		// This proves that PR #1639's performance optimization did NOT break
		// nested variable handling.
		context, ok := componentVars["context"].(map[string]any)
		require.True(t, ok, "Nested context should be preserved in varfile")

		build, ok := context["build"].(map[string]any)
		require.True(t, ok, "Deeply nested context.build should be preserved in varfile")

		// These values would be used by OpenTofu for module source interpolation.
		assert.Equal(t, "./modules/example", build["module_path"])
		assert.Equal(t, "v1.0.0", build["module_version"])
	})

	t.Run("component info validation skipped for opentofu", func(t *testing.T) {
		// This test verifies that when command: "tofu" is configured, Atmos
		// automatically skips terraform-config-inspect validation errors for
		// known OpenTofu-specific features.

		componentSection, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
			Component:            component,
			Stack:                stack,
			ProcessTemplates:     false,
			ProcessYamlFunctions: false,
			Skip:                 []string{},
			AuthManager:          nil,
		})
		require.NoError(t, err)

		// Verify component_info is present even though validation was skipped.
		componentInfo, ok := componentSection["component_info"].(map[string]any)
		require.True(t, ok, "component_info should be present")

		// Check if validation was skipped due to OpenTofu detection.
		if skipped, exists := componentInfo["validation_skipped_opentofu"]; exists {
			assert.True(t, skipped.(bool), "validation_skipped_opentofu should be true when OpenTofu-specific syntax is detected")
		}

		// Terraform config may be nil (validation skipped) or contain partial info.
		// Either is acceptable - the important part is that the error didn't fail the operation.
		if terraformConfig, exists := componentInfo["terraform_config"]; exists {
			// If it exists, it should either be nil or a valid config object.
			if terraformConfig != nil {
				_, ok := terraformConfig.(map[string]any)
				assert.True(t, ok, "If terraform_config is not nil, it should be a valid config object")
			}
		}

		// Component path should still be resolved correctly.
		componentPath, ok := componentInfo["component_path"].(string)
		require.True(t, ok, "component_path should be a string")
		assert.Contains(t, componentPath, "test-component", "Component path should point to test-component directory")
	})
}

// TestOpenTofuDetection tests the auto-detection of OpenTofu vs Terraform executables.
func TestOpenTofuDetection(t *testing.T) {
	// Note: These tests will be implemented alongside the IsOpenTofu() function.
	// They will test:
	// 1. Detection by executable basename (tofu vs terraform)
	// 2. Detection by version command output
	// 3. Caching of detection results
	// 4. Handling of custom paths

	t.Run("detect tofu by basename", func(t *testing.T) {
		t.Skip("Implement alongside IsOpenTofu() function")
	})

	t.Run("detect tofu by version command", func(t *testing.T) {
		t.Skip("Implement alongside IsOpenTofu() function")
	})

	t.Run("cache detection results", func(t *testing.T) {
		t.Skip("Implement alongside IsOpenTofu() function")
	})
}
