package stack

import (
	c "github.com/cloudposse/atmos/pkg/convert"
	u "github.com/cloudposse/atmos/pkg/utils"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
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

	assert.Equal(t, 16, len(imports))
	assert.Equal(t, "catalog/helmfile/echo-server", imports[0])
	assert.Equal(t, "catalog/helmfile/infra-server", imports[1])
	assert.Equal(t, "catalog/terraform/services/service-1", imports[2])
	assert.Equal(t, "catalog/terraform/services/service-1-override", imports[3])
	assert.Equal(t, "catalog/terraform/services/service-2", imports[4])
	assert.Equal(t, "catalog/terraform/services/service-2-override", imports[5])
	assert.Equal(t, "catalog/terraform/services/top-level-service-1", imports[6])
	assert.Equal(t, "catalog/terraform/services/top-level-service-2", imports[7])
	assert.Equal(t, "catalog/terraform/tenant1-ue2-dev", imports[8])
	assert.Equal(t, "catalog/terraform/test-component", imports[9])
	assert.Equal(t, "catalog/terraform/test-component-override", imports[10])
	assert.Equal(t, "catalog/terraform/top-level-component1", imports[11])
	assert.Equal(t, "catalog/terraform/vpc", imports[12])
	assert.Equal(t, "globals/globals", imports[13])
	assert.Equal(t, "globals/tenant1-globals", imports[14])
	assert.Equal(t, "globals/ue2-globals", imports[15])

	yamlConfig, err := yaml.Marshal(mapConfig1)
	assert.Nil(t, err)
	t.Log(string(yamlConfig))
}
