package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestEvaluateImportCondition tests the evaluateImportCondition helper.
func TestEvaluateImportCondition(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{Enabled: true},
		},
	}

	tests := []struct {
		name      string
		condition string
		context   map[string]any
		want      bool
		wantErr   bool
	}{
		{
			name:      "empty condition returns true",
			condition: "",
			context:   map[string]any{},
			want:      true,
		},
		{
			name:      "literal true",
			condition: "true",
			context:   map[string]any{},
			want:      true,
		},
		{
			name:      "literal false",
			condition: "false",
			context:   map[string]any{},
			want:      false,
		},
		{
			name:      "literal 1",
			condition: "1",
			context:   map[string]any{},
			want:      true,
		},
		{
			name:      "literal 0",
			condition: "0",
			context:   map[string]any{},
			want:      false,
		},
		{
			name:      "literal yes",
			condition: "yes",
			context:   map[string]any{},
			want:      true,
		},
		{
			name:      "literal no",
			condition: "no",
			context:   map[string]any{},
			want:      false,
		},
		{
			name:      "template producing empty string is falsy",
			condition: `{{ "" }}`,
			context:   map[string]any{},
			want:      false,
		},
		{
			name:      "template eq stage prod - matches",
			condition: `{{ eq .stage "prod" }}`,
			context:   map[string]any{"stage": "prod"},
			want:      true,
		},
		{
			name:      "template eq stage prod - no match",
			condition: `{{ eq .stage "prod" }}`,
			context:   map[string]any{"stage": "dev"},
			want:      false,
		},
		{
			name:      "template vars.pci_scope - true",
			condition: `{{ .vars.pci_scope }}`,
			context:   map[string]any{"vars": map[string]any{"pci_scope": true}},
			want:      true,
		},
		{
			name:      "template vars.pci_scope - false",
			condition: `{{ .vars.pci_scope }}`,
			context:   map[string]any{"vars": map[string]any{"pci_scope": false}},
			want:      false,
		},
		{
			name:      "template vars.pci_scope - string true",
			condition: `{{ .vars.pci_scope }}`,
			context:   map[string]any{"vars": map[string]any{"pci_scope": "true"}},
			want:      true,
		},
		{
			name:      "non-boolean result returns error",
			condition: `{{ .stage }}`,
			context:   map[string]any{"stage": "prod"},
			wantErr:   true,
		},
		{
			// An undefined function causes ProcessTmpl to return a parse error.
			// This exercises the error propagation path from ProcessTmpl.
			name:      "template parse error propagated",
			condition: `{{ undefinedFunction . }}`,
			context:   map[string]any{},
			wantErr:   true,
		},
		{
			name:      "template TRUE uppercase",
			condition: "TRUE",
			context:   map[string]any{},
			want:      true,
		},
		{
			name:      "whitespace trimmed",
			condition: "  true  ",
			context:   map[string]any{},
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evaluateImportCondition(atmosConfig, tt.condition, tt.context)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestBuildImportIfContext tests that buildImportIfContext promotes vars correctly.
func TestBuildImportIfContext(t *testing.T) {
	t.Run("promotes vars from stackConfigMap to top level", func(t *testing.T) {
		stackCfg := map[string]any{
			"vars": map[string]any{
				"stage":       "prod",
				"tenant":      "platform",
				"environment": "ue2",
				"namespace":   "cp",
				"region":      "us-east-2",
				"pci_scope":   true,
			},
		}
		data := buildImportIfContext(stackCfg, map[string]any{})

		assert.Equal(t, "prod", data["stage"])
		assert.Equal(t, "platform", data["tenant"])
		assert.Equal(t, "ue2", data["environment"])
		assert.Equal(t, "cp", data["namespace"])
		assert.Equal(t, "us-east-2", data["region"])
		// Non-standard var should NOT be promoted.
		_, hasPciScope := data["pci_scope"]
		assert.False(t, hasPciScope)
		// vars key should still be present.
		assert.NotNil(t, data["vars"])
	})

	t.Run("does not override existing top-level values from context", func(t *testing.T) {
		stackCfg := map[string]any{
			"vars": map[string]any{
				"stage": "from-stack",
			},
		}
		ctx := map[string]any{"stage": "override"}
		data := buildImportIfContext(stackCfg, ctx)
		assert.Equal(t, "override", data["stage"])
	})

	t.Run("no vars section", func(t *testing.T) {
		data := buildImportIfContext(map[string]any{"foo": "bar"}, map[string]any{})
		_, hasStage := data["stage"]
		assert.False(t, hasStage)
	})

	t.Run("falls back to context vars when stackConfigMap has no vars", func(t *testing.T) {
		ctx := map[string]any{
			"vars": map[string]any{
				"stage": "dev",
			},
		}
		data := buildImportIfContext(map[string]any{}, ctx)
		assert.Equal(t, "dev", data["stage"])
	})

	t.Run("includes locals from stackConfigMap", func(t *testing.T) {
		stackCfg := map[string]any{
			"locals": map[string]any{
				"app_name": "myapp",
				"version":  "1.0",
			},
		}
		data := buildImportIfContext(stackCfg, map[string]any{})
		locals, ok := data["locals"].(map[string]any)
		require.True(t, ok, "locals should be present in context")
		assert.Equal(t, "myapp", locals["app_name"])
		assert.Equal(t, "1.0", locals["version"])
	})

	t.Run("includes settings from stackConfigMap", func(t *testing.T) {
		stackCfg := map[string]any{
			"settings": map[string]any{
				"region":  "us-east-1",
				"enabled": true,
			},
		}
		data := buildImportIfContext(stackCfg, map[string]any{})
		settings, ok := data["settings"].(map[string]any)
		require.True(t, ok, "settings should be present in context")
		assert.Equal(t, "us-east-1", settings["region"])
		assert.Equal(t, true, settings["enabled"])
	})

	t.Run("includes both locals and settings with vars", func(t *testing.T) {
		stackCfg := map[string]any{
			"vars": map[string]any{
				"stage": "prod",
			},
			"locals": map[string]any{
				"app": "myapp",
			},
			"settings": map[string]any{
				"feature_flag": true,
			},
		}
		data := buildImportIfContext(stackCfg, map[string]any{})
		assert.Equal(t, "prod", data["stage"])
		locals, ok := data["locals"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "myapp", locals["app"])
		settings, ok := data["settings"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, true, settings["feature_flag"])
	})
}

// TestProcessImportSectionWithImportIf tests that import_if is parsed from the import object.
func TestProcessImportSectionWithImportIf(t *testing.T) {
	stackMap := map[string]any{
		"import": []any{
			map[string]any{
				"path":      "catalog/vpc/defaults",
				"import_if": `{{ eq .stage "prod" }}`,
			},
			"catalog/always",
			map[string]any{
				"path": "catalog/no-condition",
			},
		},
	}

	imports, err := ProcessImportSection(stackMap, "stacks/orgs/cp/prod.yaml")
	require.NoError(t, err)
	require.Len(t, imports, 3)

	assert.Equal(t, "catalog/vpc/defaults", imports[0].Path)
	assert.Equal(t, `{{ eq .stage "prod" }}`, imports[0].ImportIf)

	assert.Equal(t, "catalog/always", imports[1].Path)
	assert.Empty(t, imports[1].ImportIf)

	assert.Equal(t, "catalog/no-condition", imports[2].Path)
	assert.Empty(t, imports[2].ImportIf)
}

// TestImportIfEndToEnd tests that conditional imports are skipped or included based on the condition,
// exercising the full processYAMLConfigFileWithContextInternal stack processing path.
func TestImportIfEndToEnd(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath:               tempDir,
		StacksBaseAbsolutePath: tempDir,
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
			},
		},
		Logs: schema.Logs{Level: "Info"},
	}

	// Write the catalog file that should only be imported when stage == prod.
	prodCatalogContent := `
components:
  terraform:
    flow-logs:
      vars:
        enabled: true
`
	alwaysContent := `
components:
  terraform:
    vpc:
      vars:
        cidr: "10.0.0.0/16"
`

	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "catalog", "vpc"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "catalog", "vpc", "flow-logs.yaml"), []byte(prodCatalogContent), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "catalog", "vpc", "defaults.yaml"), []byte(alwaysContent), 0o600))

	// Write a prod stack that has the conditional import.
	prodStack := `
vars:
  stage: prod

import:
  - catalog/vpc/defaults
  - path: catalog/vpc/flow-logs
    import_if: "{{ eq .stage \"prod\" }}"
`
	// Write a dev stack that should NOT import flow-logs.
	devStack := `
vars:
  stage: dev

import:
  - catalog/vpc/defaults
  - path: catalog/vpc/flow-logs
    import_if: "{{ eq .stage \"prod\" }}"
`

	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "prod.yaml"), []byte(prodStack), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "dev.yaml"), []byte(devStack), 0o600))

	t.Run("prod stack includes flow-logs", func(t *testing.T) {
		result, _, _, _, _, _, _, _, err := processYAMLConfigFileWithContextInternal(
			atmosConfig, tempDir,
			filepath.Join(tempDir, "prod.yaml"),
			map[string]map[string]any{},
			map[string]any{},
			false, false, false, false,
			map[string]any{}, map[string]any{}, map[string]any{}, map[string]any{},
			"", nil,
		)
		require.NoError(t, err)

		components, ok := result["components"].(map[string]any)
		require.True(t, ok, "expected components section")
		terraform, ok := components["terraform"].(map[string]any)
		require.True(t, ok, "expected terraform section")

		_, hasFlowLogs := terraform["flow-logs"]
		assert.True(t, hasFlowLogs, "prod stack should include flow-logs component")
		_, hasVPC := terraform["vpc"]
		assert.True(t, hasVPC, "prod stack should include vpc component")
	})

	t.Run("dev stack excludes flow-logs", func(t *testing.T) {
		result, _, _, _, _, _, _, _, err := processYAMLConfigFileWithContextInternal(
			atmosConfig, tempDir,
			filepath.Join(tempDir, "dev.yaml"),
			map[string]map[string]any{},
			map[string]any{},
			false, false, false, false,
			map[string]any{}, map[string]any{}, map[string]any{}, map[string]any{},
			"", nil,
		)
		require.NoError(t, err)

		components, ok := result["components"].(map[string]any)
		require.True(t, ok, "expected components section")
		terraform, ok := components["terraform"].(map[string]any)
		require.True(t, ok, "expected terraform section")

		_, hasFlowLogs := terraform["flow-logs"]
		assert.False(t, hasFlowLogs, "dev stack should NOT include flow-logs component")
		_, hasVPC := terraform["vpc"]
		assert.True(t, hasVPC, "dev stack should include vpc component")
	})
}

