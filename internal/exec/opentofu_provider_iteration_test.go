package exec

import (
	"testing"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	terraformcomponent "github.com/cloudposse/atmos/pkg/component/terraform"
)

// TestOpenTofuProviderIteration tests that OpenTofu 1.9+ provider iteration
// (`provider = aws.by_account[each.value]`) works with Atmos's terraform-config-inspect validation.
//
// This addresses issue #3483, where terraform-config-inspect's "Invalid provider reference" diagnostic
// for an indexed provider alias failed `atmos describe component` and `atmos describe affected`.
func TestOpenTofuProviderIteration(t *testing.T) {
	t.Chdir("../../tests/fixtures/scenarios/opentofu-provider-iteration")

	componentSection, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "test-component",
		Stack:                "test",
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		Skip:                 []string{},
		AuthManager:          nil,
	})
	require.NoError(t, err, "ExecuteDescribeComponent should not fail for OpenTofu provider iteration")
	require.NotNil(t, componentSection)

	componentInfo, ok := componentSection["component_info"].(map[string]any)
	require.True(t, ok, "component_info should be a map")

	// The skip is recorded under its own reason, not as module source interpolation.
	assert.Equal(t, true, componentInfo["validation_skipped_provider_iteration"])
	assert.NotContains(t, componentInfo, "validation_skipped_module_source_interpolation")

	// The parsed module is kept, so sensitive variables are still kept out of the varfile.
	module, ok := componentInfo[terraformConfigKey].(*tfconfig.Module)
	require.True(t, ok, "%s should hold the parsed module", terraformConfigKey)
	require.NotNil(t, module)
	require.Contains(t, module.Variables, "api_token")
	assert.True(t, module.Variables["api_token"].Sensitive, "api_token should stay declared sensitive")

	componentVars, ok := componentSection["vars"].(map[string]any)
	require.True(t, ok, "component vars should be a map")
	assert.Equal(t, []any{"dev", "prod"}, componentVars["accounts"])
}

// TestOpenTofuProviderIterationWithModuleSourceInterpolation tests a component that combines provider
// iteration with module source interpolation. Validation is skipped for the interpolated module source, so
// terraform_config is nil, but the names of `sensitive = true` variables must still be recorded so the
// generated varfile leaves them out.
func TestOpenTofuProviderIterationWithModuleSourceInterpolation(t *testing.T) {
	t.Chdir("../../tests/fixtures/scenarios/opentofu-provider-iteration")

	componentSection, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "mixed-component",
		Stack:                "test",
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		Skip:                 []string{},
		AuthManager:          nil,
	})
	require.NoError(t, err, "ExecuteDescribeComponent should not fail for provider iteration with module source interpolation")

	componentInfo, ok := componentSection["component_info"].(map[string]any)
	require.True(t, ok, "component_info should be a map")
	assert.Equal(t, true, componentInfo["validation_skipped_provider_iteration"])
	assert.Equal(t, true, componentInfo["validation_skipped_module_source_interpolation"])
	assert.Nil(t, componentInfo[terraformConfigKey], "%s should be nil when validation was skipped", terraformConfigKey)
	assert.Equal(t, []string{"api_token"}, componentInfo[terraformcomponent.SensitiveVariablesKey])
}
