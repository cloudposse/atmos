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

func TestProcessYAMLConfigFileIgnoreMissingFiles(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks"
	filePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks/orgs/acme/platform/not-present.yaml"
	ignoreMissingFiles := true

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
		ignoreMissingFiles,
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
	assert.Equal(t, 0, len(stackConfigMap))
}

func TestProcessYAMLConfigFileMissingFilesReturnError(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks"
	filePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks/orgs/acme/platform/not-present.yaml"

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

	_, _, _, _, _, _, _, err := ProcessYAMLConfigFile(
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

	assert.Error(t, err)
}

func TestProcessYAMLConfigFileEmptyManifest(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks"
	filePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks/orgs/acme/platform/empty.yaml"

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
	assert.Equal(t, 0, len(stackConfigMap))
}

func TestProcessYAMLConfigFileInvalidManifest(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks"
	filePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks/orgs/acme/platform/invalid.yaml"

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

	_, _, _, _, _, _, _, err := ProcessYAMLConfigFile(
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

	assert.Error(t, err)
}

func TestProcessYAMLConfigFileInvalidImportTemplate(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks"
	filePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks/orgs/acme/platform/invalid-import-template.yaml"

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

	_, _, _, _, _, _, _, err := ProcessYAMLConfigFile(
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

	assert.Error(t, err)
}

func TestProcessYAMLConfigFileInvalidValidationSchemaPath(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks"
	filePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks/orgs/acme/platform/dev.yaml"
	atmosManifestJsonSchemaFilePath := "does-not-exist"

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

	_, _, _, _, _, _, _, err := ProcessYAMLConfigFile(
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
		atmosManifestJsonSchemaFilePath,
	)

	assert.Error(t, err)
}

func TestProcessYAMLConfigFileInvalidManifestSchema(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks"
	filePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks/orgs/acme/platform/invalid-manifest-schema.yaml"
	atmosManifestJsonSchemaFilePath := "../../tests/fixtures/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json"

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

	_, _, _, _, _, _, _, err := ProcessYAMLConfigFile(
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
		atmosManifestJsonSchemaFilePath,
	)

	assert.Error(t, err)
}

func TestProcessYAMLConfigFileInvalidGlobalOverridesSection(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks"
	filePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks/orgs/acme/platform/invalid-global-overrides.yaml"

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

	_, _, _, _, _, _, _, err := ProcessYAMLConfigFile(
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

	assert.Error(t, err)
}

func TestProcessYAMLConfigFileInvalidTerraformOverridesSection(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks"
	filePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks/orgs/acme/platform/invalid-terraform-overrides.yaml"

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

	_, _, _, _, _, _, _, err := ProcessYAMLConfigFile(
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

	assert.Error(t, err)
}

func TestProcessYAMLConfigFileInvalidHelmfileOverridesSection(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks"
	filePath := "../../tests/fixtures/scenarios/invalid-stacks/stacks/orgs/acme/platform/invalid-helmfile-overrides.yaml"

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

	_, _, _, _, _, _, _, err := ProcessYAMLConfigFile(
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

	assert.Error(t, err)
}