// TestImportIfVarsDotNotation tests that import_if can reference .vars.key notation.
func TestImportIfVarsDotNotation(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath:               tempDir,
		StacksBaseAbsolutePath: tempDir,
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{Enabled: true},
		},
		Logs: schema.Logs{Level: "Info"},
	}

	pciCatalog := `
components:
  terraform:
    pci-audit:
      vars:
        enabled: true
`
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "catalog"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "catalog", "pci.yaml"), []byte(pciCatalog), 0o600))

	// Stack with pci_scope = true.
	pciStack := `
vars:
  stage: prod
  pci_scope: true

import:
  - path: catalog/pci
    import_if: "{{ .vars.pci_scope }}"
`
	// Stack with pci_scope = false.
	noPciStack := `
vars:
  stage: prod
  pci_scope: false

import:
  - path: catalog/pci
    import_if: "{{ .vars.pci_scope }}"
`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "pci.yaml"), []byte(pciStack), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "no-pci.yaml"), []byte(noPciStack), 0o600))

	t.Run("pci_scope=true includes pci catalog", func(t *testing.T) {
		result, _, _, _, _, _, _, _, err := processYAMLConfigFileWithContextInternal(
			atmosConfig, tempDir,
			filepath.Join(tempDir, "pci.yaml"),
			map[string]map[string]any{},
			map[string]any{},
			false, false, false, false,
			map[string]any{}, map[string]any{}, map[string]any{}, map[string]any{},
			"", nil,
		)
		require.NoError(t, err)

		components, ok := result["components"].(map[string]any)
		require.True(t, ok)
		terraform, ok := components["terraform"].(map[string]any)
		require.True(t, ok)
		_, hasPCI := terraform["pci-audit"]
		assert.True(t, hasPCI, "pci_scope=true should include pci-audit")
	})

	t.Run("pci_scope=false excludes pci catalog", func(t *testing.T) {
		result, _, _, _, _, _, _, _, err := processYAMLConfigFileWithContextInternal(
			atmosConfig, tempDir,
			filepath.Join(tempDir, "no-pci.yaml"),
			map[string]map[string]any{},
			map[string]any{},
			false, false, false, false,
			map[string]any{}, map[string]any{}, map[string]any{}, map[string]any{},
			"", nil,
		)
		require.NoError(t, err)

		components := result["components"]
		if components != nil {
			terraform, ok := components.(map[string]any)["terraform"].(map[string]any)
			if ok {
				_, hasPCI := terraform["pci-audit"]
				assert.False(t, hasPCI, "pci_scope=false should NOT include pci-audit")
			}
		}
		// If components is nil, pci-audit was correctly excluded.
	})
}

