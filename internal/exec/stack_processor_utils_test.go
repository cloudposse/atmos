package exec

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestProcessBaseComponentConfig(t *testing.T) {
	tests := []struct {
		name                string
		baseComponentConfig *schema.BaseComponentConfig
		allComponentsMap    map[string]any
		component           string
		stack               string
		baseComponent       string
		expectedError       string
		expectedVars        map[string]any
		expectedSettings    map[string]any
		expectedEnv         map[string]any
		expectedBackendType string
		expectBaseComponent string
	}{
		{
			name: "basic-base-component",
			baseComponentConfig: &schema.BaseComponentConfig{
				BaseComponentVars:     map[string]any{},
				BaseComponentSettings: map[string]any{},
				BaseComponentEnv:      map[string]any{},
			},
			allComponentsMap: map[string]any{
				"base": map[string]any{
					"vars": map[string]any{
						"environment": "dev",
						"region":      "us-east-1",
					},
					"settings": map[string]any{
						"enabled": true,
					},
					"backend_type": "s3",
				},
			},
			component:     "test",
			stack:         "test-stack",
			baseComponent: "base",
			expectedVars: map[string]any{
				"environment": "dev",
				"region":      "us-east-1",
			},
			expectedSettings: map[string]any{
				"enabled": true,
			},
			expectedBackendType: "s3",
			expectBaseComponent: "base",
		},
		{
			name: "inheritance-chain",
			baseComponentConfig: &schema.BaseComponentConfig{
				BaseComponentVars:     map[string]any{},
				BaseComponentSettings: map[string]any{},
				BaseComponentEnv:      map[string]any{},
			},
			allComponentsMap: map[string]any{
				"base": map[string]any{
					"component": "base2",
					"vars": map[string]any{
						"environment": "dev",
					},
				},
				"base2": map[string]any{
					"vars": map[string]any{
						"region": "us-east-1",
					},
					"settings": map[string]any{
						"enabled": true,
					},
				},
			},
			component:     "test",
			stack:         "test-stack",
			baseComponent: "base",
			expectedVars: map[string]any{
				"environment": "dev",
				"region":      "us-east-1",
			},
			expectedSettings: map[string]any{
				"enabled": true,
			},
			expectBaseComponent: "base2",
		},
		{
			name: "invalid-base-component",
			baseComponentConfig: &schema.BaseComponentConfig{
				BaseComponentVars:     map[string]any{},
				BaseComponentSettings: map[string]any{},
				BaseComponentEnv:      map[string]any{},
			},
			allComponentsMap: map[string]any{
				"base": "invalid-type",
			},
			component:     "test",
			stack:         "test-stack",
			baseComponent: "base",
			expectedError: "invalid base component config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{}
			baseComponents := []string{}

			err := ProcessBaseComponentConfig(
				atmosConfig,
				tt.baseComponentConfig,
				tt.allComponentsMap,
				tt.component,
				tt.stack,
				tt.baseComponent,
				"/dummy/path",
				true,
				&baseComponents,
			)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)

			if tt.expectedVars != nil {
				assert.Equal(t, tt.expectedVars, tt.baseComponentConfig.BaseComponentVars)
			}

			if tt.expectedSettings != nil {
				assert.Equal(t, tt.expectedSettings, tt.baseComponentConfig.BaseComponentSettings)
			}

			if tt.expectedEnv != nil {
				assert.Equal(t, tt.expectedEnv, tt.baseComponentConfig.BaseComponentEnv)
			}

			if tt.expectedBackendType != "" {
				assert.Equal(t, tt.expectedBackendType, tt.baseComponentConfig.BaseComponentBackendType)
			}

			if tt.expectBaseComponent != "" {
				assert.Equal(t, tt.expectBaseComponent, tt.baseComponentConfig.FinalBaseComponentName)
			}

			// Verify baseComponents slice contains the expected components
			expectedComponents := []string{tt.baseComponent}
			if tt.expectBaseComponent != tt.baseComponent && tt.expectBaseComponent != "" {
				expectedComponents = append(expectedComponents, tt.expectBaseComponent)
			}
			assert.ElementsMatch(t, expectedComponents, baseComponents)
		})
	}
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
		&atmosConfig,
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
		Stacks: schema.Stacks{
			BasePath: "stacks",
		},
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

	// Verify error is returned.
	assert.Error(t, err)

	// Verify it's our specific error type.
	assert.ErrorIs(t, err, errUtils.ErrStackManifestFileNotFound)

	// Verify error message contains the sentinel error text.
	errMsg := err.Error()
	assert.Contains(t, errMsg, "stack manifest file not found")
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

	assert.Error(t, err)
}

