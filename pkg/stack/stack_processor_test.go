package stack

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestStackProcessor(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/complete/stacks"
	terraformComponentsBasePath := "../../tests/fixtures/scenarios/complete/components/terraform"
	helmfileComponentsBasePath := "../../tests/fixtures/scenarios/complete/components/helmfile"
	packerComponentsBasePath := "../../tests/fixtures/scenarios/complete/components/packer"

	filePaths := []string{
		"../../tests/fixtures/scenarios/complete/stacks/orgs/cp/tenant1/dev/us-east-2.yaml",
		"../../tests/fixtures/scenarios/complete/stacks/orgs/cp/tenant1/prod/us-east-2.yaml",
		"../../tests/fixtures/scenarios/complete/stacks/orgs/cp/tenant1/staging/us-east-2.yaml",
		"../../tests/fixtures/scenarios/complete/stacks/orgs/cp/tenant1/test1/us-east-2.yaml",
	}

	processStackDeps := true
	processComponentDeps := true

	atmosConfig := schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
				Sprig: schema.TemplatesSettingsSprig{
					Enabled: true,
				},
				Gomplate: schema.TemplatesSettingsGomplate{
					Enabled: true,
				},
			},
		},
	}

	listResult, mapResult, _, err := ProcessYAMLConfigFiles(
		&atmosConfig,
		stacksBasePath,
		terraformComponentsBasePath,
		helmfileComponentsBasePath,
		packerComponentsBasePath,
		filePaths,
		processStackDeps,
		processComponentDeps,
		false,
	)

	assert.Nil(t, err)
	assert.Equal(t, 4, len(listResult))
	assert.Equal(t, 4, len(mapResult))

	mapResultKeys := u.StringKeysFromMap(mapResult)
	assert.Equal(t, "orgs/cp/tenant1/dev/us-east-2", mapResultKeys[0])
	assert.Equal(t, "orgs/cp/tenant1/prod/us-east-2", mapResultKeys[1])
	assert.Equal(t, "orgs/cp/tenant1/staging/us-east-2", mapResultKeys[2])
	assert.Equal(t, "orgs/cp/tenant1/test1/us-east-2", mapResultKeys[3])

	mapConfig1, err := u.UnmarshalYAML[schema.AtmosSectionMapType](listResult[0])
	assert.Nil(t, err)

	imports := mapConfig1["imports"].([]any)

	mapConfig2 := mapResult["orgs/cp/tenant1/dev/us-east-2"]
	assert.Equal(t, len(imports), len(mapConfig2.(map[string]any)["imports"].([]string)))

	assert.Equal(t, 27, len(imports))
	assert.Equal(t, "catalog/helmfile/echo-server", imports[0])
	assert.Equal(t, "catalog/helmfile/infra-server", imports[1])
	assert.Equal(t, "catalog/helmfile/infra-server-override", imports[2])
	assert.Equal(t, "catalog/terraform/mixins/test-1", imports[4])
	assert.Equal(t, "catalog/terraform/mixins/test-2", imports[5])
	assert.Equal(t, "catalog/terraform/services/service-1", imports[6])
	assert.Equal(t, "catalog/terraform/services/service-1-override", imports[7])
	assert.Equal(t, "catalog/terraform/services/service-1-override-2", imports[8])
	assert.Equal(t, "catalog/terraform/services/service-2", imports[9])
	assert.Equal(t, "catalog/terraform/services/service-2-override", imports[10])
	assert.Equal(t, "catalog/terraform/services/service-2-override-2", imports[11])
	assert.Equal(t, "catalog/terraform/services/top-level-service-1", imports[12])
	assert.Equal(t, "catalog/terraform/services/top-level-service-2", imports[13])
	assert.Equal(t, "catalog/terraform/spacelift-and-backend-override-1", imports[14])
	assert.Equal(t, "catalog/terraform/tenant1-ue2-dev", imports[15])
	assert.Equal(t, "catalog/terraform/test-component", imports[16])
	assert.Equal(t, "catalog/terraform/test-component-override", imports[17])
	assert.Equal(t, "catalog/terraform/test-component-override-2", imports[18])
	assert.Equal(t, "catalog/terraform/test-component-override-3", imports[19])
	assert.Equal(t, "catalog/terraform/top-level-component1", imports[20])
	assert.Equal(t, "catalog/terraform/vpc", imports[21])
	assert.Equal(t, "mixins/region/us-east-2", imports[22])
	assert.Equal(t, "mixins/stage/dev", imports[23])
	assert.Equal(t, "orgs/cp/_defaults", imports[24])
	assert.Equal(t, "orgs/cp/tenant1/_defaults", imports[25])
	assert.Equal(t, "orgs/cp/tenant1/dev/_defaults", imports[26])

	components := mapConfig1["components"].(map[string]any)
	terraformComponents := components["terraform"].(map[string]any)
	helmfileComponents := components["helmfile"].(map[string]any)

	infraVpcComponent := terraformComponents["infra/vpc"].(map[string]any)
	infraVpcComponentBackend := infraVpcComponent["backend"].(map[string]any)
	infraVpcComponentBackendType := infraVpcComponent["backend_type"]
	infraVpcComponentRemoteSateBackend := infraVpcComponent["remote_state_backend"].(map[string]any)
	infraVpcComponentRemoteSateBackendType := infraVpcComponent["remote_state_backend_type"]
	infraVpcComponentBackendWorkspaceKeyPrefix := infraVpcComponentBackend["workspace_key_prefix"]
	infraVpcComponentRemoteStateBackendWorkspaceKeyPrefix := infraVpcComponentRemoteSateBackend["workspace_key_prefix"]
	assert.Equal(t, "infra-vpc", infraVpcComponentBackendWorkspaceKeyPrefix)
	assert.Equal(t, "infra-vpc", infraVpcComponentRemoteStateBackendWorkspaceKeyPrefix)
	assert.Equal(t, "s3", infraVpcComponentBackendType)
	assert.Equal(t, "s3", infraVpcComponentRemoteSateBackendType)

	testTestComponent := terraformComponents["test/test-component"].(map[string]any)
	testTestComponentBackend := testTestComponent["backend"].(map[string]any)
	testTestComponentBackendType := testTestComponent["backend_type"]
	testTestComponentBackendBucket := testTestComponentBackend["bucket"]
	testTestComponentBackendWorkspaceKeyPrefix := testTestComponentBackend["workspace_key_prefix"]
	testTestComponentBackendRoleArn := testTestComponentBackend["role_arn"]
	testTestComponentRemoteStateBackend := testTestComponent["remote_state_backend"].(map[string]any)
	testTestComponentRemoteStateBackendType := testTestComponent["remote_state_backend_type"]
	testTestComponentRemoteStateBackendBucket := testTestComponentRemoteStateBackend["bucket"]
	testTestComponentRemoteStateBackendWorkspaceKeyPrefix := testTestComponentRemoteStateBackend["workspace_key_prefix"]
	testTestComponentRemoteStateBackendRoleArn := testTestComponentRemoteStateBackend["role_arn"]
	assert.Equal(t, "cp-ue2-root-tfstate", testTestComponentBackendBucket)
	assert.Equal(t, "cp-ue2-root-tfstate", testTestComponentRemoteStateBackendBucket)
	assert.Equal(t, "test-test-component", testTestComponentBackendWorkspaceKeyPrefix)
	assert.Equal(t, "test-test-component", testTestComponentRemoteStateBackendWorkspaceKeyPrefix)
	assert.Equal(t, "s3", testTestComponentBackendType)
	assert.Equal(t, "s3", testTestComponentRemoteStateBackendType)
	assert.Equal(t, nil, testTestComponentBackendRoleArn)
	assert.Equal(t, "arn:aws:iam::123456789012:role/cp-gbl-root-terraform", testTestComponentRemoteStateBackendRoleArn)

	testTestComponentOverrideComponent := terraformComponents["test/test-component-override"].(map[string]any)
	testTestComponentOverrideComponentBackend := testTestComponentOverrideComponent["backend"].(map[string]any)
	testTestComponentOverrideComponentBackendType := testTestComponentOverrideComponent["backend_type"]
	testTestComponentOverrideComponentBackendWorkspaceKeyPrefix := testTestComponentOverrideComponentBackend["workspace_key_prefix"]
	testTestComponentOverrideComponentBackendBucket := testTestComponentOverrideComponentBackend["bucket"]
	testTestComponentOverrideComponentRemoteStateBackend := testTestComponentOverrideComponent["remote_state_backend"].(map[string]any)
	testTestComponentOverrideComponentRemoteStateBackendVal1 := testTestComponentOverrideComponentRemoteStateBackend["val1"].(bool)
	testTestComponentOverrideComponentRemoteStateBackendVal2 := testTestComponentOverrideComponentRemoteStateBackend["val2"]
	testTestComponentOverrideComponentRemoteStateBackendVal3 := testTestComponentOverrideComponentRemoteStateBackend["val3"].(int)
	testTestComponentOverrideComponentRemoteStateBackendVal4 := testTestComponentOverrideComponentRemoteStateBackend["val4"]
	testTestComponentOverrideComponentRemoteStateBackendType := testTestComponentOverrideComponent["remote_state_backend_type"]
	testTestComponentOverrideComponentBaseComponent := testTestComponentOverrideComponent["component"]
	assert.Equal(t, "test-test-component", testTestComponentOverrideComponentBackendWorkspaceKeyPrefix)
	assert.Equal(t, "cp-ue2-root-tfstate", testTestComponentOverrideComponentBackendBucket)
	assert.Equal(t, "test/test-component", testTestComponentOverrideComponentBaseComponent)
	assert.Equal(t, "s3", testTestComponentOverrideComponentBackendType)
	assert.Equal(t, "static", testTestComponentOverrideComponentRemoteStateBackendType)
	assert.Equal(t, true, testTestComponentOverrideComponentRemoteStateBackendVal1)
	assert.Equal(t, "2", testTestComponentOverrideComponentRemoteStateBackendVal2)
	assert.Equal(t, 3, testTestComponentOverrideComponentRemoteStateBackendVal3)
	assert.Equal(t, nil, testTestComponentOverrideComponentRemoteStateBackendVal4)

	topLevelComponent1 := terraformComponents["top-level-component1"].(map[string]any)
	topLevelComponent1Backend := topLevelComponent1["backend"].(map[string]any)
	topLevelComponent1RemoteSateBackend := topLevelComponent1["remote_state_backend"].(map[string]any)
	topLevelComponent1BackendWorkspaceKeyPrefix := topLevelComponent1Backend["workspace_key_prefix"]
	topLevelComponent1RemoteStateBackendWorkspaceKeyPrefix := topLevelComponent1RemoteSateBackend["workspace_key_prefix"]
	assert.Equal(t, "top-level-component1", topLevelComponent1BackendWorkspaceKeyPrefix)
	assert.Equal(t, "top-level-component1", topLevelComponent1RemoteStateBackendWorkspaceKeyPrefix)

	testTestComponentOverrideComponent2 := terraformComponents["test/test-component-override-2"].(map[string]any)
	testTestComponentOverrideComponentBackend2 := testTestComponentOverrideComponent2["backend"].(map[string]any)
	testTestComponentOverrideComponentBackendType2 := testTestComponentOverrideComponent2["backend_type"]
	testTestComponentOverrideComponentBackendWorkspaceKeyPrefix2 := testTestComponentOverrideComponentBackend2["workspace_key_prefix"]
	testTestComponentOverrideComponentBackendBucket2 := testTestComponentOverrideComponentBackend2["bucket"]
	testTestComponentOverrideComponentBaseComponent2 := testTestComponentOverrideComponent2["component"]
	testTestComponentOverrideInheritance2 := testTestComponentOverrideComponent2["inheritance"].([]any)
	assert.Equal(t, "test-test-component", testTestComponentOverrideComponentBackendWorkspaceKeyPrefix2)
	assert.Equal(t, "cp-ue2-root-tfstate", testTestComponentOverrideComponentBackendBucket2)
	assert.Equal(t, "test/test-component", testTestComponentOverrideComponentBaseComponent2)
	assert.Equal(t, "s3", testTestComponentOverrideComponentBackendType2)
	assert.Equal(t, "test/test-component-override", testTestComponentOverrideInheritance2[0])
	assert.Equal(t, "test/test-component", testTestComponentOverrideInheritance2[1])

	infraInfraServerOverrideComponent := helmfileComponents["infra/infra-server-override"].(map[string]any)
	infraInfraServerOverrideComponentCommand := infraInfraServerOverrideComponent["command"]
	infraInfraServerOverrideComponentVars := infraInfraServerOverrideComponent["vars"].(map[string]any)
	infraInfraServerOverrideComponentVarsA := infraInfraServerOverrideComponentVars["a"]
	infraInfraServerOverrideComponentInheritance := infraInfraServerOverrideComponent["inheritance"].([]any)
	assert.Equal(t, "helmfile", infraInfraServerOverrideComponentCommand)
	assert.Equal(t, "infra/infra-server", infraInfraServerOverrideComponentInheritance[0])
	assert.Equal(t, "1_override", infraInfraServerOverrideComponentVarsA)

	testTestComponentOverrideComponent3 := terraformComponents["test/test-component-override-3"].(map[string]any)
	testTestComponentOverrideComponent3Metadata := testTestComponentOverrideComponent3["metadata"].(map[string]any)
	testTestComponentOverrideComponent3TerraformWorkspace := testTestComponentOverrideComponent3Metadata["terraform_workspace"]
	assert.Equal(t, "test-component-override-3-workspace", testTestComponentOverrideComponent3TerraformWorkspace)

	yamlConfig, err := u.ConvertToYAML(mapConfig1)
	assert.Nil(t, err)
	t.Log(yamlConfig)
}

