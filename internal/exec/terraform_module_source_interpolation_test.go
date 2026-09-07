package exec

import (
	"errors"
	"testing"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// assertModuleSourceInterpolationHandledCorrectly verifies that terraform-config-inspect either
// parsed the component's Terraform config successfully, or -- if it raised the known
// module-source-interpolation diagnostic -- Atmos correctly recorded the skip.
//
// Whether the diagnostic fires at all depends on the vendored terraform-config-inspect version:
// newer versions may tolerate a variable reference in module.source/version directly, in which
// case there's nothing for Atmos to skip. Both outcomes are correct; only a nil terraform_config
// with no skip flag would indicate a real regression (an error was silently dropped without
// being recorded).
func assertModuleSourceInterpolationHandledCorrectly(t *testing.T, componentInfo map[string]any) {
	t.Helper()

	terraformConfig, hasConfig := componentInfo[terraformConfigKey]
	require.True(t, hasConfig, "%s should be present", terraformConfigKey)

	if skipped, skippedExists := componentInfo["validation_skipped_module_source_interpolation"]; skippedExists {
		require.Equal(t, true, skipped, "validation_skipped_module_source_interpolation should be true when present")
		require.Nil(t, terraformConfig, "%s should be nil when validation was skipped", terraformConfigKey)
		return
	}

	require.NotNil(t, terraformConfig, "%s should be parsed when validation was not skipped", terraformConfigKey)
	_, ok := terraformConfig.(*tfconfig.Module)
	require.True(t, ok, "%s should be a parsed Terraform module when not skipped", terraformConfigKey)
}

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

		assertModuleSourceInterpolationHandledCorrectly(t, componentInfo)

		componentPath, ok := componentInfo["component_path"].(string)
		require.True(t, ok, "component_path should be a string")
		require.Contains(t, componentPath, "test-component", "Component path should point to test-component directory")
	})
}

// TestTerraformModuleSourceInterpolationDoesNotSwallowUnrelatedErrors guards against a genuine,
// unrelated HCL error being silently discarded when it co-occurs, in the same module, with a
// known-safe module-source-interpolation diagnostic that sorts before it.
//
// Terraform-config-inspect's Diagnostics.Error() only renders the FIRST diagnostic's text,
// collapsing any others to "(and N other messages)" with no content. Before the fix, Atmos
// pattern-matched that collapsed string: if the module-source diagnostic happened to be
// diags[0], the match succeeded and the whole diagnostics set -- including any other real
// error -- was silently discarded (component_info["validation_skipped_module_source_interpolation"]
// = true, no error returned). The fixture's module block (source = "./mods/${var.org}", known-safe)
// is declared before an output block with a genuinely invalid `sensitive` value (a list, not a
// bool) -- unrelated to module source interpolation and must never be silently accepted.
func TestTerraformModuleSourceInterpolationDoesNotSwallowUnrelatedErrors(t *testing.T) {
	workDir := "../../tests/fixtures/scenarios/terraform-module-source-interpolation-mixed-diagnostics"

	t.Chdir(workDir)

	_, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "test-component",
		Stack:                "test",
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		Skip:                 []string{},
		AuthManager:          nil,
	})

	require.Error(t, err, "the genuine 'sensitive must be bool' error must not be silently swallowed by the module-source-interpolation skip")
	require.True(t, errors.Is(err, errUtils.ErrFailedToLoadTerraformComponent),
		"error should be the standard invalid-HCL error, not a silent success")
}