func TestProcessStackConfigProviderSection(t *testing.T) {
	basePath := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "atmos-providers-section")
	stacksBasePath := filepath.Join(basePath, "stacks")
	manifest := filepath.Join(stacksBasePath, "deploy", "nonprod.yaml")

	atmosConfig := schema.AtmosConfiguration{
		Logs: schema.Logs{
			Level: "Info",
		},
	}

	deepMergedStackConfig, importsConfig, _, _, _, _, _, err := ProcessYAMLConfigFile(
		&atmosConfig,
		stacksBasePath,
		manifest,
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

	config, err := ProcessStackConfig(
		&atmosConfig,
		stacksBasePath,
		filepath.Join(basePath, "components", "terraform"),
		filepath.Join(basePath, "components", "helmfile"),
		filepath.Join(basePath, "components", "packer"),
		"nonprod",
		deepMergedStackConfig,
		false,
		false,
		"",
		map[string]map[string][]string{},
		importsConfig,
		true,
	)
	assert.Nil(t, err)

	providers, err := u.EvaluateYqExpression(&atmosConfig, config, ".components.terraform.component-2.providers")
	assert.Nil(t, err)

	awsProvider, err := u.EvaluateYqExpression(&atmosConfig, providers, ".aws")
	assert.Nil(t, err)

	contextProvider, err := u.EvaluateYqExpression(&atmosConfig, providers, ".context")
	assert.Nil(t, err)

	awsProviderRoleArn, err := u.EvaluateYqExpression(&atmosConfig, awsProvider, ".assume_role.role_arn")
	assert.Nil(t, err)
	assert.Equal(t, "Derived component IAM Role ARN", awsProviderRoleArn)

	contextProviderPropertyOrder0, err := u.EvaluateYqExpression(&atmosConfig, contextProvider, ".property_order[0]")
	assert.Nil(t, err)
	assert.Equal(t, "product", contextProviderPropertyOrder0)
}

func TestProcessSettingsIntegrationsGithub(t *testing.T) {
	tests := []struct {
		name           string
		inputSettings  map[string]any
		expectedOutput map[string]any
		expectError    bool
		errorContains  string
	}{
		{
			name: "Valid GitHub integration settings",
			inputSettings: map[string]any{
				"github": map[string]any{
					"token":      "test-token",
					"owner":      "test-owner",
					"repository": "test-repo",
					"branch":     "main",
				},
			},
			expectedOutput: map[string]any{
				"github": map[string]any{
					"token":      "test-token",
					"owner":      "test-owner",
					"repository": "test-repo",
					"branch":     "main",
				},
			},
			expectError: false,
		},
		{
			name: "Additional valid fields",
			inputSettings: map[string]any{
				"github": map[string]any{
					"token":          "test-token",
					"owner":          "test-owner",
					"repository":     "test-repo",
					"branch":         "develop",
					"base_branch":    "main",
					"webhook_secret": "secret123",
				},
			},
			expectedOutput: map[string]any{
				"github": map[string]any{
					"token":          "test-token",
					"owner":          "test-owner",
					"repository":     "test-repo",
					"branch":         "develop",
					"base_branch":    "main",
					"webhook_secret": "secret123",
				},
			},
			expectError: false,
		},
		{
			name: "With workspace configuration",
			inputSettings: map[string]any{
				"github": map[string]any{
					"token":      "test-token",
					"owner":      "test-owner",
					"repository": "test-repo",
					"workspaces": map[string]any{
						"prefix": "test-",
						"suffix": "-prod",
					},
				},
			},
			expectedOutput: map[string]any{
				"github": map[string]any{
					"token":      "test-token",
					"owner":      "test-owner",
					"repository": "test-repo",
					"workspaces": map[string]any{
						"prefix": "test-",
						"suffix": "-prod",
					},
				},
			},
			expectError: false,
		},
		{
			name: "With path configuration",
			inputSettings: map[string]any{
				"github": map[string]any{
					"token":      "test-token",
					"owner":      "test-owner",
					"repository": "test-repo",
					"paths": []any{
						"terraform/**",
						"modules/**",
					},
				},
			},
			expectedOutput: map[string]any{
				"github": map[string]any{
					"token":      "test-token",
					"owner":      "test-owner",
					"repository": "test-repo",
					"paths": []any{
						"terraform/**",
						"modules/**",
					},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processSettingsIntegrationsGithub(&schema.AtmosConfiguration{}, tt.inputSettings)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedOutput, result)

				// Additional validation for required fields
				githubConfig := result["github"].(map[string]any)
				assert.Contains(t, githubConfig, "token")
				assert.Contains(t, githubConfig, "owner")
				assert.Contains(t, githubConfig, "repository")

				// Type assertions for key fields
				assert.IsType(t, "", githubConfig["token"])
				assert.IsType(t, "", githubConfig["owner"])
				assert.IsType(t, "", githubConfig["repository"])

				// Optional field type assertions
				if branch, ok := githubConfig["branch"]; ok {
					assert.IsType(t, "", branch)
				}
				if workspaces, ok := githubConfig["workspaces"]; ok {
					assert.IsType(t, map[string]any{}, workspaces)
				}
				if paths, ok := githubConfig["paths"]; ok {
					assert.IsType(t, []any{}, paths)
				}
			}
		})
	}
}