func TestStackProcessorRelativePaths(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/relative-paths/stacks"
	terraformComponentsBasePath := "../../tests/fixtures/components/terraform"

	filePaths := []string{
		"../../tests/fixtures/scenarios/relative-paths/stacks/orgs/acme/platform/dev.yaml",
		"../../tests/fixtures/scenarios/relative-paths/stacks/orgs/acme/platform/prod.yaml",
	}

	atmosConfig := schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
				Sprig: schema.TemplatesSettingsSprig{
					Enabled: true,
				},
				Gomplate: schema.TemplatesSettingsGomplate{
					Enabled: true,
				},
			},
		},
	}

	listResult, mapResult, _, err := ProcessYAMLConfigFiles(
		&atmosConfig,
		stacksBasePath,
		terraformComponentsBasePath,
		"",
		"",
		filePaths,
		true,
		true,
		false,
	)

	assert.Nil(t, err)
	assert.Equal(t, 2, len(listResult))
	assert.Equal(t, 2, len(mapResult))

	mapResultKeys := u.StringKeysFromMap(mapResult)
	assert.Equal(t, "orgs/acme/platform/dev", mapResultKeys[0])
	assert.Equal(t, "orgs/acme/platform/prod", mapResultKeys[1])

	mapConfig1, err := u.UnmarshalYAML[schema.AtmosSectionMapType](listResult[0])
	assert.Nil(t, err)

	components := mapConfig1["components"].(map[string]any)
	terraformComponents := components["terraform"].(map[string]any)

	mockComponent := terraformComponents["mock"].(map[string]any)
	assert.NotNil(t, mockComponent)
}

