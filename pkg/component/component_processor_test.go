package component

import (
	"testing"
	"path/filepath"
	"strings"

	"github.com/stretchr/testify/assert"

	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestComponentProcessor(t *testing.T) {
	var err error
	var component string
	var stack string
	namespace := ""
	var yamlConfig string

	var tenant1Ue2DevTestTestComponent map[string]any
	component = "test/test-component"
	stack = "tenant1-ue2-dev"
	tenant1Ue2DevTestTestComponent, err = ProcessComponentInStack(component, stack, "", "")
	assert.Nil(t, err)
	tenant1Ue2DevTestTestComponentBackend := tenant1Ue2DevTestTestComponent["backend"].(map[string]any)
	tenant1Ue2DevTestTestComponentRemoteStateBackend := tenant1Ue2DevTestTestComponent["remote_state_backend"].(map[string]any)
	tenant1Ue2DevTestTestComponentTerraformWorkspace := tenant1Ue2DevTestTestComponent["workspace"].(string)
	tenant1Ue2DevTestTestComponentBackendWorkspaceKeyPrefix := tenant1Ue2DevTestTestComponentBackend["workspace_key_prefix"].(string)
	tenant1Ue2DevTestTestComponentRemoteStateBackendWorkspaceKeyPrefix := tenant1Ue2DevTestTestComponentRemoteStateBackend["workspace_key_prefix"].(string)
	tenant1Ue2DevTestTestComponentDeps := tenant1Ue2DevTestTestComponent["deps"].([]any)
	assert.Equal(t, "test-test-component", tenant1Ue2DevTestTestComponentBackendWorkspaceKeyPrefix)
	assert.Equal(t, "test-test-component", tenant1Ue2DevTestTestComponentRemoteStateBackendWorkspaceKeyPrefix)
	assert.Equal(t, "tenant1-ue2-dev", tenant1Ue2DevTestTestComponentTerraformWorkspace)
	assert.Equal(t, 9, len(tenant1Ue2DevTestTestComponentDeps))

	// Normalize expected paths for cross-platform compatibility
	expectedPaths := []string{
		"catalog/terraform/services/service-1",
		"catalog/terraform/services/service-2",
		"catalog/terraform/spacelift-and-backend-override-1",
		"catalog/terraform/test-component",
		"mixins/region/us-east-2",
		"mixins/stage/dev",
		"orgs/cp/_defaults",
		"orgs/cp/tenant1/_defaults",
		"orgs/cp/tenant1/dev/us-east-2",
	}

	// Helper function to extract relative path
	getRelativePath := func(path string) string {
		// Split the path by common test directories
		parts := []string{
			"examples/tests/stacks/",
			"examples\\tests\\stacks\\",
		}
		
		normalizedPath := filepath.ToSlash(path)
		for _, part := range parts {
			if idx := strings.Index(normalizedPath, part); idx >= 0 {
				return normalizedPath[idx+len(part):]
			}
		}
		return normalizedPath
	}

	// Convert actual paths to forward slashes and extract relative paths for comparison
	for i, dep := range tenant1Ue2DevTestTestComponentDeps {
		actualPath := getRelativePath(dep.(string))
		assert.Equal(t, expectedPaths[i], actualPath, "Path mismatch at index %d", i)
	}

	var tenant1Ue2DevTestTestComponent2 map[string]any
	component = "test/test-component"
	tenant := "tenant1"
	environment := "ue2"
	stage := "dev"
	tenant1Ue2DevTestTestComponent2, err = ProcessComponentFromContext(component, namespace, tenant, environment, stage, "", "")
	assert.Nil(t, err)
	tenant1Ue2DevTestTestComponentBackend2 := tenant1Ue2DevTestTestComponent2["backend"].(map[string]any)
	tenant1Ue2DevTestTestComponentRemoteStateBackend2 := tenant1Ue2DevTestTestComponent2["remote_state_backend"].(map[string]any)
	tenant1Ue2DevTestTestComponentTerraformWorkspace2 := tenant1Ue2DevTestTestComponent2["workspace"].(string)
	tenant1Ue2DevTestTestComponentBackendWorkspaceKeyPrefix2 := tenant1Ue2DevTestTestComponentBackend2["workspace_key_prefix"].(string)
	tenant1Ue2DevTestTestComponentRemoteStateBackendWorkspaceKeyPrefix2 := tenant1Ue2DevTestTestComponentRemoteStateBackend2["workspace_key_prefix"].(string)
	tenant1Ue2DevTestTestComponentDeps2 := tenant1Ue2DevTestTestComponent2["deps"].([]any)
	assert.Equal(t, "test-test-component", tenant1Ue2DevTestTestComponentBackendWorkspaceKeyPrefix2)
	assert.Equal(t, "test-test-component", tenant1Ue2DevTestTestComponentRemoteStateBackendWorkspaceKeyPrefix2)
	assert.Equal(t, "tenant1-ue2-dev", tenant1Ue2DevTestTestComponentTerraformWorkspace2)
	assert.Equal(t, 9, len(tenant1Ue2DevTestTestComponentDeps2))

	// Normalize expected paths for cross-platform compatibility
	expectedPaths = []string{
		"catalog/terraform/services/service-1",
		"catalog/terraform/services/service-2",
		"catalog/terraform/spacelift-and-backend-override-1",
		"catalog/terraform/test-component",
		"mixins/region/us-east-2",
		"mixins/stage/dev",
		"orgs/cp/_defaults",
		"orgs/cp/tenant1/_defaults",
		"orgs/cp/tenant1/dev/us-east-2",
	}

	// Convert actual paths to forward slashes and extract relative paths for comparison
	for i, dep := range tenant1Ue2DevTestTestComponentDeps2 {
		actualPath := getRelativePath(dep.(string))
		assert.Equal(t, expectedPaths[i], actualPath, "Path mismatch at index %d", i)
	}

	yamlConfig, err = u.ConvertToYAML(tenant1Ue2DevTestTestComponent)
	assert.Nil(t, err)
	t.Log(yamlConfig)

	var tenant1Ue2DevTestTestComponentOverrideComponent map[string]any
	component = "test/test-component-override"
	stack = "tenant1-ue2-dev"
	tenant1Ue2DevTestTestComponentOverrideComponent, err = ProcessComponentInStack(component, stack, "", "")
	assert.Nil(t, err)
	tenant1Ue2DevTestTestComponentOverrideComponentBackend := tenant1Ue2DevTestTestComponentOverrideComponent["backend"].(map[string]any)
	tenant1Ue2DevTestTestComponentOverrideComponentBaseComponent := tenant1Ue2DevTestTestComponentOverrideComponent["component"].(string)
	tenant1Ue2DevTestTestComponentOverrideComponentWorkspace := tenant1Ue2DevTestTestComponentOverrideComponent["workspace"].(string)
	tenant1Ue2DevTestTestComponentOverrideComponentBackendWorkspaceKeyPrefix := tenant1Ue2DevTestTestComponentOverrideComponentBackend["workspace_key_prefix"].(string)
	tenant1Ue2DevTestTestComponentOverrideComponentDeps := tenant1Ue2DevTestTestComponentOverrideComponent["deps"].([]any)
	tenant1Ue2DevTestTestComponentOverrideComponentRemoteStateBackend := tenant1Ue2DevTestTestComponentOverrideComponent["remote_state_backend"].(map[string]any)
	tenant1Ue2DevTestTestComponentOverrideComponentRemoteStateBackendVal2 := tenant1Ue2DevTestTestComponentOverrideComponentRemoteStateBackend["val2"].(string)
	assert.Equal(t, "test-test-component", tenant1Ue2DevTestTestComponentOverrideComponentBackendWorkspaceKeyPrefix)
	assert.Equal(t, "test/test-component", tenant1Ue2DevTestTestComponentOverrideComponentBaseComponent)
	assert.Equal(t, "test-component-override-workspace-override", tenant1Ue2DevTestTestComponentOverrideComponentWorkspace)

	assert.Equal(t, 10, len(tenant1Ue2DevTestTestComponentOverrideComponentDeps))

	// Normalize expected paths for cross-platform compatibility
	expectedPaths = []string{
		"catalog/terraform/services/service-1-override",
		"catalog/terraform/services/service-2-override",
		"catalog/terraform/spacelift-and-backend-override-1",
		"catalog/terraform/test-component",
		"catalog/terraform/test-component-override",
		"mixins/region/us-east-2",
		"mixins/stage/dev",
		"orgs/cp/_defaults",
		"orgs/cp/tenant1/_defaults",
		"orgs/cp/tenant1/dev/us-east-2",
	}

	// Convert actual paths to forward slashes and extract relative paths for comparison
	for i, dep := range tenant1Ue2DevTestTestComponentOverrideComponentDeps {
		actualPath := getRelativePath(dep.(string))
		assert.Equal(t, expectedPaths[i], actualPath, "Path mismatch at index %d", i)
	}

	assert.Equal(t, "2", tenant1Ue2DevTestTestComponentOverrideComponentRemoteStateBackendVal2)

	var tenant1Ue2DevTestTestComponentOverrideComponent2 map[string]any
	component = "test/test-component-override-2"
	tenant = "tenant1"
	environment = "ue2"
	stage = "dev"
	tenant1Ue2DevTestTestComponentOverrideComponent2, err = ProcessComponentFromContext(component, namespace, tenant, environment, stage, "", "")
	assert.Nil(t, err)
	tenant1Ue2DevTestTestComponentOverrideComponent2Backend := tenant1Ue2DevTestTestComponentOverrideComponent2["backend"].(map[string]any)
	tenant1Ue2DevTestTestComponentOverrideComponent2Workspace := tenant1Ue2DevTestTestComponentOverrideComponent2["workspace"].(string)
	tenant1Ue2DevTestTestComponentOverrideComponent2WorkspaceKeyPrefix := tenant1Ue2DevTestTestComponentOverrideComponent2Backend["workspace_key_prefix"].(string)
	assert.Equal(t, "tenant1-ue2-dev-test-test-component-override-2", tenant1Ue2DevTestTestComponentOverrideComponent2Workspace)
	assert.Equal(t, "test-test-component", tenant1Ue2DevTestTestComponentOverrideComponent2WorkspaceKeyPrefix)

	yamlConfig, err = u.ConvertToYAML(tenant1Ue2DevTestTestComponentOverrideComponent2)
	assert.Nil(t, err)
	t.Log(yamlConfig)

	// Test having a dash `-` in the stage name
	var tenant1Ue2Test1TestTestComponentOverrideComponent2 map[string]any
	component = "test/test-component-override-2"
	tenant = "tenant1"
	environment = "ue2"
	stage = "test-1"
	tenant1Ue2Test1TestTestComponentOverrideComponent2, err = ProcessComponentFromContext(component, namespace, tenant, environment, stage, "", "")
	assert.Nil(t, err)
	tenant1Ue2Test1TestTestComponentOverrideComponent2Backend := tenant1Ue2DevTestTestComponentOverrideComponent2["backend"].(map[string]any)
	tenant1Ue2Test1TestTestComponentOverrideComponent2Workspace := tenant1Ue2Test1TestTestComponentOverrideComponent2["workspace"].(string)
	tenant1Ue2Test1TestTestComponentOverrideComponent2WorkspaceKeyPrefix := tenant1Ue2Test1TestTestComponentOverrideComponent2Backend["workspace_key_prefix"].(string)
	assert.Equal(t, "tenant1-ue2-test-1-test-test-component-override-2", tenant1Ue2Test1TestTestComponentOverrideComponent2Workspace)
	assert.Equal(t, "test-test-component", tenant1Ue2Test1TestTestComponentOverrideComponent2WorkspaceKeyPrefix)

	var tenant1Ue2DevTestTestComponentOverrideComponent3 map[string]any
	component = "test/test-component-override-3"
	stack = "tenant1-ue2-dev"
	tenant1Ue2DevTestTestComponentOverrideComponent3, err = ProcessComponentInStack(component, stack, "", "")
	assert.Nil(t, err)

	tenant1Ue2DevTestTestComponentOverrideComponent3Deps := tenant1Ue2DevTestTestComponentOverrideComponent3["deps"].([]any)

	assert.Equal(t, 11, len(tenant1Ue2DevTestTestComponentOverrideComponent3Deps))

	// Normalize expected paths for cross-platform compatibility
	expectedPaths = []string{
		"catalog/terraform/mixins/test-2",
		"catalog/terraform/services/service-1-override-2",
		"catalog/terraform/services/service-2-override-2",
		"catalog/terraform/spacelift-and-backend-override-1",
		"catalog/terraform/test-component",
		"catalog/terraform/test-component-override-3",
		"mixins/region/us-east-2",
		"mixins/stage/dev",
		"orgs/cp/_defaults",
		"orgs/cp/tenant1/_defaults",
		"orgs/cp/tenant1/dev/us-east-2",
	}

	// Convert actual paths to forward slashes and extract relative paths for comparison
	for i, dep := range tenant1Ue2DevTestTestComponentOverrideComponent3Deps {
		actualPath := getRelativePath(dep.(string))
		assert.Equal(t, expectedPaths[i], actualPath, "Path mismatch at index %d", i)
	}
}

func TestComponentProcessorHierarchicalInheritance(t *testing.T) {
	var yamlConfig string
	namespace := ""
	component := "derived-component-2"
	tenant := "tenant1"
	environment := "ue2"
	stage := "test-1"

	componentMap, err := ProcessComponentFromContext(component, namespace, tenant, environment, stage, "", "")
	assert.Nil(t, err)

	componentVars := componentMap["vars"].(map[string]any)
	componentHierarchicalInheritanceTestVar := componentVars["hierarchical_inheritance_test"].(string)
	assert.Equal(t, "base-component-1", componentHierarchicalInheritanceTestVar)

	yamlConfig, err = u.ConvertToYAML(componentMap)
	assert.Nil(t, err)
	t.Log(yamlConfig)
}
