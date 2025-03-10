package exec

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

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
		atmosConfig,
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
