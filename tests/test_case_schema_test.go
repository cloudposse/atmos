package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/validator"
)

// TestTestCaseSchemaValidation validates all test-case YAML files against test-cases/schema.json.
//
// It dogfoods Atmos's own schema validator (`pkg/validator`, the same engine behind
// `atmos validate schema`) instead of calling a third-party JSON Schema library directly,
// so the test-case schema is exercised through the exact code path users rely on.
func TestTestCaseSchemaValidation(t *testing.T) {
	av := validator.NewYAMLSchemaValidator(&schema.AtmosConfiguration{})

	schemaPath, err := filepath.Abs(filepath.Join("test-cases", "schema.json"))
	require.NoError(t, err, "Failed to resolve schema file")
	require.FileExists(t, schemaPath, "test-cases/schema.json must exist")

	// Find all YAML files in test-cases directory.
	files, err := filepath.Glob(filepath.Join("test-cases", "*.yaml"))
	require.NoError(t, err, "Failed to find test case files")
	require.NotEmpty(t, files, "No test case YAML files found")

	// Validate each YAML file through Atmos's validator.
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			abs, err := filepath.Abs(file)
			require.NoError(t, err, "Failed to resolve path for %s", file)

			errs, err := av.ValidateYAMLSchema(schemaPath, abs)
			require.NoError(t, err, "Schema validation failed with error for %s", file)

			if len(errs) > 0 {
				t.Errorf("Test case file %s is invalid according to schema:", file)
				for _, e := range errs {
					t.Errorf("  - %s: %s", e.Field(), e.Description())
				}
			}
		})
	}

	// Negative: a deliberately schema-invalid document must produce validation errors,
	// proving the dogfooded validation actually has teeth (it is not a silent no-op).
	t.Run("negative/schema-invalid document is rejected", func(t *testing.T) {
		// `tests` must be a list of test objects per schema.json; a scalar violates it.
		bad := filepath.Join(t.TempDir(), "bad.yaml")
		require.NoError(t, os.WriteFile(bad, []byte("tests: \"must be a list of test objects\"\n"), 0o644))

		errs, err := av.ValidateYAMLSchema(schemaPath, bad)
		require.NoError(t, err, "validation must run without an engine error")
		require.NotEmpty(t, errs, "a schema-invalid document must yield validation errors")
	})
}