// TestImportIfInvalidTemplateReturnsError tests that an invalid import_if expression
// causes an error to be returned from processYAMLConfigFileWithContextInternal.
// This covers the error propagation path at the import loop level.
func TestImportIfInvalidTemplateReturnsError(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath:               tempDir,
		StacksBaseAbsolutePath: tempDir,
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{Enabled: true},
		},
		Logs: schema.Logs{Level: "Info"},
	}

	// Create a dummy catalog file so the path exists (import_if error should occur before file I/O).
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "catalog"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "catalog", "dummy.yaml"), []byte("vars: {}"), 0o600))

	// Stack with an import_if that uses an undefined function — causes a template parse error.
	invalidStack := `
vars:
  stage: prod

import:
  - path: catalog/dummy
    import_if: "{{ undefinedFunction . }}"
`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "invalid.yaml"), []byte(invalidStack), 0o600))

	_, _, _, _, _, _, _, _, err := processYAMLConfigFileWithContextInternal(
		atmosConfig, tempDir,
		filepath.Join(tempDir, "invalid.yaml"),
		map[string]map[string]any{},
		map[string]any{},
		false, false, false, false,
		map[string]any{}, map[string]any{}, map[string]any{}, map[string]any{},
		"", nil,
	)
	require.Error(t, err, "expected error when import_if uses an undefined template function")
	assert.Contains(t, err.Error(), "import_if")
}

