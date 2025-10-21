package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"
)

// TestTestCaseSchemaValidation validates all test-case YAML files against schema.json.
func TestTestCaseSchemaValidation(t *testing.T) {
	schemaPath := "test-cases/schema.json"
	testCasesDir := "test-cases"

	// Load the schema.
	schemaData, err := os.ReadFile(schemaPath)
	require.NoError(t, err, "Failed to read schema file")

	schemaLoader := gojsonschema.NewBytesLoader(schemaData)

	// Find all YAML files in test-cases directory.
	files, err := filepath.Glob(filepath.Join(testCasesDir, "*.yaml"))
	require.NoError(t, err, "Failed to find test case files")
	require.NotEmpty(t, files, "No test case YAML files found")

	// Validate each YAML file.
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			// Read YAML file.
			yamlData, err := os.ReadFile(file)
			require.NoError(t, err, "Failed to read test case file: %s", file)

			// Convert YAML to generic structure.
			var data interface{}
			err = yaml.Unmarshal(yamlData, &data)
			require.NoError(t, err, "Failed to parse YAML: %s", file)

			// Create document loader.
			documentLoader := gojsonschema.NewGoLoader(data)

			// Validate.
			result, err := gojsonschema.Validate(schemaLoader, documentLoader)
			require.NoError(t, err, "Schema validation failed with error for %s", file)

			// Check validation result.
			if !result.Valid() {
				t.Errorf("Test case file %s is invalid according to schema:", file)
				for _, err := range result.Errors() {
					t.Errorf("  - %s: %s", err.Field(), err.Description())
				}
			}
		})
	}
}
