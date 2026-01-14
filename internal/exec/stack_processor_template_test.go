package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestProcessYAMLConfigFileWithTemplate tests that template files are processed based on their extension.
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