// TestImportIfWithLocalsAndSettings tests that locals and settings from the stack file
// are available in the import_if template context.
func TestImportIfWithLocalsAndSettings(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath:               tempDir,
		StacksBaseAbsolutePath: tempDir,
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{Enabled: true},
		},
		Logs: schema.Logs{Level: "Info"},
	}

	// Catalog file included only when settings.feature_enabled is true.
	featureCatalog := `
components:
  terraform:
    feature-component:
      vars:
        enabled: true
`
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "catalog"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "catalog", "feature.yaml"), []byte(featureCatalog), 0o600))

	// Stack with settings-based import condition and locals.
	stackWithFeature := `
locals:
  is_enabled: true

vars:
  stage: prod

settings:
  feature_enabled: true

import:
  - path: catalog/feature
    import_if: "{{ .settings.feature_enabled }}"
`
	stackWithoutFeature := `
locals:
  is_enabled: false

vars:
  stage: prod

settings:
  feature_enabled: false

import:
  - path: catalog/feature
    import_if: "{{ .settings.feature_enabled }}"
`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "with-feature.yaml"), []byte(stackWithFeature), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "without-feature.yaml"), []byte(stackWithoutFeature), 0o600))

	t.Run("settings.feature_enabled=true includes catalog", func(t *testing.T) {
		result, _, _, _, _, _, _, _, err := processYAMLConfigFileWithContextInternal(
			atmosConfig, tempDir,
			filepath.Join(tempDir, "with-feature.yaml"),
			map[string]map[string]any{},
			map[string]any{},
			false, false, false, false,
			map[string]any{}, map[string]any{}, map[string]any{}, map[string]any{},
			"", nil,
		)
		require.NoError(t, err)
		components, ok := result["components"].(map[string]any)
		require.True(t, ok, "expected components section")
		terraform, ok := components["terraform"].(map[string]any)
		require.True(t, ok, "expected terraform section")
		_, hasFeature := terraform["feature-component"]
		assert.True(t, hasFeature, "feature-component should be included")
	})

	t.Run("settings.feature_enabled=false excludes catalog", func(t *testing.T) {
		result, _, _, _, _, _, _, _, err := processYAMLConfigFileWithContextInternal(
			atmosConfig, tempDir,
			filepath.Join(tempDir, "without-feature.yaml"),
			map[string]map[string]any{},
			map[string]any{},
			false, false, false, false,
			map[string]any{}, map[string]any{}, map[string]any{}, map[string]any{},
			"", nil,
		)
		require.NoError(t, err)
		// When no imports are processed, components section may be absent or empty.
		components := result["components"]
		if components != nil {
			terraform, ok := components.(map[string]any)["terraform"].(map[string]any)
			if ok {
				_, hasFeature := terraform["feature-component"]
				assert.False(t, hasFeature, "feature-component should be excluded")
			}
		}
	})
}

