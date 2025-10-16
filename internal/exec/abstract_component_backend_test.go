package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAbstractComponentBackendGeneration tests that backend configuration
// for components inheriting from abstract components uses the correct
// workspace_key_prefix based on metadata.component, not the abstract component name.
func TestAbstractComponentBackendGeneration(t *testing.T) {
	workDir := "../../tests/fixtures/scenarios/abstract-component-backend"

	// Change to the test directory.
	t.Chdir(workDir)

	component := "eks/service/app1"
	stack := "tenant1-ue2-dev"

	// Describe the component to get its configuration.
	componentSection, err := ExecuteDescribeComponent(component, stack, false, false, []string{})
	require.NoError(t, err, "ExecuteDescribeComponent should work for component inheriting from abstract component")

	// Verify that the component section is not nil.
	require.NotNil(t, componentSection, "Component section should not be nil")

	// Extract backend configuration.
	backend, ok := componentSection["backend"].(map[string]any)
	require.True(t, ok, "Backend should be a map")

	// Verify backend type.
	backendType, ok := componentSection["backend_type"].(string)
	require.True(t, ok, "backend_type should be a string")
	assert.Equal(t, "s3", backendType, "Backend type should be s3")

	// The critical test: workspace_key_prefix should be based on metadata.component (eks-service)
	// NOT on the abstract component name (eks-service-defaults).
	// This ensures the component finds its existing state instead of creating a new one.
	workspaceKeyPrefix, ok := backend["workspace_key_prefix"].(string)
	require.True(t, ok, "workspace_key_prefix should be a string")

	// Verify the component path is correct.
	componentPath, ok := componentSection["component"].(string)
	require.True(t, ok, "Component path should be a string")

	// Expected: "eks-service" (from metadata.component)
	// Wrong (regression): "eks-service-defaults" or "eks/service/defaults" (from abstract component name)
	assert.Equal(t, "eks-service", componentPath, "Component should point to eks-service directory")
	assert.Equal(t, "eks-service", workspaceKeyPrefix,
		"workspace_key_prefix should be derived from metadata.component (eks-service), not from abstract component name")

	// Verify inheritance chain includes the abstract component.
	inheritance, ok := componentSection["inheritance"].([]string)
	require.True(t, ok, "Inheritance should be a string array")
	assert.Contains(t, inheritance, "eks/service/defaults", "Inheritance chain should include abstract component")

	// Verify that vars are inherited correctly.
	componentVars, ok := componentSection["vars"].(map[string]any)
	require.True(t, ok, "Component vars should be a map")

	// Verify vars from both abstract and concrete component.
	assert.Equal(t, "app1", componentVars["name"], "Component name should be from concrete component")
	assert.Equal(t, "acme", componentVars["namespace"], "Should inherit namespace from abstract component")
	assert.Equal(t, true, componentVars["enabled"], "Should inherit enabled flag from abstract component")

	// Verify metadata.component is preserved (not overwritten by abstract component).
	metadata, ok := componentSection["metadata"].(map[string]any)
	require.True(t, ok, "Metadata should be a map")

	metadataComponent, ok := metadata["component"].(string)
	require.True(t, ok, "metadata.component should be a string")
	assert.Equal(t, "eks-service", metadataComponent, "metadata.component should be eks-service, not eks-service-defaults")

	// Verify metadata.inherits is present.
	inherits, ok := metadata["inherits"].([]any)
	require.True(t, ok, "metadata.inherits should be present")
	assert.Contains(t, inherits, "eks/service/defaults", "Should inherit from abstract component")

	// Verify component_info has correct path.
	componentInfo, ok := componentSection["component_info"].(map[string]any)
	require.True(t, ok, "component_info should be a map")
	componentInfoPath, ok := componentInfo["component_path"].(string)
	require.True(t, ok, "component_path should be a string")
	assert.Contains(t, componentInfoPath, "eks-service", "Component path should point to eks-service directory")
}
