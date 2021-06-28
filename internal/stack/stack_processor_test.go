package stack

import (
	c "github.com/cloudposse/terraform-provider-utils/internal/convert"
	u "github.com/cloudposse/terraform-provider-utils/internal/utils"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
)

func TestStackProcessor(t *testing.T) {
	filePaths := []string{
		"../../examples/data-sources/utils_stack_config_yaml/stacks/uw2-dev.yaml",
		"../../examples/data-sources/utils_stack_config_yaml/stacks/uw2-prod.yaml",
		"../../examples/data-sources/utils_stack_config_yaml/stacks/uw2-staging.yaml",
		"../../examples/data-sources/utils_stack_config_yaml/stacks/uw2-uat.yaml",
	}

	processStackDeps := true
	processComponentDeps := true

	var listResult, mapResult, err = ProcessYAMLConfigFiles(filePaths, processStackDeps, processComponentDeps)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(listResult))
	assert.Equal(t, 4, len(mapResult))

	mapResultKeys := u.StringKeysFromMap(mapResult)
	assert.Equal(t, "uw2-dev", mapResultKeys[0])
	assert.Equal(t, "uw2-prod", mapResultKeys[1])
	assert.Equal(t, "uw2-staging", mapResultKeys[2])
	assert.Equal(t, "uw2-uat", mapResultKeys[3])

	mapConfig1, err := c.YAMLToMapOfInterfaces(listResult[0])
	assert.Nil(t, err)

	terraformComponents := mapConfig1["components"].(map[interface{}]interface{})["terraform"].(map[interface{}]interface{})
	helmfileComponents := mapConfig1["components"].(map[interface{}]interface{})["helmfile"].(map[interface{}]interface{})

	imports := mapConfig1["imports"].([]interface{})

	mapConfig2 := mapResult["uw2-dev"]
	assert.Equal(t, len(imports), len(mapConfig2.(map[interface{}]interface{})["imports"].([]string)))

	auroraPostgresComponent := terraformComponents["aurora-postgres"].(map[interface{}]interface{})
	auroraPostgres2Component := terraformComponents["aurora-postgres-2"].(map[interface{}]interface{})

	assert.Equal(t, "aurora-postgres", auroraPostgres2Component["component"])
	assert.Equal(t, "dev", auroraPostgres2Component["settings"].(map[interface{}]interface{})["spacelift"].(map[interface{}]interface{})["branch"])
	assert.Equal(t, "db.r4.xlarge", auroraPostgres2Component["vars"].(map[interface{}]interface{})["instance_type"])
	assert.Equal(t, "test1_override2", auroraPostgres2Component["env"].(map[interface{}]interface{})["ENV_TEST_1"].(string))
	assert.Equal(t, "test2_override2", auroraPostgres2Component["env"].(map[interface{}]interface{})["ENV_TEST_2"].(string))
	assert.Equal(t, "test3", auroraPostgres2Component["env"].(map[interface{}]interface{})["ENV_TEST_3"].(string))
	assert.Equal(t, "test4", auroraPostgres2Component["env"].(map[interface{}]interface{})["ENV_TEST_4"].(string))
	assert.Equal(t, "test5", auroraPostgres2Component["env"].(map[interface{}]interface{})["ENV_TEST_5"].(string))
	assert.Equal(t, "test6", auroraPostgres2Component["env"].(map[interface{}]interface{})["ENV_TEST_6"].(string))
	assert.Equal(t, "test7", auroraPostgres2Component["env"].(map[interface{}]interface{})["ENV_TEST_7"].(string))
	assert.Equal(t, "test8", auroraPostgres2Component["env"].(map[interface{}]interface{})["ENV_TEST_8"].(string))
	assert.Nil(t, auroraPostgres2Component["env"].(map[interface{}]interface{})["ENV_TEST_9"])

	if processStackDeps {
		assert.Equal(t, "catalog/rds-defaults", auroraPostgres2Component["stacks"].([]interface{})[0])
		assert.Equal(t, "globals", auroraPostgres2Component["stacks"].([]interface{})[1])
		assert.Equal(t, "uw2-dev", auroraPostgres2Component["stacks"].([]interface{})[2])
		assert.Equal(t, "uw2-globals", auroraPostgres2Component["stacks"].([]interface{})[3])
		assert.Equal(t, "uw2-prod", auroraPostgres2Component["stacks"].([]interface{})[4])
		assert.Equal(t, "uw2-staging", auroraPostgres2Component["stacks"].([]interface{})[5])
		assert.Equal(t, "uw2-uat", auroraPostgres2Component["stacks"].([]interface{})[6])
	}

	if processComponentDeps {
		assert.Equal(t, "catalog/rds-defaults", auroraPostgresComponent["deps"].([]interface{})[0])
		assert.Equal(t, "globals", auroraPostgresComponent["deps"].([]interface{})[1])
		assert.Equal(t, "uw2-dev", auroraPostgresComponent["deps"].([]interface{})[2])
		assert.Equal(t, "uw2-globals", auroraPostgresComponent["deps"].([]interface{})[3])

		assert.Equal(t, "catalog/rds-defaults", auroraPostgres2Component["deps"].([]interface{})[0])
		assert.Equal(t, "globals", auroraPostgres2Component["deps"].([]interface{})[1])
		assert.Equal(t, "uw2-dev", auroraPostgres2Component["deps"].([]interface{})[2])
		assert.Equal(t, "uw2-globals", auroraPostgres2Component["deps"].([]interface{})[3])
	}

	eksComponent := terraformComponents["eks"].(map[interface{}]interface{})
	assert.Equal(t, true, eksComponent["settings"].(map[interface{}]interface{})["spacelift"].(map[interface{}]interface{})["workspace_enabled"])
	assert.Equal(t, "test", eksComponent["settings"].(map[interface{}]interface{})["spacelift"].(map[interface{}]interface{})["branch"])
	assert.Equal(t, 3, eksComponent["vars"].(map[interface{}]interface{})["spotinst_oceans"].(map[interface{}]interface{})["main"].(map[interface{}]interface{})["max_group_size"])
	assert.Equal(t, "eg-gbl-dev-spotinst-worker", eksComponent["vars"].(map[interface{}]interface{})["spotinst_instance_profile"])
	assert.Equal(t, "test1_override", eksComponent["env"].(map[interface{}]interface{})["ENV_TEST_1"].(string))
	assert.Equal(t, "test2_override", eksComponent["env"].(map[interface{}]interface{})["ENV_TEST_2"].(string))
	assert.Equal(t, "test3", eksComponent["env"].(map[interface{}]interface{})["ENV_TEST_3"].(string))
	assert.Equal(t, "test4", eksComponent["env"].(map[interface{}]interface{})["ENV_TEST_4"].(string))
	assert.Nil(t, eksComponent["env"].(map[interface{}]interface{})["ENV_TEST_5"])

	accountComponent := terraformComponents["account"].(map[interface{}]interface{})
	assert.Equal(t, "s3", accountComponent["backend_type"].(string))
	assert.Equal(t, "account", accountComponent["backend"].(map[interface{}]interface{})["workspace_key_prefix"])
	assert.Equal(t, "eg-uw2-root-tfstate", accountComponent["backend"].(map[interface{}]interface{})["bucket"])
	assert.Nil(t, accountComponent["backend"].(map[interface{}]interface{})["role_arn"])

	datadogHelmfileComponent := helmfileComponents["datadog"].(map[interface{}]interface{})
	assert.Equal(t, "1234567890", datadogHelmfileComponent["vars"].(map[interface{}]interface{})["account_number"])
	assert.Equal(t, true, datadogHelmfileComponent["vars"].(map[interface{}]interface{})["installed"])
	assert.Equal(t, "dev", datadogHelmfileComponent["vars"].(map[interface{}]interface{})["stage"])
	assert.Equal(t, true, datadogHelmfileComponent["vars"].(map[interface{}]interface{})["processAgent"].(map[interface{}]interface{})["enabled"])

	assert.Equal(t, 5, len(imports))
	assert.Equal(t, "catalog/eks-defaults", imports[0])
	assert.Equal(t, "catalog/rds-defaults", imports[1])
	assert.Equal(t, "catalog/s3-defaults", imports[2])
	assert.Equal(t, "globals", imports[3])
	assert.Equal(t, "uw2-globals", imports[4])

	yamlConfig, err := yaml.Marshal(mapConfig1)
	assert.Nil(t, err)
	t.Log(string(yamlConfig))
}