func TestProcessYAMLConfigFileWithTemplate(t *testing.T) {
	// Create a temporary directory for test files
	tempDir := t.TempDir()

	// Create test AtmosConfiguration
	atmosConfig := &schema.AtmosConfiguration{
		BasePath:               tempDir,
		StacksBaseAbsolutePath: tempDir,
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
			},
		},
		Logs: schema.Logs{
			Level: "Info",
		},
	}

	testCases := []struct {
		name            string
		fileName        string
		fileContent     string
		context         map[string]any
		shouldProcess   bool
		validateContent func(t *testing.T, result map[string]any)
	}{
		{
			name:     "yaml template without context",
			fileName: "test1.yaml.tmpl",
			fileContent: `
components:
  terraform:
    test-component:
      metadata:
        created_at: "{{ now | date "2006-01-02" }}"
      vars:
        static: value
        calculated: {{ add 5 10 }}`,
			context:       nil,
			shouldProcess: true,
			validateContent: func(t *testing.T, result map[string]any) {
				components := result["components"].(map[string]any)
				terraform := components["terraform"].(map[string]any)
				testComponent := terraform["test-component"].(map[string]any)

				metadata := testComponent["metadata"].(map[string]any)
				assert.NotEmpty(t, metadata["created_at"])

				vars := testComponent["vars"].(map[string]any)
				assert.Equal(t, "value", vars["static"])
				assert.Equal(t, 15, vars["calculated"])
			},
		},
		{
			name:     "yml template without context",
			fileName: "test2.yml.tmpl",
			fileContent: `
settings:
  timestamp: "{{ now | date "15:04:05" }}"
  version: "1.0.0"`,
			context:       nil,
			shouldProcess: true,
			validateContent: func(t *testing.T, result map[string]any) {
				settings := result["settings"].(map[string]any)
				assert.NotEmpty(t, settings["timestamp"])
				assert.Equal(t, "1.0.0", settings["version"])
			},
		},
		{
			name:     "plain yaml file with template syntax",
			fileName: "test3.yaml",
			fileContent: `
components:
  terraform:
    test-component:
      vars:
        value: "{{ .should_not_process }}"`,
			context:       nil,
			shouldProcess: false,
			validateContent: func(t *testing.T, result map[string]any) {
				components := result["components"].(map[string]any)
				terraform := components["terraform"].(map[string]any)
				testComponent := terraform["test-component"].(map[string]any)
				vars := testComponent["vars"].(map[string]any)
				// Template syntax should be preserved as-is
				assert.Equal(t, "{{ .should_not_process }}", vars["value"])
			},
		},
		{
			name:     "template with context",
			fileName: "test4.yaml.tmpl",
			fileContent: `
components:
  terraform:
    "{{ .component_name }}":
      vars:
        environment: "{{ .environment }}"
        static: value
        computed: {{ add 1 1 }}`,
			context: map[string]any{
				"component_name": "my-component",
				"environment":    "production",
			},
			shouldProcess: true,
			validateContent: func(t *testing.T, result map[string]any) {
				components := result["components"].(map[string]any)
				terraform := components["terraform"].(map[string]any)
				myComponent := terraform["my-component"].(map[string]any)
				vars := myComponent["vars"].(map[string]any)
				assert.Equal(t, "production", vars["environment"])
				assert.Equal(t, "value", vars["static"])
				assert.Equal(t, 2, vars["computed"])
			},
		},
		{
			name:     "template with empty context",
			fileName: "test5.yaml.tmpl",
			fileContent: `
metadata:
  generated: true
  timestamp: "{{ now | date "2006-01-02" }}"`,
			context:       map[string]any{},
			shouldProcess: true,
			validateContent: func(t *testing.T, result map[string]any) {
				metadata := result["metadata"].(map[string]any)
				assert.Equal(t, true, metadata["generated"])
				assert.NotEmpty(t, metadata["timestamp"])
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Write test file
			filePath := filepath.Join(tempDir, tc.fileName)
			err := os.WriteFile(filePath, []byte(tc.fileContent), 0o644)
			require.NoError(t, err)

			// Process the file
			result, _, _, _, _, _, _, _, err := ProcessYAMLConfigFileWithContext(
				atmosConfig,
				tempDir,
				filePath,
				map[string]map[string]any{},
				tc.context,
				false, // ignoreMissingFiles
				false, // skipTemplatesProcessingInImports
				true,  // ignoreMissingTemplateValues
				false, // skipIfMissing
				nil,   // parentTerraformOverridesInline
				nil,   // parentTerraformOverridesImports
				nil,   // parentHelmfileOverridesInline
				nil,   // parentHelmfileOverridesImports
				"",    // atmosManifestJsonSchemaFilePath
				nil,   // mergeContext
			)

			require.NoError(t, err)
			require.NotNil(t, result)

			// Validate the content
			tc.validateContent(t, result)
		})
	}
}

