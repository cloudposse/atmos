package component

import (
	"testing"

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
	tenant1Ue2DevTestTestComponentBaseComponent := tenant1Ue2DevTestTestComponent["component"]
	tenant1Ue2DevTestTestComponentTerraformWorkspace := tenant1Ue2DevTestTestComponent["workspace"].(string)
	tenant1Ue2DevTestTestComponentWorkspace := tenant1Ue2DevTestTestComponent["workspace"].(string)
	tenant1Ue2DevTestTestComponentBackendWorkspaceKeyPrefix := tenant1Ue2DevTestTestComponentBackend["workspace_key_prefix"].(string)
	tenant1Ue2DevTestTestComponentRemoteStateBackendWorkspaceKeyPrefix := tenant1Ue2DevTestTestComponentRemoteStateBackend["workspace_key_prefix"].(string)
	tenant1Ue2DevTestTestComponentDeps := tenant1Ue2DevTestTestComponent["deps"].([]any)
	assert.Equal(t, "test-test-component", tenant1Ue2DevTestTestComponentBackendWorkspaceKeyPrefix)
	assert.Equal(t, "test-test-component", tenant1Ue2DevTestTestComponentRemoteStateBackendWorkspaceKeyPrefix)
	assert.Equal(t, "test/test-component", tenant1Ue2DevTestTestComponentBaseComponent)
	assert.Equal(t, "tenant1-ue2-dev", tenant1Ue2DevTestTestComponentWorkspace)
	assert.Equal(t, 9, len(tenant1Ue2DevTestTestComponentDeps))
	assert.Equal(t, "catalog/terraform/services/service-1", tenant1Ue2DevTestTestComponentDeps[0])
	assert.Equal(t, "catalog/terraform/services/service-2", tenant1Ue2DevTestTestComponentDeps[1])
	assert.Equal(t, "catalog/terraform/spacelift-and-backend-override-1", tenant1Ue2DevTestTestComponentDeps[2])
	assert.Equal(t, "catalog/terraform/test-component", tenant1Ue2DevTestTestComponentDeps[3])
	assert.Equal(t, "mixins/region/us-east-2", tenant1Ue2DevTestTestComponentDeps[4])
	assert.Equal(t, "mixins/stage/dev", tenant1Ue2DevTestTestComponentDeps[5])
	assert.Equal(t, "orgs/cp/_defaults", tenant1Ue2DevTestTestComponentDeps[6])
	assert.Equal(t, "orgs/cp/tenant1/_defaults", tenant1Ue2DevTestTestComponentDeps[7])
	assert.Equal(t, "orgs/cp/tenant1/dev/us-east-2", tenant1Ue2DevTestTestComponentDeps[8])
	assert.Equal(t, "tenant1-ue2-dev", tenant1Ue2DevTestTestComponentTerraformWorkspace)

	var tenant1Ue2DevTestTestComponent2 map[string]any
	component = "test/test-component"
	tenant := "tenant1"
	environment := "ue2"
	stage := "dev"
	tenant1Ue2DevTestTestComponent2, err = ProcessComponentFromContext(component, namespace, tenant, environment, stage, "", "")
	assert.Nil(t, err)
	tenant1Ue2DevTestTestComponentBackend2 := tenant1Ue2DevTestTestComponent2["backend"].(map[string]any)
	tenant1Ue2DevTestTestComponentRemoteStateBackend2 := tenant1Ue2DevTestTestComponent2["remote_state_backend"].(map[string]any)
	tenant1Ue2DevTestTestComponentBaseComponent2 := tenant1Ue2DevTestTestComponent2["component"]
	tenant1Ue2DevTestTestComponentTerraformWorkspace2 := tenant1Ue2DevTestTestComponent2["workspace"].(string)
	tenant1Ue2DevTestTestComponentWorkspace2 := tenant1Ue2DevTestTestComponent2["workspace"].(string)
	tenant1Ue2DevTestTestComponentBackendWorkspaceKeyPrefix2 := tenant1Ue2DevTestTestComponentBackend2["workspace_key_prefix"].(string)
	tenant1Ue2DevTestTestComponentRemoteStateBackendWorkspaceKeyPrefix2 := tenant1Ue2DevTestTestComponentRemoteStateBackend2["workspace_key_prefix"].(string)
	tenant1Ue2DevTestTestComponentDeps2 := tenant1Ue2DevTestTestComponent2["deps"].([]any)
	assert.Equal(t, "test-test-component", tenant1Ue2DevTestTestComponentBackendWorkspaceKeyPrefix2)
	assert.Equal(t, "test-test-component", tenant1Ue2DevTestTestComponentRemoteStateBackendWorkspaceKeyPrefix2)
	assert.Equal(t, "test/test-component", tenant1Ue2DevTestTestComponentBaseComponent2)
	assert.Equal(t, "tenant1-ue2-dev", tenant1Ue2DevTestTestComponentWorkspace2)
	assert.Equal(t, 9, len(tenant1Ue2DevTestTestComponentDeps2))
	assert.Equal(t, "catalog/terraform/services/service-1", tenant1Ue2DevTestTestComponentDeps2[0])
	assert.Equal(t, "catalog/terraform/services/service-2", tenant1Ue2DevTestTestComponentDeps2[1])
	assert.Equal(t, "catalog/terraform/spacelift-and-backend-override-1", tenant1Ue2DevTestTestComponentDeps2[2])
	assert.Equal(t, "catalog/terraform/test-component", tenant1Ue2DevTestTestComponentDeps2[3])
	assert.Equal(t, "mixins/region/us-east-2", tenant1Ue2DevTestTestComponentDeps2[4])
	assert.Equal(t, "mixins/stage/dev", tenant1Ue2DevTestTestComponentDeps2[5])
	assert.Equal(t, "orgs/cp/_defaults", tenant1Ue2DevTestTestComponentDeps2[6])
	assert.Equal(t, "orgs/cp/tenant1/_defaults", tenant1Ue2DevTestTestComponentDeps2[7])
	assert.Equal(t, "orgs/cp/tenant1/dev/us-east-2", tenant1Ue2DevTestTestComponentDeps2[8])
	assert.Equal(t, "tenant1-ue2-dev", tenant1Ue2DevTestTestComponentTerraformWorkspace2)

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
	assert.Equal(t, "catalog/terraform/services/service-1-override", tenant1Ue2DevTestTestComponentOverrideComponentDeps[0])
	assert.Equal(t, "catalog/terraform/services/service-2-override", tenant1Ue2DevTestTestComponentOverrideComponentDeps[1])
	assert.Equal(t, "catalog/terraform/spacelift-and-backend-override-1", tenant1Ue2DevTestTestComponentOverrideComponentDeps[2])
	assert.Equal(t, "catalog/terraform/test-component", tenant1Ue2DevTestTestComponentOverrideComponentDeps[3])
	assert.Equal(t, "catalog/terraform/test-component-override", tenant1Ue2DevTestTestComponentOverrideComponentDeps[4])
	assert.Equal(t, "mixins/region/us-east-2", tenant1Ue2DevTestTestComponentOverrideComponentDeps[5])
	assert.Equal(t, "mixins/stage/dev", tenant1Ue2DevTestTestComponentOverrideComponentDeps[6])
	assert.Equal(t, "orgs/cp/_defaults", tenant1Ue2DevTestTestComponentOverrideComponentDeps[7])
	assert.Equal(t, "orgs/cp/tenant1/_defaults", tenant1Ue2DevTestTestComponentOverrideComponentDeps[8])
	assert.Equal(t, "orgs/cp/tenant1/dev/us-east-2", tenant1Ue2DevTestTestComponentOverrideComponentDeps[9])

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
	assert.Equal(t, "catalog/terraform/mixins/test-2", tenant1Ue2DevTestTestComponentOverrideComponent3Deps[0])
	assert.Equal(t, "catalog/terraform/services/service-1-override-2", tenant1Ue2DevTestTestComponentOverrideComponent3Deps[1])
	assert.Equal(t, "catalog/terraform/services/service-2-override-2", tenant1Ue2DevTestTestComponentOverrideComponent3Deps[2])
	assert.Equal(t, "catalog/terraform/spacelift-and-backend-override-1", tenant1Ue2DevTestTestComponentOverrideComponent3Deps[3])
	assert.Equal(t, "catalog/terraform/test-component", tenant1Ue2DevTestTestComponentOverrideComponent3Deps[4])
	assert.Equal(t, "catalog/terraform/test-component-override-3", tenant1Ue2DevTestTestComponentOverrideComponent3Deps[5])
	assert.Equal(t, "mixins/region/us-east-2", tenant1Ue2DevTestTestComponentOverrideComponent3Deps[6])
	assert.Equal(t, "mixins/stage/dev", tenant1Ue2DevTestTestComponentOverrideComponent3Deps[7])
	assert.Equal(t, "orgs/cp/_defaults", tenant1Ue2DevTestTestComponentOverrideComponent3Deps[8])
	assert.Equal(t, "orgs/cp/tenant1/_defaults", tenant1Ue2DevTestTestComponentOverrideComponent3Deps[9])
	assert.Equal(t, "orgs/cp/tenant1/dev/us-east-2", tenant1Ue2DevTestTestComponentOverrideComponent3Deps[10])
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

func TestComponentProcessor_StackNameTemplate(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/stack-name-template"
	component := "c1"
	namespace := "acme"
	tenant := "plat"
	environment := "ue2"
	stage := "dev"

	componentMap, err := ProcessComponentFromContext(component, namespace, tenant, environment, stage, stacksPath, stacksPath)
	assert.Nil(t, err)

	componentVars := componentMap["vars"].(map[string]any)
	assert.Equal(t, "a", componentVars["a"].(string))
	assert.Equal(t, "b", componentVars["b"].(string))
	assert.Equal(t, namespace, componentVars["namespace"].(string))
	assert.Equal(t, tenant, componentVars["tenant"].(string))
	assert.Equal(t, environment, componentVars["environment"].(string))
	assert.Equal(t, stage, componentVars["stage"].(string))
}

func TestComponentProcessor_StackNameTemplate_Errors(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/stack-name-template"
	component := "c1"
	namespace := ""
	tenant := "plat"
	environment := "ue2"
	stage := "dev"

	_, err := ProcessComponentFromContext(component, namespace, tenant, environment, stage, stacksPath, stacksPath)
	assert.ErrorContains(t, err, "'namespace' is required")

	namespace = "acme"
	tenant = ""
	_, err = ProcessComponentFromContext(component, namespace, tenant, environment, stage, stacksPath, stacksPath)
	assert.ErrorContains(t, err, "'environment' requires 'tenant' and 'namespace'")

	t.Setenv("ATMOS_STACKS_NAME_TEMPLATE", "{{ .invalid }}")

	_, err = ProcessComponentFromContext(component, namespace, tenant, environment, stage, stacksPath, stacksPath)
	assert.ErrorContains(t, err, "map has no entry for key \"invalid\"")
}

func TestComponentProcessor_Helmfile(t *testing.T) {
	var err error
	var component string
	var stack string

	var componentMap map[string]any
	component = "echo-server"
	stack = "tenant1-ue2-dev"

	componentMap, err = ProcessComponentInStack(component, stack, "", "")
	assert.Nil(t, err)
	componentVars := componentMap["vars"].(map[string]any)
	installed := componentVars["installed"].(bool)
	assert.Equal(t, true, installed)
}

func TestComponentProcessor_Packer(t *testing.T) {
	var err error
	var component string
	var stack string

	var componentMap map[string]any
	component = "aws/bastion"
	stack = "tenant1-ue2-dev"

	componentMap, err = ProcessComponentInStack(component, stack, "", "")
	assert.Nil(t, err)
	componentVars := componentMap["vars"].(map[string]any)
	sourceAmi := componentVars["source_ami"].(string)
	assert.Equal(t, "ami-0013ceeff668b979b", sourceAmi)
	region := componentVars["region"].(string)
	assert.Equal(t, "us-east-2", region)
}
