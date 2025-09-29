package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Test the functions of validate_component.go.

func TestFindValidationSection(t *testing.T) {
	tests := []struct {
		name             string
		componentSection map[string]any
		expectedCount    int
		expectError      bool
		errorMsg         string
	}{
		{
			name: "valid validation section",
			componentSection: map[string]any{
				"settings": map[string]any{
					"validation": map[string]any{
						"test-validation": map[string]any{
							"schema_type": "jsonschema",
							"schema_path": "test.json",
							"timeout":     30,
							"disabled":    false,
						},
					},
				},
			},
			expectedCount: 1,
			expectError:   false,
		},
		{
			name: "multiple validations",
			componentSection: map[string]any{
				"settings": map[string]any{
					"validation": map[string]any{
						"test-validation-1": map[string]any{
							"schema_type": "jsonschema",
							"schema_path": "test1.json",
						},
						"test-validation-2": map[string]any{
							"schema_type": "opa",
							"schema_path": "test2.rego",
						},
					},
				},
			},
			expectedCount: 2,
			expectError:   false,
		},
		{
			name: "no validation section",
			componentSection: map[string]any{
				"vars": map[string]any{
					"environment": "dev",
				},
			},
			expectedCount: 0,
			expectError:   false,
		},
		{
			name: "no settings section",
			componentSection: map[string]any{
				"vars": map[string]any{
					"environment": "dev",
				},
			},
			expectedCount: 0,
			expectError:   false,
		},
		{
			name:             "empty component section",
			componentSection: map[string]any{},
			expectedCount:    0,
			expectError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FindValidationSection(tt.componentSection)
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.Len(t, result, tt.expectedCount)
			}
		})
	}
}

func TestValidateComponentInternal(t *testing.T) {
	// Test error cases for validateComponentInternal function.
	tests := []struct {
		name        string
		schemaType  string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "unsupported schema type",
			schemaType:  "invalid",
			expectError: true,
			errorMsg:    "invalid schema type",
		},
		{
			name:        "valid jsonschema type",
			schemaType:  "jsonschema",
			expectError: false, // Will fail on file not found, but schema type is valid
		},
		{
			name:        "valid opa type",
			schemaType:  "opa",
			expectError: false, // Will fail on file not found, but schema type is valid
		},
	}

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: "/tmp",
	}

	componentSection := map[string]any{
		"vars": map[string]any{
			"environment": "dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We expect most tests to fail due to missing files, but we're testing the schema type validation.
			_, err := validateComponentInternal(atmosConfig, componentSection, "nonexistent.json", tt.schemaType, []string{}, 30)
			if tt.expectError && tt.errorMsg == "invalid schema type" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else if !tt.expectError {
				// For valid schema types, we expect a different error (file not found).
				if err != nil {
					assert.NotContains(t, err.Error(), "invalid schema type")
				}
			}
		})
	}
}