// TestProcessImportSectionWithTemplates tests that imports correctly identify template files.
func TestProcessImportSectionWithTemplates(t *testing.T) {
	testCases := []struct {
		name            string
		stackMap        map[string]any
		expectedImports []string
		expectedContext []bool // Whether each import should have context
	}{
		{
			name: "mixed template and non-template imports",
			stackMap: map[string]any{
				"import": []any{
					"catalog/config.yaml.tmpl",
					"catalog/static.yaml",
					map[string]any{
						"path": "catalog/dynamic.yaml.tmpl",
						"context": map[string]any{
							"env": "dev",
						},
					},
				},
			},
			expectedImports: []string{
				"catalog/config.yaml.tmpl",
				"catalog/static.yaml",
				"catalog/dynamic.yaml.tmpl",
			},
			expectedContext: []bool{false, false, true},
		},
		{
			name: "all template imports",
			stackMap: map[string]any{
				"import": []any{
					"stacks/base.yaml.tmpl",
					"stacks/networking.yml.tmpl",
				},
			},
			expectedImports: []string{
				"stacks/base.yaml.tmpl",
				"stacks/networking.yml.tmpl",
			},
			expectedContext: []bool{false, false},
		},
		{
			name: "no template imports",
			stackMap: map[string]any{
				"import": []any{
					"stacks/base.yaml",
					"stacks/networking.yml",
				},
			},
			expectedImports: []string{
				"stacks/base.yaml",
				"stacks/networking.yml",
			},
			expectedContext: []bool{false, false},
		},
		{
			name: "empty import array - allows clearing inherited imports",
			stackMap: map[string]any{
				"import": []any{},
			},
			expectedImports: []string{},
			expectedContext: []bool{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			imports, err := ProcessImportSection(tc.stackMap, "test.yaml")
			require.NoError(t, err)
			require.Len(t, imports, len(tc.expectedImports))

			for i, imp := range imports {
				assert.Equal(t, tc.expectedImports[i], imp.Path)
				if tc.expectedContext[i] {
					assert.NotNil(t, imp.Context)
				}
			}
		})
	}
}

