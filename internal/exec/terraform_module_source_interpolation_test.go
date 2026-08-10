package exec

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTerraformModuleSourceInterpolation tests that Terraform 1.15+ `const`-variable
// module source interpolation works with Atmos's terraform-config-inspect validation,
// using plain `terraform` (no OpenTofu, no `command:` override).
//
// This addresses issue #2913, where the existing OpenTofu-only skip for the
// "Variables not allowed" diagnostic left plain-Terraform users hitting a hard failure
// even though Terraform 1.15+ now supports the identical pattern via `const = true`.
func TestTerraformModuleSourceInterpolation(t *testing.T) {
	workDir := "../../tests/fixtures/scenarios/terraform-module-source-interpolation"

	t.Chdir(workDir)

	component := "test-component"
	stack := "test"

	t.Run("describe component with const-variable module source interpolation", func(t *testing.T) {
		// Before the fix, this fails with:
		// "failed to load terraform component ... Variables not allowed: Variables may not be used here"
		// even though no `command: tofu` override is configured.
		componentSection, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
			Component:            component,
			Stack:                stack,
			ProcessTemplates:     false,
			ProcessYamlFunctions: false,
			Skip:                 []string{},
			AuthManager:          nil,
		})

		require.NoError(t, err, "ExecuteDescribeComponent should not fail for Terraform const-variable module source interpolation")
		require.NotNil(t, componentSection, "Component section should not be nil")

		componentVars, ok := componentSection["vars"].(map[string]any)
		require.True(t, ok, "Component vars should be a map")
		require.Equal(t, "acme", componentVars["org"], "org should be preserved")
	})

	t.Run("component info validation skipped for plain terraform", func(t *testing.T) {
		componentSection, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
			Component:            component,
			Stack:                stack,
			ProcessTemplates:     false,
			ProcessYamlFunctions: false,
			Skip:                 []string{},
			AuthManager:          nil,
		})
		require.NoError(t, err)

		componentInfo, ok := componentSection["component_info"].(map[string]any)
		require.True(t, ok, "component_info should be present")

		// Unconditional (not a soft/optional check): proves the skip applies to plain
		// `terraform`, not just OpenTofu.
		skipped, exists := componentInfo["validation_skipped_module_source_interpolation"]
		require.True(t, exists, "validation_skipped_module_source_interpolation flag should be present")
		require.Equal(t, true, skipped, "validation_skipped_module_source_interpolation should be true for plain Terraform const-variable interpolation")

		componentPath, ok := componentInfo["component_path"].(string)
		require.True(t, ok, "component_path should be a string")
		require.Contains(t, componentPath, "test-component", "Component path should point to test-component directory")
	})
}
