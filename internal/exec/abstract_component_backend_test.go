package exec

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAbstractComponentBackendGeneration tests that backend configuration
// for components inheriting from abstract components uses the correct
// workspace_key_prefix based on metadata.component, not the abstract component name.
// This is a regression test for the bug where metadata.component was being overwritten by inherited values.
func TestAbstractComponentBackendGeneration(t *testing.T) {
	workDir := "../../tests/fixtures/scenarios/abstract-component-backend"

	// Save current directory and restore after test.
	startingDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(startingDir)
	})

	// Change to the test directory.
	err = os.Chdir(workDir)
	require.NoError(t, err)

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
}