// TestTemplateFileDetectionIntegration tests end-to-end template file detection and processing.
func TestTemplateFileDetectionIntegration(t *testing.T) {
	tempDir := t.TempDir()

	// Create test files with different extensions
	testFiles := map[string]string{
		"stack.yaml": `
import:
  - catalog/base.yaml.tmpl
components:
  terraform:
    vpc:
      vars:
        name: main-vpc`,

		"catalog/base.yaml.tmpl": `
components:
  terraform:
    vpc:
      vars:
        created: "{{ now | date "2006-01-02" }}"
        region: us-east-1`,

		"catalog/static.yaml": `
components:
  terraform:
    vpc:
      vars:
        template_syntax: "{{ .should_not_process }}"`,
	}

	// Write all test files
	for path, content := range testFiles {
		fullPath := filepath.Join(tempDir, path)
		dir := filepath.Dir(fullPath)
		err := os.MkdirAll(dir, 0o755)
		require.NoError(t, err)
		err = os.WriteFile(fullPath, []byte(content), 0o644)
		require.NoError(t, err)
	}

	atmosConfig := &schema.AtmosConfiguration{
		BasePath:               tempDir,
		StacksBaseAbsolutePath: tempDir,
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
			},
		},
		Logs: schema.Logs{
			Level: "Info",
		},
	}

	// Process the main stack file
	stackPath := filepath.Join(tempDir, "stack.yaml")
	result, _, _, _, _, _, _, _, err := ProcessYAMLConfigFileWithContext( //nolint:dogsled
		atmosConfig,
		tempDir,
		stackPath,
		map[string]map[string]any{},
		nil,
		false, // ignoreMissingFiles
		false, // skipTemplatesProcessingInImports
		true,  // ignoreMissingTemplateValues
		false, // skipIfMissing
		nil,
		nil,
		nil,
		nil,
		"",
		nil,
	)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Check that the import was processed
	imports := result["import"]
	assert.NotNil(t, imports)

	// Verify components section exists
	components, ok := result["components"].(map[string]any)
	require.True(t, ok)
	terraform, ok := components["terraform"].(map[string]any)
	require.True(t, ok)
	vpc, ok := terraform["vpc"].(map[string]any)
	require.True(t, ok)
	vars, ok := vpc["vars"].(map[string]any)
	require.True(t, ok)

	// Check that the non-template values are preserved
	assert.Equal(t, "main-vpc", vars["name"])
}

// TestTemplateProcessingWithSkipFlag tests that skip_templates_processing flag still works.
func TestTemplateProcessingWithSkipFlag(t *testing.T) {
	tempDir := t.TempDir()

	// Create a template file with valid YAML even when not processed
	templateFile := filepath.Join(tempDir, "test.yaml.tmpl")
	content := `
components:
  terraform:
    test:
      vars:
        computed: "{{ add 1 2 }}"
        value: 10`

	err := os.WriteFile(templateFile, []byte(content), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath:               tempDir,
		StacksBaseAbsolutePath: tempDir,
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
			},
		},
		Logs: schema.Logs{
			Level: "Info",
		},
	}

	// Test with skipTemplatesProcessingInImports = true
	result, _, _, _, _, _, _, _, err := ProcessYAMLConfigFileWithContext( //nolint:dogsled
		atmosConfig,
		tempDir,
		templateFile,
		map[string]map[string]any{},
		nil,
		false,
		true,
		true,
		false,
		nil,
		nil,
		nil,
		nil,
		"",
		nil,
	)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Template should NOT be processed
	components := result["components"].(map[string]any)
	terraform := components["terraform"].(map[string]any)
	test := terraform["test"].(map[string]any)
	vars := test["vars"].(map[string]any)

	// The template syntax should be preserved as a string
	assert.Equal(t, "{{ add 1 2 }}", vars["computed"])
	assert.Equal(t, 10, vars["value"])

	// Test with skipTemplatesProcessingInImports = false
	result2, _, _, _, _, _, _, _, err2 := ProcessYAMLConfigFileWithContext( //nolint:dogsled
		atmosConfig,
		tempDir,
		templateFile,
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
		nil,
	)

	require.NoError(t, err2)
	require.NotNil(t, result2)

	// Template SHOULD be processed
	components2 := result2["components"].(map[string]any)
	terraform2 := components2["terraform"].(map[string]any)
	test2 := terraform2["test"].(map[string]any)
	vars2 := test2["vars"].(map[string]any)

	// The template should be evaluated
	assert.Equal(t, "3", vars2["computed"])
	assert.Equal(t, 10, vars2["value"])
}