func TestProcessYAMLConfigFile(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/relative-paths/stacks"
	filePath := "../../tests/fixtures/scenarios/relative-paths/stacks/orgs/acme/platform/dev.yaml"

	atmosConfig := schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
				Sprig: schema.TemplatesSettingsSprig{
					Enabled: true,
				},
				Gomplate: schema.TemplatesSettingsGomplate{
					Enabled: true,
				},
			},
		},
	}

	_, _, stackConfigMap, _, _, _, _, err := ProcessYAMLConfigFile(
		&atmosConfig,
		stacksBasePath,
		filePath,
		map[string]map[string]any{},
		nil,
		false,
		false,
		true,
		false,
		nil,
		nil,
		nil,
		nil,
		"",
	)

	assert.Nil(t, err)
	assert.Equal(t, 3, len(stackConfigMap))

	mapResultKeys := u.StringKeysFromMap(stackConfigMap)
	// sorting so that the output is deterministic
	sort.Strings(mapResultKeys)

	assert.Equal(t, "components", mapResultKeys[0])
	assert.Equal(t, "import", mapResultKeys[1])
	assert.Equal(t, "vars", mapResultKeys[2])
}

func TestProcessYAMLConfigFiles(t *testing.T) {
	t.Run("should handle empty file paths", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Templates: schema.Templates{
				Settings: schema.TemplatesSettings{
					Enabled: true,
					Sprig: schema.TemplatesSettingsSprig{
						Enabled: true,
					},
					Gomplate: schema.TemplatesSettingsGomplate{
						Enabled: true,
					},
				},
			},
		}

		listResult, mapResult, importsConfig, err := ProcessYAMLConfigFiles(
			atmosConfig,
			"stacks",
			"terraform",
			"helmfile",
			"packer",
			[]string{},
			true,
			true,
			false,
		)

		assert.NoError(t, err)
		assert.Empty(t, listResult)
		assert.Empty(t, mapResult)
		assert.Empty(t, importsConfig)
	})

	t.Run("should error on missing files without ignoreMissingFiles", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Templates: schema.Templates{
				Settings: schema.TemplatesSettings{
					Enabled: true,
				},
			},
		}

		_, _, _, err := ProcessYAMLConfigFiles(
			atmosConfig,
			"stacks",
			"terraform",
			"helmfile",
			"packer",
			[]string{"non-existent-file.yaml"},
			true,
			true,
			false,
		)

		assert.Error(t, err)
	})

	t.Run("should process valid config files", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Templates: schema.Templates{
				Settings: schema.TemplatesSettings{
					Enabled: true,
					Sprig: schema.TemplatesSettingsSprig{
						Enabled: true,
					},
					Gomplate: schema.TemplatesSettingsGomplate{
						Enabled: true,
					},
				},
			},
		}

		filePaths := []string{
			"../../tests/fixtures/scenarios/complete/stacks/orgs/cp/tenant1/dev/us-east-2.yaml",
		}

		listResult, mapResult, _, err := ProcessYAMLConfigFiles(
			atmosConfig,
			"../../tests/fixtures/scenarios/complete/stacks",
			"../../tests/fixtures/scenarios/complete/components/terraform",
			"../../tests/fixtures/scenarios/complete/components/helmfile",
			"../../tests/fixtures/scenarios/complete/components/packer",
			filePaths,
			true,
			true,
			false,
		)

		assert.NoError(t, err)
		assert.NotEmpty(t, listResult)
		assert.NotEmpty(t, mapResult)
		assert.Equal(t, 1, len(listResult))
		assert.Equal(t, 1, len(mapResult))

		// Verify the content of the processed file
		config, err := u.UnmarshalYAML[schema.AtmosSectionMapType](listResult[0])
		assert.NoError(t, err)

		// Check if the components section exists
		components, ok := config["components"].(map[string]any)
		assert.True(t, ok)
		assert.NotNil(t, components)

		// Verify the stack name is correctly mapped
		assert.Contains(t, mapResult, "orgs/cp/tenant1/dev/us-east-2")
	})
}