func TestProcessSettingsIntegrationsGithub_MissingGithubConfig(t *testing.T) {
	settings := map[string]any{}

	result, err := processSettingsIntegrationsGithub(&schema.AtmosConfiguration{}, settings)

	assert.Nil(t, err)
	assert.Equal(t, settings, result)
}

func TestProcessYAMLConfigFiles(t *testing.T) {
	stacksBasePath := "../../tests/fixtures/scenarios/relative-paths/stacks"
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

	listResult, mapResult, rawStackConfigs, err := ProcessYAMLConfigFiles(
		&atmosConfig,
		stacksBasePath,
		"", // terraformComponentsBasePath
		"", // helmfileComponentsBasePath
		"", // packerComponentsBasePath
		filePaths,
		true,  // processStackDeps
		true,  // processComponentDeps
		false, // ignoreMissingFiles
	)

	// Verify no error occurred
	assert.Nil(t, err)

	// Verify listResult contains the expected number of results
	assert.Equal(t, len(filePaths), len(listResult))

	// Verify mapResult contains the expected stack names
	assert.Equal(t, len(filePaths), len(mapResult))

	// Verify rawStackConfigs contains the expected stack names
	assert.Equal(t, len(filePaths), len(rawStackConfigs))
}

func TestSectionContainsAnyNotEmptySections(t *testing.T) {
	tests := []struct {
		name            string
		section         map[string]any
		sectionsToCheck []string
		expected        bool
	}{
		{
			name:            "empty section and empty check list",
			section:         map[string]any{},
			sectionsToCheck: []string{},
			expected:        false,
		},
		{
			name:            "empty section with check list",
			section:         map[string]any{},
			sectionsToCheck: []string{"vars", "settings"},
			expected:        false,
		},
		{
			name: "section with empty map value",
			section: map[string]any{
				"vars": map[string]any{},
			},
			sectionsToCheck: []string{"vars"},
			expected:        false,
		},
		{
			name: "section with non-empty map value",
			section: map[string]any{
				"vars": map[string]any{
					"key": "value",
				},
			},
			sectionsToCheck: []string{"vars"},
			expected:        true,
		},
		{
			name: "section with empty string value",
			section: map[string]any{
				"backend_type": "",
			},
			sectionsToCheck: []string{"backend_type"},
			expected:        false,
		},
		{
			name: "section with non-empty string value",
			section: map[string]any{
				"backend_type": "s3",
			},
			sectionsToCheck: []string{"backend_type"},
			expected:        true,
		},
		{
			name: "multiple sections - first empty, second has value",
			section: map[string]any{
				"vars":     map[string]any{},
				"settings": map[string]any{"key": "value"},
			},
			sectionsToCheck: []string{"vars", "settings"},
			expected:        true,
		},
		{
			name: "check non-existent section",
			section: map[string]any{
				"vars": map[string]any{"key": "value"},
			},
			sectionsToCheck: []string{"non_existent"},
			expected:        false,
		},
		{
			name: "section with nil value",
			section: map[string]any{
				"vars": nil,
			},
			sectionsToCheck: []string{"vars"},
			expected:        false,
		},
		{
			name: "section with non-map non-string value",
			section: map[string]any{
				"count": 42,
			},
			sectionsToCheck: []string{"count"},
			expected:        false,
		},
		{
			name: "empty string in sectionsToCheck should be ignored",
			section: map[string]any{
				"vars": map[string]any{"key": "value"},
			},
			sectionsToCheck: []string{"", "vars"},
			expected:        true,
		},
		{
			name: "only empty strings in sectionsToCheck",
			section: map[string]any{
				"vars": map[string]any{"key": "value"},
			},
			sectionsToCheck: []string{"", ""},
			expected:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sectionContainsAnyNotEmptySections(tt.section, tt.sectionsToCheck)
			assert.Equal(t, tt.expected, result)
		})
	}
}
