package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestComponentInheritanceWithoutMetadataComponent tests that components with
// metadata.inherits but no explicit metadata.component work correctly.
func TestComponentInheritanceWithoutMetadataComponent(t *testing.T) {
	workDir := "../../tests/fixtures/scenarios/component-inheritance-without-metadata-component"

	// Change to the test directory.
	t.Chdir(workDir)

	component := "derived-component"
	stack := "test"

	// Describe the component.
	componentSection, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            component,
		Stack:                stack,
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		Skip:                 []string{},
		AuthManager:          nil,
	})
	require.NoError(t, err, "ExecuteDescribeComponent should not fail for component with metadata.inherits but no metadata.component")

	// Verify that the component section is not nil.
	require.NotNil(t, componentSection, "Component section should not be nil")

	// Extract key fields from the component section.
	componentVars, ok := componentSection["vars"].(map[string]any)
	require.True(t, ok, "Component vars should be a map")

	// Verify that vars from both base and derived component are present.
	assert.Equal(t, "derived-component", componentVars["name"], "Component name should be from derived component")
	assert.Equal(t, "base-value", componentVars["base_var"], "Should inherit base_var from base-component")
	assert.Equal(t, "derived-value", componentVars["derived_var"], "Should have derived_var from derived component")

	// Verify component path is set correctly.
	// When metadata.component is not set, it should default to the component name itself.
	componentPath, ok := componentSection["component"].(string)
	require.True(t, ok, "Component path should be a string")
	assert.Equal(t, "derived-component", componentPath, "Component path should default to component name when metadata.component is not set")

	// Verify inheritance chain includes the base component.
	inheritance, ok := componentSection["inheritance"].([]string)
	require.True(t, ok, "Inheritance should be a string array")
	assert.Contains(t, inheritance, "base-component", "Inheritance chain should include base-component")

	// Verify that the component metadata does not contain inherited metadata.
	// The metadata section is per-component and should not be inherited.
	metadata, ok := componentSection["metadata"].(map[string]any)
	require.True(t, ok, "Metadata should be a map")

	// Verify metadata.inherits is present (from the derived component).
	inherits, ok := metadata["inherits"].([]any)
	require.True(t, ok, "metadata.inherits should be present")
	assert.Len(t, inherits, 1, "Should have one inheritance entry")
	assert.Equal(t, "base-component", inherits[0], "Should inherit from base-component")

	// Verify component_info section is present and correct.
	componentInfo, ok := componentSection["component_info"].(map[string]any)
	require.True(t, ok, "component_info should be a map")
	componentInfoPath, ok := componentInfo["component_path"].(string)
	require.True(t, ok, "component_path should be a string")
	assert.Contains(t, componentInfoPath, "derived-component", "Component path should point to derived-component directory")
}