// TestGlobalIgnoreMissingTemplateValues tests that the global templates.settings.ignore_missing_template_values
// setting in atmos.yaml is used as a fallback when per-import ignore_missing_template_values is not set.
func TestGlobalIgnoreMissingTemplateValues(t *testing.T) {
	tempDir := t.TempDir()

	// Create a main stack file that imports a catalog file with context but missing template vars.
	mainStack := `
import:
  - path: catalog/component.yaml
    context:
      flavor: blue
`

	// The catalog file uses a template variable {{ .undeclared_var }} which is not in the context.
	catalogFile := `
components:
  terraform:
    "{{ .flavor }}/cluster":
      vars:
        flavor: "{{ .flavor }}"
        extra: "{{ .undeclared_var }}"
`

	// Write test files.
	err := os.MkdirAll(filepath.Join(tempDir, "catalog"), 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tempDir, "stack.yaml"), []byte(mainStack), 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tempDir, "catalog", "component.yaml"), []byte(catalogFile), 0o644)
	require.NoError(t, err)

	// Test 1: Without the global setting, missing template values should cause an error.
	atmosConfigNoIgnore := &schema.AtmosConfiguration{
		BasePath:               tempDir,
		StacksBaseAbsolutePath: tempDir,
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled:                     true,
				IgnoreMissingTemplateValues: false,
			},
		},
		Logs: schema.Logs{Level: "Info"},
	}

	stackPath := filepath.Join(tempDir, "stack.yaml")
	_, _, _, _, _, _, _, _, err = ProcessYAMLConfigFileWithContext( //nolint:dogsled
		atmosConfigNoIgnore,
		tempDir,
		stackPath,
		map[string]map[string]any{},
		nil,
		false, // ignoreMissingFiles
		false, // skipTemplatesProcessingInImports
		false, // ignoreMissingTemplateValues (import-level)
		false, // skipIfMissing
		nil,
		nil,
		nil,
		nil,
		"",
		nil,
	)
	assert.Error(t, err, "expected an error when ignore_missing_template_values is false and template vars are missing")

	// Test 2: With the global setting enabled, missing template values should not cause an error.
	atmosConfigWithIgnore := &schema.AtmosConfiguration{
		BasePath:               tempDir,
		StacksBaseAbsolutePath: tempDir,
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled:                     true,
				IgnoreMissingTemplateValues: true, // Global setting.
			},
		},
		Logs: schema.Logs{Level: "Info"},
	}

	result, _, _, _, _, _, _, _, err := ProcessYAMLConfigFileWithContext( //nolint:dogsled
		atmosConfigWithIgnore,
		tempDir,
		stackPath,
		map[string]map[string]any{},
		nil,
		false, // ignoreMissingFiles
		false, // skipTemplatesProcessingInImports
		false, // ignoreMissingTemplateValues (import-level, not set; global should apply)
		false, // skipIfMissing
		nil,
		nil,
		nil,
		nil,
		"",
		nil,
	)
	require.NoError(t, err, "expected no error when global ignore_missing_template_values is true")
	require.NotNil(t, result)

	// Verify the component was created with the available template values.
	components, ok := result["components"].(map[string]any)
	require.True(t, ok)
	terraform, ok := components["terraform"].(map[string]any)
	require.True(t, ok)
	cluster, ok := terraform["blue/cluster"].(map[string]any)
	require.True(t, ok)
	vars, ok := cluster["vars"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "blue", vars["flavor"])
}
