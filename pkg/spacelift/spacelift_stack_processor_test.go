package spacelift

import (
	"testing"

	"github.com/stretchr/testify/assert"

	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestSpaceliftStackProcessor(t *testing.T) {
	processStackDeps := true
	processComponentDeps := true
	processImports := true
	stackConfigPathTemplate := "stacks/%s.yaml"

	spaceliftStacks, err := CreateSpaceliftStacks(
		"",
		"",
		"",
		"",
		nil,
		processStackDeps,
		processComponentDeps,
		processImports,
		stackConfigPathTemplate,
	)

	assert.Nil(t, err)
	assert.Equal(t, 47, len(spaceliftStacks))

	tenant1Ue2DevInfraVpcStack := spaceliftStacks["tenant1-ue2-dev-infra-vpc"].(map[string]any)
	tenant1Ue2DevInfraVpcStackInfrastructureStackName := tenant1Ue2DevInfraVpcStack["stack"].(string)
	tenant1Ue2DevInfraVpcStackBackend := tenant1Ue2DevInfraVpcStack["backend"].(map[string]any)
	tenant1Ue2DevInfraVpcStackBackendWorkspaceKeyPrefix := tenant1Ue2DevInfraVpcStackBackend["workspace_key_prefix"].(string)
	assert.Equal(t, "tenant1-ue2-dev", tenant1Ue2DevInfraVpcStackInfrastructureStackName)
	assert.Equal(t, "infra-vpc", tenant1Ue2DevInfraVpcStackBackendWorkspaceKeyPrefix)

	// Test having a dash `-` in tenant/environment/stage names
	tenant1Ue2Test1InfraVpcStack := spaceliftStacks["tenant1-ue2-test-1-infra-vpc"].(map[string]any)
	tenant1Ue2Test1InfraVpcStackInfrastructureStackName := tenant1Ue2Test1InfraVpcStack["stack"].(string)
	tenant1Ue2Test1InfraVpcStackBackend := tenant1Ue2Test1InfraVpcStack["backend"].(map[string]any)
	tenant1Ue2Test1InfraVpcStackBackendWorkspaceKeyPrefix := tenant1Ue2Test1InfraVpcStackBackend["workspace_key_prefix"].(string)
	assert.Equal(t, "tenant1-ue2-test-1", tenant1Ue2Test1InfraVpcStackInfrastructureStackName)
	assert.Equal(t, "infra-vpc", tenant1Ue2Test1InfraVpcStackBackendWorkspaceKeyPrefix)

	tenant1Ue2DevTestTestComponentOverrideComponent := spaceliftStacks["tenant1-ue2-dev-test-test-component-override"].(map[string]any)
	tenant1Ue2DevTestTestComponentOverrideComponentInfrastructureStackName := tenant1Ue2DevTestTestComponentOverrideComponent["stack"].(string)
	tenant1Ue2DevTestTestComponentOverrideComponentBackend := tenant1Ue2DevTestTestComponentOverrideComponent["backend"].(map[string]any)
	tenant1Ue2DevTestTestComponentOverrideComponentBaseComponent := tenant1Ue2DevTestTestComponentOverrideComponent["base_component"].(string)
	tenant1Ue2DevTestTestComponentOverrideComponentBackendWorkspaceKeyPrefix := tenant1Ue2DevTestTestComponentOverrideComponentBackend["workspace_key_prefix"].(string)
	tenant1Ue2DevTestTestComponentOverrideComponentDeps := tenant1Ue2DevTestTestComponentOverrideComponent["deps"].([]string)
	tenant1Ue2DevTestTestComponentOverrideComponentLabels := tenant1Ue2DevTestTestComponentOverrideComponent["labels"].([]string)
	tenant1Ue2DevTestTestComponentOverrideTerraformWorkspace := tenant1Ue2DevTestTestComponentOverrideComponent["workspace"]

	assert.Equal(t, "tenant1-ue2-dev", tenant1Ue2DevTestTestComponentOverrideComponentInfrastructureStackName)
	assert.Equal(t, "test-test-component", tenant1Ue2DevTestTestComponentOverrideComponentBackendWorkspaceKeyPrefix)
	assert.Equal(t, "test/test-component", tenant1Ue2DevTestTestComponentOverrideComponentBaseComponent)
	assert.Equal(t, "test-component-override-workspace-override", tenant1Ue2DevTestTestComponentOverrideTerraformWorkspace)

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

	assert.Equal(t, 39, len(tenant1Ue2DevTestTestComponentOverrideComponentLabels))
	assert.Equal(t, "deps:stacks/catalog/terraform/services/service-1-override.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[27])
	assert.Equal(t, "deps:stacks/catalog/terraform/services/service-2-override.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[28])
	assert.Equal(t, "deps:stacks/catalog/terraform/spacelift-and-backend-override-1.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[29])
	assert.Equal(t, "deps:stacks/catalog/terraform/test-component.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[30])
	assert.Equal(t, "deps:stacks/catalog/terraform/test-component-override.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[31])
	assert.Equal(t, "deps:stacks/mixins/region/us-east-2.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[32])
	assert.Equal(t, "deps:stacks/mixins/stage/dev.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[33])
	assert.Equal(t, "deps:stacks/orgs/cp/_defaults.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[34])
	assert.Equal(t, "deps:stacks/orgs/cp/tenant1/_defaults.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[35])
	assert.Equal(t, "deps:stacks/orgs/cp/tenant1/dev/us-east-2.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[36])
	assert.Equal(t, "folder:component/test/test-component-override", tenant1Ue2DevTestTestComponentOverrideComponentLabels[37])
	assert.Equal(t, "folder:tenant1/ue2/dev", tenant1Ue2DevTestTestComponentOverrideComponentLabels[38])

	newTenant1Ue2DevTestTestComponentOverrideComponent2 := spaceliftStacks["tenant1-ue2-dev-new-component"].(map[string]any)
	newTenant1Ue2DevTestTestComponentOverrideComponent2InfrastructureStackName := newTenant1Ue2DevTestTestComponentOverrideComponent2["stack"].(string)
	assert.Equal(t, "tenant1-ue2-dev", newTenant1Ue2DevTestTestComponentOverrideComponent2InfrastructureStackName)

	// Test having a dash `-` in tenant/environment/stage names
	newTenant1Ue2Test1TestTestComponentOverrideComponent2 := spaceliftStacks["tenant1-ue2-test-1-new-component"].(map[string]any)
	newTenant1Ue2Test1TestTestComponentOverrideComponent2InfrastructureStackName := newTenant1Ue2Test1TestTestComponentOverrideComponent2["stack"].(string)
	assert.Equal(t, "tenant1-ue2-test-1", newTenant1Ue2Test1TestTestComponentOverrideComponent2InfrastructureStackName)

	// Test `settings.depends_on`
	tenant1Ue2ProdTopLevelComponent1 := spaceliftStacks["tenant1-ue2-prod-top-level-component1"].(map[string]any)
	tenant1Ue2ProdTopLevelComponent1Labels := tenant1Ue2ProdTopLevelComponent1["labels"].([]string)
	assert.Equal(t, "depends-on:tenant1-ue2-dev-test-test-component", tenant1Ue2ProdTopLevelComponent1Labels[35])
	assert.Equal(t, "depends-on:tenant1-ue2-prod-test-test-component-override", tenant1Ue2ProdTopLevelComponent1Labels[36])

	yamlSpaceliftStacks, err := u.ConvertToYAML(spaceliftStacks)
	assert.Nil(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			if yamlSpaceliftStacks != "" {
				t.Logf("Spacelift stacks:\n%s", yamlSpaceliftStacks)
			} else {
				t.Logf("Spacelift stacks (raw): %+v", spaceliftStacks)
			}
		}
	})
}

func TestLegacySpaceliftStackProcessor(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/complete/stacks"
	terraformComponentsBasePath := "../../tests/fixtures/scenarios/complete/components/terraform"
	helmfileComponentsBasePath := "../../tests/fixtures/scenarios/complete/components/helmfile"

	filePaths := []string{
		"../../tests/fixtures/scenarios/complete/stacks/orgs/cp/tenant1/dev/us-east-2.yaml",
		"../../tests/fixtures/scenarios/complete/stacks/orgs/cp/tenant1/prod/us-east-2.yaml",
		"../../tests/fixtures/scenarios/complete/stacks/orgs/cp/tenant1/staging/us-east-2.yaml",
		"../../tests/fixtures/scenarios/complete/stacks/orgs/cp/tenant1/test1/us-east-2.yaml",
		"../../tests/fixtures/scenarios/complete/stacks/orgs/cp/tenant2/dev/us-east-2.yaml",
		"../../tests/fixtures/scenarios/complete/stacks/orgs/cp/tenant2/prod/us-east-2.yaml",
		"../../tests/fixtures/scenarios/complete/stacks/orgs/cp/tenant2/staging/us-east-2.yaml",
	}

	processStackDeps := true
	processComponentDeps := true
	processImports := true
	stackConfigPathTemplate := "stacks/%s.yaml"

	spaceliftStacks, err := CreateSpaceliftStacks(
		stacksBasePath,
		terraformComponentsBasePath,
		helmfileComponentsBasePath,
		"",
		filePaths,
		processStackDeps,
		processComponentDeps,
		processImports,
		stackConfigPathTemplate,
	)

	assert.Nil(t, err)
	assert.Equal(t, 44, len(spaceliftStacks))

	tenant1Ue2DevInfraVpcStack := spaceliftStacks["orgs-cp-tenant1-dev-us-east-2-infra-vpc"].(map[string]any)
	tenant1Ue2DevInfraVpcStackBackend := tenant1Ue2DevInfraVpcStack["backend"].(map[string]any)
	tenant1Ue2DevInfraVpcStackBackendWorkspaceKeyPrefix := tenant1Ue2DevInfraVpcStackBackend["workspace_key_prefix"].(string)
	assert.Equal(t, "infra-vpc", tenant1Ue2DevInfraVpcStackBackendWorkspaceKeyPrefix)

	tenant1Ue2DevTestTestComponentOverrideComponent := spaceliftStacks["orgs-cp-tenant1-dev-us-east-2-test-test-component-override"].(map[string]any)
	tenant1Ue2DevTestTestComponentOverrideComponentBackend := tenant1Ue2DevTestTestComponentOverrideComponent["backend"].(map[string]any)
	tenant1Ue2DevTestTestComponentOverrideComponentBaseComponent := tenant1Ue2DevTestTestComponentOverrideComponent["base_component"].(string)
	tenant1Ue2DevTestTestComponentOverrideComponentBackendWorkspaceKeyPrefix := tenant1Ue2DevTestTestComponentOverrideComponentBackend["workspace_key_prefix"].(string)
	tenant1Ue2DevTestTestComponentOverrideComponentDeps := tenant1Ue2DevTestTestComponentOverrideComponent["deps"].([]string)
	tenant1Ue2DevTestTestComponentOverrideComponentLabels := tenant1Ue2DevTestTestComponentOverrideComponent["labels"].([]string)
	tenant1Ue2DevTestTestComponentOverrideTerraformWorkspace := tenant1Ue2DevTestTestComponentOverrideComponent["workspace"]

	assert.Equal(t, "test-test-component", tenant1Ue2DevTestTestComponentOverrideComponentBackendWorkspaceKeyPrefix)
	assert.Equal(t, "test/test-component", tenant1Ue2DevTestTestComponentOverrideComponentBaseComponent)
	assert.Equal(t, "test-component-override-workspace-override", tenant1Ue2DevTestTestComponentOverrideTerraformWorkspace)

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

	assert.Equal(t, 39, len(tenant1Ue2DevTestTestComponentOverrideComponentLabels))
	assert.Equal(t, "deps:stacks/catalog/terraform/services/service-1-override.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[27])
	assert.Equal(t, "deps:stacks/catalog/terraform/services/service-2-override.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[28])
	assert.Equal(t, "deps:stacks/catalog/terraform/spacelift-and-backend-override-1.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[29])
	assert.Equal(t, "deps:stacks/catalog/terraform/test-component.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[30])
	assert.Equal(t, "deps:stacks/catalog/terraform/test-component-override.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[31])
	assert.Equal(t, "deps:stacks/mixins/region/us-east-2.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[32])
	assert.Equal(t, "deps:stacks/mixins/stage/dev.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[33])
	assert.Equal(t, "deps:stacks/orgs/cp/_defaults.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[34])
	assert.Equal(t, "deps:stacks/orgs/cp/tenant1/_defaults.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[35])
	assert.Equal(t, "deps:stacks/orgs/cp/tenant1/dev/us-east-2.yaml", tenant1Ue2DevTestTestComponentOverrideComponentLabels[36])
	assert.Equal(t, "folder:component/test/test-component-override", tenant1Ue2DevTestTestComponentOverrideComponentLabels[37])

	yamlSpaceliftStacks, err := u.ConvertToYAML(spaceliftStacks)
	assert.Nil(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			if yamlSpaceliftStacks != "" {
				t.Logf("Spacelift stacks:\n%s", yamlSpaceliftStacks)
			} else {
				t.Logf("Spacelift stacks (raw): %+v", spaceliftStacks)
			}
		}
	})
}
