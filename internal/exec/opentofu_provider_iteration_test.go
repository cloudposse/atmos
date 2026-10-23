package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.Nil(t, componentInfo[terraformConfigKey], "%s should be nil when validation was skipped", terraformConfigKey)

	componentVars, ok := componentSection["vars"].(map[string]any)
	require.True(t, ok, "component vars should be a map")
	assert.Equal(t, []any{"dev", "prod"}, componentVars["accounts"])
}