func TestValidateComponentInternal_ProcessEnvSection(t *testing.T) {
	// Create temporary directory structure for testing.
	tempDir, err := os.MkdirTemp("", "atmos_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test schema files.
	jsonSchemaDir := filepath.Join(tempDir, "schemas", "jsonschema")
	opaSchemaDir := filepath.Join(tempDir, "schemas", "opa")
	err = os.MkdirAll(jsonSchemaDir, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(opaSchemaDir, 0o755)
	require.NoError(t, err)

	// Create a JSON schema that checks for the process_env section.
	jsonSchemaFile := filepath.Join(jsonSchemaDir, "test.json")
	jsonSchema := `{
		"type": "object",
		"properties": {
			"vars": {
				"type": "object",
				"properties": {
					"environment": {"type": "string"}
				}
			},
			"process_env": {
				"type": "object",
				"description": "Process environment variables"
			}
		},
		"required": ["process_env"]
	}`
	err = os.WriteFile(jsonSchemaFile, []byte(jsonSchema), 0o644)
	require.NoError(t, err)

	// Create an OPA policy that checks for the process_env section.
	opaSchemaFile := filepath.Join(opaSchemaDir, "test.rego")
	opaPolicy := `package atmos

errors["process_env section is missing"] {
    not input.process_env
}

errors["process_env section is empty"] {
    count(input.process_env) == 0
}`
	err = os.WriteFile(opaSchemaFile, []byte(opaPolicy), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Schemas: map[string]any{
			"jsonschema": schema.ResourcePath{
				BasePath: "schemas/jsonschema",
			},
			"opa": schema.ResourcePath{
				BasePath: "schemas/opa",
			},
		},
	}

	// Set some test environment variables.
	originalEnv := os.Environ()
	defer func() {
		// Restore original environment
		os.Clearenv()
		for _, envVar := range originalEnv {
			parts := strings.SplitN(envVar, "=", 2)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()

	// Set specific test environment variables.
	os.Setenv("ATMOS_TEST_VAR", "test_value")
	os.Setenv("ANOTHER_TEST_VAR", "another_value")

	tests := []struct {
		name         string
		schemaType   string
		schemaPath   string
		expectError  bool
		expectEnvVar bool
		envVarKey    string
		envVarValue  string
	}{
		{
			name:         "jsonschema validation with process_env",
			schemaType:   "jsonschema",
			schemaPath:   "test.json",
			expectError:  false, // Should pass since process_env will be added
			expectEnvVar: true,
			envVarKey:    "ATMOS_TEST_VAR",
			envVarValue:  "test_value",
		},
		{
			name:         "opa validation with process_env",
			schemaType:   "opa",
			schemaPath:   "test.rego",
			expectError:  false, // Should pass since process_env will be added
			expectEnvVar: true,
			envVarKey:    "ANOTHER_TEST_VAR",
			envVarValue:  "another_value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fresh component section for each test.
			componentSection := map[string]any{
				"vars": map[string]any{
					"environment": "dev",
				},
			}

			// Call validateComponentInternal which should add process_env section.
			result, err := validateComponentInternal(atmosConfig, componentSection, tt.schemaPath, tt.schemaType, []string{}, 30)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				// The function might still fail due to other validation issues, but we're primarily testing
				// that process_env is added to the component section.

				// Check that process_env section was added to componentSection.
				assert.Contains(t, componentSection, cfg.ProcessEnvSectionName, "process_env section should be added to componentSection")

				processEnv, ok := componentSection[cfg.ProcessEnvSectionName].(map[string]string)
				assert.True(t, ok, "process_env should be a map[string]string")

				if tt.expectEnvVar {
					assert.Contains(t, processEnv, tt.envVarKey, "Expected environment variable should be present")
					assert.Equal(t, tt.envVarValue, processEnv[tt.envVarKey], "Environment variable should have correct value")
				}

				// Log the result for debugging, but don't fail the test if validation fails
				// since we're primarily testing the process_env functionality.
				t.Logf("Validation result: %v, error: %v", result, err)
			}
		})
	}
}

func TestValidateComponentInternal_ProcessEnvSectionContent(t *testing.T) {
	// Test specifically that the process environment section contains expected content.

	// Create a temporary directory for testing.
	tempDir, err := os.MkdirTemp("", "atmos_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a minimal JSON schema that will pass validation.
	jsonSchemaDir := filepath.Join(tempDir, "schemas", "jsonschema")
	err = os.MkdirAll(jsonSchemaDir, 0o755)
	require.NoError(t, err)

	jsonSchemaFile := filepath.Join(jsonSchemaDir, "minimal.json")
	minimalSchema := `{"type": "object"}`
	err = os.WriteFile(jsonSchemaFile, []byte(minimalSchema), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Schemas: map[string]any{
			"jsonschema": schema.ResourcePath{
				BasePath: "schemas/jsonschema",
			},
		},
	}

	// Set specific test environment variables.
	testEnvVars := map[string]string{
		"ATMOS_TEST_COMPONENT": "test-component-value",
		"ATMOS_TEST_STACK":     "test-stack-value",
		"PATH":                 "/usr/bin:/bin", // Common env var that should exist
		"HOME":                 "/home/test",
	}

	// Store original values to restore later.
	originalValues := make(map[string]string)
	for key := range testEnvVars {
		originalValues[key] = os.Getenv(key)
	}
	defer func() {
		for key, value := range originalValues {
			if value == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, value)
			}
		}
	}()

	// Set test environment variables.
	for key, value := range testEnvVars {
		os.Setenv(key, value)
	}

	componentSection := map[string]any{
		"vars": map[string]any{
			"test": "value",
		},
	}

	t.Run("process_env section is populated correctly", func(t *testing.T) {
		// Call validateComponentInternal
		_, _ = validateComponentInternal(atmosConfig, componentSection, "minimal.json", "jsonschema", []string{}, 30)

		// The validation might fail, but we're testing that process_env is added.

		// Verify process_env section was added.
		assert.Contains(t, componentSection, cfg.ProcessEnvSectionName, "process_env section should be added")

		processEnv, ok := componentSection[cfg.ProcessEnvSectionName].(map[string]string)
		assert.True(t, ok, "process_env should be a map[string]string")
		assert.NotEmpty(t, processEnv, "process_env should not be empty")

		// Check that our test environment variables are present.
		for key, expectedValue := range testEnvVars {
			assert.Contains(t, processEnv, key, "Environment variable %s should be present", key)
			assert.Equal(t, expectedValue, processEnv[key], "Environment variable %s should have correct value", key)
		}

		// Check that process_env contains common environment variables.
		// Note: We can't guarantee specific env vars exist, but PATH usually does.
		if pathValue, exists := processEnv["PATH"]; exists {
			assert.NotEmpty(t, pathValue, "PATH environment variable should not be empty")
		}

		t.Logf("Found %d environment variables in process_env section", len(processEnv))
		t.Logf("Sample environment variables: %v", getSampleEnvVars(processEnv, 3))
	})

	t.Run("process_env section is updated on multiple calls", func(t *testing.T) {
		// Create a fresh component section.
		componentSection2 := map[string]any{
			"vars": map[string]any{
				"test": "value2",
			},
		}

		// Set a new environment variable.
		os.Setenv("ATMOS_NEW_TEST_VAR", "new-test-value")
		defer os.Unsetenv("ATMOS_NEW_TEST_VAR")

		// Call validateComponentInternal again
		_, _ = validateComponentInternal(atmosConfig, componentSection2, "minimal.json", "jsonschema", []string{}, 30)

		// Verify process_env section was added and contains the new variable.
		assert.Contains(t, componentSection2, cfg.ProcessEnvSectionName)

		processEnv, ok := componentSection2[cfg.ProcessEnvSectionName].(map[string]string)
		assert.True(t, ok)
		assert.Contains(t, processEnv, "ATMOS_NEW_TEST_VAR", "New environment variable should be present")
		assert.Equal(t, "new-test-value", processEnv["ATMOS_NEW_TEST_VAR"], "New environment variable should have correct value")
	})
}

// Helper function to get a sample of environment variables for logging.
func getSampleEnvVars(envMap map[string]string, count int) map[string]string {
	sample := make(map[string]string)
	i := 0
	for k, v := range envMap {
		if i >= count {
			break
		}
		sample[k] = v
		i++
	}
	return sample
}
