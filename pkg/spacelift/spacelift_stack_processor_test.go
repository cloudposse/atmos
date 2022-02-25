package spacelift

import (
	"gopkg.in/yaml.v2"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSpaceliftStackProcessor(t *testing.T) {
	processStackDeps := true
	processComponentDeps := true
	processImports := true
	stackConfigPathTemplate := "stacks/%s.yaml"

	var spaceliftStacks, err = CreateSpaceliftStacks("", nil, processStackDeps, processComponentDeps, processImports, stackConfigPathTemplate)
	assert.Nil(t, err)
	assert.Equal(t, 30, len(spaceliftStacks))

	tenant1Ue2DevInfraVpcStack := spaceliftStacks["tenant1-ue2-dev-infra-vpc"].(map[string]interface{})
	tenant1Ue2DevInfraVpcStackInfrastructureStackName := tenant1Ue2DevInfraVpcStack["stack"].(string)
	tenant1Ue2DevInfraVpcStackBackend := tenant1Ue2DevInfraVpcStack["backend"].(map[interface{}]interface{})
	tenant1Ue2DevInfraVpcStackBackendWorkspaceKeyPrefix := tenant1Ue2DevInfraVpcStackBackend["workspace_key_prefix"].(string)
	assert.Equal(t, "tenant1-ue2-dev", tenant1Ue2DevInfraVpcStackInfrastructureStackName)
	assert.Equal(t, "infra-vpc", tenant1Ue2DevInfraVpcStackBackendWorkspaceKeyPrefix)

	tenant1Ue2DevTestTestComponentOverrideComponent := spaceliftStacks["tenant1-ue2-dev-test-test-component-override"].(map[string]interface{})
	tenant1Ue2DevTestTestComponentOverrideComponentInfrastructureStackName := tenant1Ue2DevTestTestComponentOverrideComponent["stack"].(string)
	tenant1Ue2DevTestTestComponentOverrideComponentBackend := tenant1Ue2DevTestTestComponentOverrideComponent["backend"].(map[interface{}]interface{})
	tenant1Ue2DevTestTestComponentOverrideComponentBaseComponent := tenant1Ue2DevTestTestComponentOverrideComponent["base_component"].(string)
	tenant1Ue2DevTestTestComponentOverrideComponentBackendWorkspaceKeyPrefix := tenant1Ue2DevTestTestComponentOverrideComponentBackend["workspace_key_prefix"].(string)
	tenant1Ue2DevTestTestComponentOverrideComponentDeps := tenant1Ue2DevTestTestComponentOverrideComponent["deps"].([]string)
	tenant1Ue2DevTestTestComponentOverrideComponentLabels := tenant1Ue2DevTestTestComponentOverrideComponent["labels"].([]string)
	tenant1Ue2DevTestTestComponentOverrideTerraformWorkspace := tenant1Ue2DevTestTestComponentOverrideComponent["workspace"]
	assert.Equal(t, "tenant1-ue2-dev", tenant1Ue2DevTestTestComponentOverrideComponentInfrastructureStackName)
	assert.Equal(t, "test-test-component", tenant1Ue2DevTestTestComponentOverrideComponentBackendWorkspaceKeyPrefix)
	assert.Equal(t, "test/test-component", tenant1Ue2DevTestTestComponentOverrideComponentBaseComponent)

	assert.Equal(t, 11, len(tenant1Ue2DevTestTestComponentOverrideComponentDeps))
	assert.Contains(t, tenant1Ue2DevTestTestComponentOverrideComponentDeps, "catalog/terraform/services/service-1")
	assert.Contains(t, tenant1Ue2DevTestTestComponentOverrideComponentDeps, "catalog/terraform/services/service-1-override")
	assert.Contains(t, tenant1Ue2DevTestTestComponentOverrideComponentDeps, "catalog/terraform/services/service-2")
	assert.Contains(t, tenant1Ue2DevTestTestComponentOverrideComponentDeps, "catalog/terraform/services/service-2-override")
	assert.Contains(t, tenant1Ue2DevTestTestComponentOverrideComponentDeps, "catalog/terraform/tenant1-ue2-dev")
	assert.Contains(t, tenant1Ue2DevTestTestComponentOverrideComponentDeps, "catalog/terraform/test-component")
	assert.Contains(t, tenant1Ue2DevTestTestComponentOverrideComponentDeps, "catalog/terraform/test-component-override")
	assert.Contains(t, tenant1Ue2DevTestTestComponentOverrideComponentDeps, "globals/globals")
	assert.Contains(t, tenant1Ue2DevTestTestComponentOverrideComponentDeps, "globals/tenant1-globals")
	assert.Contains(t, tenant1Ue2DevTestTestComponentOverrideComponentDeps, "globals/ue2-globals")
	assert.Contains(t, tenant1Ue2DevTestTestComponentOverrideComponentDeps, "tenant1/ue2/dev")

	assert.Equal(t, 37, len(tenant1Ue2DevTestTestComponentOverrideComponentLabels))
	assert.Contains(t, tenant1Ue2DevTestTestComponentOverrideComponentLabels, "deps:stacks/catalog/terraform/services/service-2.yaml")
	assert.Contains(t, tenant1Ue2DevTestTestComponentOverrideComponentLabels, "deps:stacks/catalog/terraform/services/service-2-override.yaml")
	assert.Contains(t, tenant1Ue2DevTestTestComponentOverrideComponentLabels, "deps:stacks/catalog/terraform/tenant1-ue2-dev.yaml")
	assert.Contains(t, tenant1Ue2DevTestTestComponentOverrideComponentLabels, "deps:stacks/catalog/terraform/test-component.yaml")
	assert.Contains(t, tenant1Ue2DevTestTestComponentOverrideComponentLabels, "deps:stacks/catalog/terraform/test-component-override.yaml")
	assert.Contains(t, tenant1Ue2DevTestTestComponentOverrideComponentLabels, "deps:stacks/globals/globals.yaml")
	assert.Contains(t, tenant1Ue2DevTestTestComponentOverrideComponentLabels, "deps:stacks/globals/tenant1-globals.yaml")
	assert.Contains(t, tenant1Ue2DevTestTestComponentOverrideComponentLabels, "deps:stacks/globals/ue2-globals.yaml")
	assert.Contains(t, tenant1Ue2DevTestTestComponentOverrideComponentLabels, "deps:stacks/tenant1/ue2/dev.yaml")
	assert.Contains(t, tenant1Ue2DevTestTestComponentOverrideComponentLabels, "folder:component/test/test-component-override")
	assert.Contains(t, tenant1Ue2DevTestTestComponentOverrideComponentLabels, "folder:tenant1/ue2/dev")
	assert.Contains(t, tenant1Ue2DevTestTestComponentOverrideComponentLabels, "test-component-override-workspace-override")

	yamlSpaceliftStacks, err := yaml.Marshal(spaceliftStacks)
	assert.Nil(t, err)
	t.Log(string(yamlSpaceliftStacks))
}

func TestLegacySpaceliftStackProcessor(t *testing.T) {
	basePath := "../../examples/complete/stacks"

	filePaths := []string{
		"../../examples/complete/stacks/tenant1/ue2/dev.yaml",
		"../../examples/complete/stacks/tenant1/ue2/prod.yaml",
		"../../examples/complete/stacks/tenant1/ue2/staging.yaml",
		"../../examples/complete/stacks/tenant2/ue2/dev.yaml",
		"../../examples/complete/stacks/tenant2/ue2/prod.yaml",
		"../../examples/complete/stacks/tenant2/ue2/staging.yaml",
	}

	processStackDeps := true
	processComponentDeps := true
	processImports := true
	stackConfigPathTemplate := "stacks/%s.yaml"

	var spaceliftStacks, err = CreateSpaceliftStacks(basePath, filePaths, processStackDeps, processComponentDeps, processImports, stackConfigPathTemplate)
	assert.Nil(t, err)
	assert.Equal(t, 30, len(spaceliftStacks))

	tenant1Ue2DevInfraVpcStack := spaceliftStacks["tenant1-ue2-dev-infra-vpc"].(map[string]interface{})
	tenant1Ue2DevInfraVpcStackBackend := tenant1Ue2DevInfraVpcStack["backend"].(map[interface{}]interface{})
	tenant1Ue2DevInfraVpcStackBackendWorkspaceKeyPrefix := tenant1Ue2DevInfraVpcStackBackend["workspace_key_prefix"].(string)
	assert.Equal(t, "infra-vpc", tenant1Ue2DevInfraVpcStackBackendWorkspaceKeyPrefix)

	tenant1Ue2DevTestTestComponentOverrideComponent := spaceliftStacks["tenant1-ue2-dev-test-test-component-override"].(map[string]interface{})
	tenant1Ue2DevTestTestComponentOverrideComponentBackend := tenant1Ue2DevTestTestComponentOverrideComponent["backend"].(map[interface{}]interface{})
	tenant1Ue2DevTestTestComponentOverrideComponentBaseComponent := tenant1Ue2DevTestTestComponentOverrideComponent["base_component"].(string)
	tenant1Ue2DevTestTestComponentOverrideComponentBackendWorkspaceKeyPrefix := tenant1Ue2DevTestTestComponentOverrideComponentBackend["workspace_key_prefix"].(string)
	tenant1Ue2DevTestTestComponentOverrideComponentDeps := tenant1Ue2DevTestTestComponentOverrideComponent["deps"].([]string)
	tenant1Ue2DevTestTestComponentOverrideComponentLabels := tenant1Ue2DevTestTestComponentOverrideComponent["labels"].([]string)
	assert.Equal(t, "test-test-component", tenant1Ue2DevTestTestComponentOverrideComponentBackendWorkspaceKeyPrefix)
	assert.Equal(t, "test/test-component", tenant1Ue2DevTestTestComponentOverrideComponentBaseComponent)
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
	assert.Equal(t, 35, len(tenant1Ue2DevTestTestComponentOverrideComponentLabels))
	assert.Equal(t, "deps:stacks/catalog/terraform/test-component.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[28])
	assert.Equal(t, "deps:stacks/catalog/terraform/test-component-override.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[29])
	assert.Equal(t, "deps:stacks/globals/globals.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[30])
	assert.Equal(t, "deps:stacks/globals/tenant1-globals.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[31])
	assert.Equal(t, "deps:stacks/globals/ue2-globals.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[32])
	assert.Equal(t, "deps:stacks/tenant1/ue2/dev.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[33])
	assert.Equal(t, "folder:component/test/test-component-override", tenant1Ue2DevTestTestComponentOverrideComponentLabels[34])

	yamlSpaceliftStacks, err := yaml.Marshal(spaceliftStacks)
	assert.Nil(t, err)
	t.Log(string(yamlSpaceliftStacks))
}
