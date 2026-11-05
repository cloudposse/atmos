package exec

import (
	"testing"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
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
	assert.Equal(t, map[string]bool{"api_token": true},
		terraformSensitiveVarKeys(&schema.ConfigAndStacksInfo{ComponentSection: componentSection}))

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
	assert.Equal(t, []string{"api_token"}, componentInfo[sensitiveVariablesKey])

	assert.Equal(t, map[string]bool{"api_token": true},
		terraformSensitiveVarKeys(&schema.ConfigAndStacksInfo{ComponentSection: componentSection}))
}

// TestTerraformSensitiveVarKeysFromRecordedNames tests that terraformSensitiveVarKeys reads the recorded
// names when terraform_config is nil, including after a deep copy turns them into []any.
func TestTerraformSensitiveVarKeysFromRecordedNames(t *testing.T) {
	tests := []struct {
		name  string
		names any
		want  map[string]bool
	}{
		{name: "string slice", names: []string{"api_token", "password"}, want: map[string]bool{"api_token": true, "password": true}},
		{name: "any slice", names: []any{"api_token", "password"}, want: map[string]bool{"api_token": true, "password": true}},
		{name: "not recorded", names: nil, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			componentInfo := map[string]any{terraformConfigKey: nil}
			if tt.names != nil {
				componentInfo[sensitiveVariablesKey] = tt.names
			}
			info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{componentInfoKey: componentInfo}}
			assert.Equal(t, tt.want, terraformSensitiveVarKeys(info))
		})
	}
}
