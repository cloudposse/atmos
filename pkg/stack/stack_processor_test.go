package stack

import (
	c "github.com/cloudposse/atmos/pkg/convert"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
	"testing"
)

func TestStackProcessor(t *testing.T) {
	basePath := "../../examples/complete/stacks"

	filePaths := []string{
		"../../examples/complete/stacks/tenant1/ue2/dev.yaml",
		"../../examples/complete/stacks/tenant1/ue2/prod.yaml",
		"../../examples/complete/stacks/tenant1/ue2/staging.yaml",
	}

	processStackDeps := true
	processComponentDeps := true

	var listResult, mapResult, err = ProcessYAMLConfigFiles(basePath, filePaths, processStackDeps, processComponentDeps)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(listResult))
	assert.Equal(t, 3, len(mapResult))

	mapResultKeys := u.StringKeysFromMap(mapResult)
	assert.Equal(t, "tenant1/ue2/dev", mapResultKeys[0])
	assert.Equal(t, "tenant1/ue2/prod", mapResultKeys[1])
	assert.Equal(t, "tenant1/ue2/staging", mapResultKeys[2])

	mapConfig1, err := c.YAMLToMapOfInterfaces(listResult[0])
	assert.Nil(t, err)

	imports := mapConfig1["imports"].([]interface{})

	mapConfig2 := mapResult["tenant1/ue2/dev"]
	assert.Equal(t, len(imports), len(mapConfig2.(map[interface{}]interface{})["imports"].([]string)))

	assert.Equal(t, 23, len(imports))
	assert.Equal(t, "catalog/helmfile/echo-server", imports[0])
	assert.Equal(t, "catalog/helmfile/infra-server", imports[1])
	assert.Equal(t, "catalog/helmfile/infra-server-override", imports[2])
	assert.Equal(t, "catalog/terraform/mixins/test-1", imports[3])
	assert.Equal(t, "catalog/terraform/mixins/test-2", imports[4])
	assert.Equal(t, "catalog/terraform/services/service-1", imports[5])
	assert.Equal(t, "catalog/terraform/services/service-1-override", imports[6])
	assert.Equal(t, "catalog/terraform/services/service-1-override-2", imports[7])
	assert.Equal(t, "catalog/terraform/services/service-2", imports[8])
	assert.Equal(t, "catalog/terraform/services/service-2-override", imports[9])
	assert.Equal(t, "catalog/terraform/services/service-2-override-2", imports[10])
	assert.Equal(t, "catalog/terraform/services/top-level-service-1", imports[11])
	assert.Equal(t, "catalog/terraform/services/top-level-service-2", imports[12])
	assert.Equal(t, "catalog/terraform/tenant1-ue2-dev", imports[13])
	assert.Equal(t, "catalog/terraform/test-component", imports[14])
	assert.Equal(t, "catalog/terraform/test-component-override", imports[15])
	assert.Equal(t, "catalog/terraform/test-component-override-2", imports[16])
	assert.Equal(t, "catalog/terraform/test-component-override-3", imports[17])
	assert.Equal(t, "catalog/terraform/top-level-component1", imports[18])
	assert.Equal(t, "catalog/terraform/vpc", imports[19])
	assert.Equal(t, "globals/globals", imports[20])
	assert.Equal(t, "globals/tenant1-globals", imports[21])
	assert.Equal(t, "globals/ue2-globals", imports[22])

	components := mapConfig1["components"].(map[interface{}]interface{})
	terraformComponents := components["terraform"].(map[interface{}]interface{})
	helmfileComponents := components["helmfile"].(map[interface{}]interface{})

	infraVpcComponent := terraformComponents["infra/vpc"].(map[interface{}]interface{})
	infraVpcComponentBackend := infraVpcComponent["backend"].(map[interface{}]interface{})
	infraVpcComponentBackendType := infraVpcComponent["backend_type"]
	infraVpcComponentRemoteSateBackend := infraVpcComponent["remote_state_backend"].(map[interface{}]interface{})
	infraVpcComponentRemoteSateBackendType := infraVpcComponent["remote_state_backend_type"]
	infraVpcComponentBackendWorkspaceKeyPrefix := infraVpcComponentBackend["workspace_key_prefix"]
	infraVpcComponentRemoteStateBackendWorkspaceKeyPrefix := infraVpcComponentRemoteSateBackend["workspace_key_prefix"]
	assert.Equal(t, "infra-vpc", infraVpcComponentBackendWorkspaceKeyPrefix)
	assert.Equal(t, "infra-vpc", infraVpcComponentRemoteStateBackendWorkspaceKeyPrefix)
	assert.Equal(t, "s3", infraVpcComponentBackendType)
	assert.Equal(t, "s3", infraVpcComponentRemoteSateBackendType)

	testTestComponent := terraformComponents["test/test-component"].(map[interface{}]interface{})
	testTestComponentBackend := testTestComponent["backend"].(map[interface{}]interface{})
	testTestComponentBackendType := testTestComponent["backend_type"]
	testTestComponentBackendBucket := testTestComponentBackend["bucket"]
	testTestComponentBackendWorkspaceKeyPrefix := testTestComponentBackend["workspace_key_prefix"]
	testTestComponentBackendRoleArn := testTestComponentBackend["role_arn"]
	testTestComponentRemoteStateBackend := testTestComponent["remote_state_backend"].(map[interface{}]interface{})
	testTestComponentRemoteStateBackendType := testTestComponent["remote_state_backend_type"]
	testTestComponentRemoteStateBackendBucket := testTestComponentRemoteStateBackend["bucket"]
	testTestComponentRemoteStateBackendWorkspaceKeyPrefix := testTestComponentRemoteStateBackend["workspace_key_prefix"]
	testTestComponentRemoteStateBackendRoleArn := testTestComponentRemoteStateBackend["role_arn"]
	assert.Equal(t, "eg-ue2-root-tfstate", testTestComponentBackendBucket)
	assert.Equal(t, "eg-ue2-root-tfstate", testTestComponentRemoteStateBackendBucket)
	assert.Equal(t, "test-test-component", testTestComponentBackendWorkspaceKeyPrefix)
	assert.Equal(t, "test-test-component", testTestComponentRemoteStateBackendWorkspaceKeyPrefix)
	assert.Equal(t, "s3", testTestComponentBackendType)
	assert.Equal(t, "s3", testTestComponentRemoteStateBackendType)
	assert.Equal(t, nil, testTestComponentBackendRoleArn)
	assert.Equal(t, "arn:aws:iam::123456789012:role/eg-gbl-root-terraform", testTestComponentRemoteStateBackendRoleArn)

	testTestComponentOverrideComponent := terraformComponents["test/test-component-override"].(map[interface{}]interface{})
	testTestComponentOverrideComponentBackend := testTestComponentOverrideComponent["backend"].(map[interface{}]interface{})
	testTestComponentOverrideComponentBackendType := testTestComponentOverrideComponent["backend_type"]
	testTestComponentOverrideComponentBackendWorkspaceKeyPrefix := testTestComponentOverrideComponentBackend["workspace_key_prefix"]
	testTestComponentOverrideComponentBackendBucket := testTestComponentOverrideComponentBackend["bucket"]
	testTestComponentOverrideComponentRemoteStateBackend := testTestComponentOverrideComponent["remote_state_backend"].(map[interface{}]interface{})
	testTestComponentOverrideComponentRemoteStateBackendVal1 := testTestComponentOverrideComponentRemoteStateBackend["val1"].(bool)
	testTestComponentOverrideComponentRemoteStateBackendVal2 := testTestComponentOverrideComponentRemoteStateBackend["val2"]
	testTestComponentOverrideComponentRemoteStateBackendVal3 := testTestComponentOverrideComponentRemoteStateBackend["val3"].(int)
	testTestComponentOverrideComponentRemoteStateBackendVal4 := testTestComponentOverrideComponentRemoteStateBackend["val4"]
	testTestComponentOverrideComponentRemoteStateBackendType := testTestComponentOverrideComponent["remote_state_backend_type"]
	testTestComponentOverrideComponentBaseComponent := testTestComponentOverrideComponent["component"]
	assert.Equal(t, "test-test-component", testTestComponentOverrideComponentBackendWorkspaceKeyPrefix)
	assert.Equal(t, "eg-ue2-root-tfstate", testTestComponentOverrideComponentBackendBucket)
	assert.Equal(t, "test/test-component", testTestComponentOverrideComponentBaseComponent)
	assert.Equal(t, "s3", testTestComponentOverrideComponentBackendType)
	assert.Equal(t, "static", testTestComponentOverrideComponentRemoteStateBackendType)
	assert.Equal(t, true, testTestComponentOverrideComponentRemoteStateBackendVal1)
	assert.Equal(t, "2", testTestComponentOverrideComponentRemoteStateBackendVal2)
	assert.Equal(t, 3, testTestComponentOverrideComponentRemoteStateBackendVal3)
	assert.Equal(t, nil, testTestComponentOverrideComponentRemoteStateBackendVal4)

	topLevelComponent1 := terraformComponents["top-level-component1"].(map[interface{}]interface{})
	topLevelComponent1Backend := topLevelComponent1["backend"].(map[interface{}]interface{})
	topLevelComponent1RemoteSateBackend := topLevelComponent1["remote_state_backend"].(map[interface{}]interface{})
	topLevelComponent1BackendWorkspaceKeyPrefix := topLevelComponent1Backend["workspace_key_prefix"]
	topLevelComponent1RemoteStateBackendWorkspaceKeyPrefix := topLevelComponent1RemoteSateBackend["workspace_key_prefix"]
	assert.Equal(t, "top-level-component1", topLevelComponent1BackendWorkspaceKeyPrefix)
	assert.Equal(t, "top-level-component1", topLevelComponent1RemoteStateBackendWorkspaceKeyPrefix)

	testTestComponentOverrideComponent2 := terraformComponents["test/test-component-override-2"].(map[interface{}]interface{})
	testTestComponentOverrideComponentBackend2 := testTestComponentOverrideComponent2["backend"].(map[interface{}]interface{})
	testTestComponentOverrideComponentBackendType2 := testTestComponentOverrideComponent2["backend_type"]
	testTestComponentOverrideComponentBackendWorkspaceKeyPrefix2 := testTestComponentOverrideComponentBackend2["workspace_key_prefix"]
	testTestComponentOverrideComponentBackendBucket2 := testTestComponentOverrideComponentBackend2["bucket"]
	testTestComponentOverrideComponentBaseComponent2 := testTestComponentOverrideComponent2["component"]
	testTestComponentOverrideInheritance2 := testTestComponentOverrideComponent2["inheritance"].([]interface{})
	assert.Equal(t, "test-test-component", testTestComponentOverrideComponentBackendWorkspaceKeyPrefix2)
	assert.Equal(t, "eg-ue2-root-tfstate", testTestComponentOverrideComponentBackendBucket2)
	assert.Equal(t, "test/test-component", testTestComponentOverrideComponentBaseComponent2)
	assert.Equal(t, "s3", testTestComponentOverrideComponentBackendType2)
	assert.Equal(t, "test/test-component-override", testTestComponentOverrideInheritance2[0])
	assert.Equal(t, "test/test-component", testTestComponentOverrideInheritance2[1])

	infraInfraServerOverrideComponent := helmfileComponents["infra/infra-server-override"].(map[interface{}]interface{})
	infraInfraServerOverrideComponentCommand := infraInfraServerOverrideComponent["command"]
	infraInfraServerOverrideComponentDeps := infraInfraServerOverrideComponent["deps"].([]interface{})
	infraInfraServerOverrideComponentVars := infraInfraServerOverrideComponent["vars"].(map[interface{}]interface{})
	infraInfraServerOverrideComponentVarsA := infraInfraServerOverrideComponentVars["a"]
	infraInfraServerOverrideComponentInheritance := infraInfraServerOverrideComponent["inheritance"].([]interface{})
	assert.Equal(t, "helmfile", infraInfraServerOverrideComponentCommand)
	assert.Equal(t, "catalog/helmfile/infra-server", infraInfraServerOverrideComponentDeps[0])
	assert.Equal(t, "catalog/helmfile/infra-server-override", infraInfraServerOverrideComponentDeps[1])
	assert.Equal(t, "globals/globals", infraInfraServerOverrideComponentDeps[2])
	assert.Equal(t, "globals/tenant1-globals", infraInfraServerOverrideComponentDeps[3])
	assert.Equal(t, "globals/ue2-globals", infraInfraServerOverrideComponentDeps[4])
	assert.Equal(t, "tenant1/ue2/dev", infraInfraServerOverrideComponentDeps[5])
	assert.Equal(t, "infra/infra-server", infraInfraServerOverrideComponentInheritance[0])
	assert.Equal(t, "1_override", infraInfraServerOverrideComponentVarsA)

	yamlConfig, err := yaml.Marshal(mapConfig1)
	assert.Nil(t, err)
	t.Log(string(yamlConfig))
}
