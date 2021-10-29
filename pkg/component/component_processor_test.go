package component

import (
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
	"testing"
)

func TestComponentProcessor(t *testing.T) {
	var err error
	var component string
	var stack string
	var yamlConfig []byte

	var tenant1Ue2DevTestTestComponent map[string]interface{}
	component = "test/test-component"
	stack = "tenant1-ue2-dev"
	tenant1Ue2DevTestTestComponent, err = ProcessComponentInStack(component, stack)
	assert.Nil(t, err)
	tenant1Ue2DevTestTestComponentBackend := tenant1Ue2DevTestTestComponent["backend"].(map[interface{}]interface{})
	tenant1Ue2DevTestTestComponentBaseComponent := tenant1Ue2DevTestTestComponent["component"]
	tenant1Ue2DevTestTestComponentWorkspace := tenant1Ue2DevTestTestComponent["workspace"].(string)
	tenant1Ue2DevTestTestComponentBackendWorkspaceKeyPrefix := tenant1Ue2DevTestTestComponentBackend["workspace_key_prefix"].(string)
	tenant1Ue2DevTestTestComponentDeps := tenant1Ue2DevTestTestComponent["deps"].([]string)
	assert.Equal(t, "test-test-component", tenant1Ue2DevTestTestComponentBackendWorkspaceKeyPrefix)
	assert.Nil(t, tenant1Ue2DevTestTestComponentBaseComponent)
	assert.Equal(t, "tenant1-ue2-dev", tenant1Ue2DevTestTestComponentWorkspace)
	assert.Equal(t, 7, len(tenant1Ue2DevTestTestComponentDeps))
	assert.Equal(t, "catalog/terraform/services/service-1", tenant1Ue2DevTestTestComponentDeps[0])
	assert.Equal(t, "catalog/terraform/services/service-2", tenant1Ue2DevTestTestComponentDeps[1])
	assert.Equal(t, "catalog/terraform/test-component", tenant1Ue2DevTestTestComponentDeps[2])
	assert.Equal(t, "globals/globals", tenant1Ue2DevTestTestComponentDeps[3])
	assert.Equal(t, "globals/tenant1-globals", tenant1Ue2DevTestTestComponentDeps[4])
	assert.Equal(t, "globals/ue2-globals", tenant1Ue2DevTestTestComponentDeps[5])
	assert.Equal(t, "tenant1/ue2/dev", tenant1Ue2DevTestTestComponentDeps[6])

	var tenant1Ue2DevTestTestComponent2 map[string]interface{}
	component = "test/test-component"
	tenant := "tenant1"
	environment := "ue2"
	stage := "dev"
	tenant1Ue2DevTestTestComponent2, err = ProcessComponentFromContext(component, tenant, environment, stage)
	assert.Nil(t, err)
	tenant1Ue2DevTestTestComponentBackend2 := tenant1Ue2DevTestTestComponent2["backend"].(map[interface{}]interface{})
	tenant1Ue2DevTestTestComponentBaseComponent2 := tenant1Ue2DevTestTestComponent2["component"]
	tenant1Ue2DevTestTestComponentWorkspace2 := tenant1Ue2DevTestTestComponent2["workspace"].(string)
	tenant1Ue2DevTestTestComponentBackendWorkspaceKeyPrefix2 := tenant1Ue2DevTestTestComponentBackend2["workspace_key_prefix"].(string)
	tenant1Ue2DevTestTestComponentDeps2 := tenant1Ue2DevTestTestComponent2["deps"].([]string)
	assert.Equal(t, "test-test-component", tenant1Ue2DevTestTestComponentBackendWorkspaceKeyPrefix2)
	assert.Nil(t, tenant1Ue2DevTestTestComponentBaseComponent2)
	assert.Equal(t, "tenant1-ue2-dev", tenant1Ue2DevTestTestComponentWorkspace2)
	assert.Equal(t, 7, len(tenant1Ue2DevTestTestComponentDeps2))
	assert.Equal(t, "catalog/terraform/services/service-1", tenant1Ue2DevTestTestComponentDeps2[0])
	assert.Equal(t, "catalog/terraform/services/service-2", tenant1Ue2DevTestTestComponentDeps2[1])
	assert.Equal(t, "catalog/terraform/test-component", tenant1Ue2DevTestTestComponentDeps2[2])
	assert.Equal(t, "globals/globals", tenant1Ue2DevTestTestComponentDeps2[3])
	assert.Equal(t, "globals/tenant1-globals", tenant1Ue2DevTestTestComponentDeps2[4])
	assert.Equal(t, "globals/ue2-globals", tenant1Ue2DevTestTestComponentDeps2[5])
	assert.Equal(t, "tenant1/ue2/dev", tenant1Ue2DevTestTestComponentDeps2[6])

	yamlConfig, err = yaml.Marshal(tenant1Ue2DevTestTestComponent)
	assert.Nil(t, err)
	t.Log(string(yamlConfig))

	var tenant1Ue2DevTestTestComponentOverrideComponent map[string]interface{}
	component = "test/test-component-override"
	stack = "tenant1-ue2-dev"
	tenant1Ue2DevTestTestComponentOverrideComponent, err = ProcessComponentInStack(component, stack)
	assert.Nil(t, err)
	tenant1Ue2DevTestTestComponentOverrideComponentBackend := tenant1Ue2DevTestTestComponentOverrideComponent["backend"].(map[interface{}]interface{})
	tenant1Ue2DevTestTestComponentOverrideComponentBaseComponent := tenant1Ue2DevTestTestComponentOverrideComponent["component"].(string)
	tenant1Ue2DevTestTestComponentOverrideComponentWorkspace := tenant1Ue2DevTestTestComponentOverrideComponent["workspace"].(string)
	tenant1Ue2DevTestTestComponentOverrideComponentBackendWorkspaceKeyPrefix := tenant1Ue2DevTestTestComponentOverrideComponentBackend["workspace_key_prefix"].(string)
	tenant1Ue2DevTestTestComponentOverrideComponentDeps := tenant1Ue2DevTestTestComponentOverrideComponent["deps"].([]string)
	assert.Equal(t, "test-test-component", tenant1Ue2DevTestTestComponentOverrideComponentBackendWorkspaceKeyPrefix)
	assert.Equal(t, "test/test-component", tenant1Ue2DevTestTestComponentOverrideComponentBaseComponent)
	assert.Equal(t, "tenant1-ue2-dev-test-test-component-override", tenant1Ue2DevTestTestComponentOverrideComponentWorkspace)
	assert.Equal(t, 11, len(tenant1Ue2DevTestTestComponentOverrideComponentDeps))
	assert.Equal(t, "catalog/terraform/services/service-1", tenant1Ue2DevTestTestComponentOverrideComponentDeps[0])
	assert.Equal(t, "catalog/terraform/services/service-1-override", tenant1Ue2DevTestTestComponentOverrideComponentDeps[1])
	assert.Equal(t, "catalog/terraform/services/service-2", tenant1Ue2DevTestTestComponentOverrideComponentDeps[2])
	assert.Equal(t, "catalog/terraform/services/service-2-override", tenant1Ue2DevTestTestComponentOverrideComponentDeps[3])
	assert.Equal(t, "catalog/terraform/tenant1-ue2-dev", tenant1Ue2DevTestTestComponentOverrideComponentDeps[4])
	assert.Equal(t, "catalog/terraform/test-component", tenant1Ue2DevTestTestComponentOverrideComponentDeps[5])
	assert.Equal(t, "catalog/terraform/test-component-override", tenant1Ue2DevTestTestComponentOverrideComponentDeps[6])
	assert.Equal(t, "globals/globals", tenant1Ue2DevTestTestComponentOverrideComponentDeps[7])
	assert.Equal(t, "globals/tenant1-globals", tenant1Ue2DevTestTestComponentOverrideComponentDeps[8])
	assert.Equal(t, "globals/ue2-globals", tenant1Ue2DevTestTestComponentOverrideComponentDeps[9])
	assert.Equal(t, "tenant1/ue2/dev", tenant1Ue2DevTestTestComponentOverrideComponentDeps[10])

	yamlConfig, err = yaml.Marshal(tenant1Ue2DevTestTestComponentOverrideComponent)
	assert.Nil(t, err)
	t.Log(string(yamlConfig))
}
