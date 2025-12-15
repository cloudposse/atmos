package exec

import (
	"os"
	"path/filepath"
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
	tempDir := t.TempDir()

	// Create test schema files.
	jsonSchemaDir := filepath.Join(tempDir, "schemas", "jsonschema")
	opaSchemaDir := filepath.Join(tempDir, "schemas", "opa")
	err := os.MkdirAll(jsonSchemaDir, 0o755)
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

	// Set specific test environment variables using t.Setenv for automatic cleanup.
	t.Setenv("ATMOS_TEST_VAR", "test_value")
	t.Setenv("ANOTHER_TEST_VAR", "another_value")

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
	tempDir := t.TempDir()

	// Create a minimal JSON schema that will pass validation.
	jsonSchemaDir := filepath.Join(tempDir, "schemas", "jsonschema")
	err := os.MkdirAll(jsonSchemaDir, 0o755)
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

	// Set test environment variables using t.Setenv for automatic cleanup.
	for key, value := range testEnvVars {
		t.Setenv(key, value)
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

		// Set a new environment variable using t.Setenv for automatic cleanup.
		t.Setenv("ATMOS_NEW_TEST_VAR", "new-test-value")

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

func TestValidateComponent(t *testing.T) {
	// Create temporary directory structure for testing.
	tempDir := t.TempDir()

	// Create test schema files.
	jsonSchemaDir := filepath.Join(tempDir, "schemas", "jsonschema")
	opaSchemaDir := filepath.Join(tempDir, "schemas", "opa")
	err := os.MkdirAll(jsonSchemaDir, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(opaSchemaDir, 0o755)
	require.NoError(t, err)

	// Create JSON schema files.
	validJsonSchemaFile := filepath.Join(jsonSchemaDir, "valid.json")
	validJsonSchema := `{"type": "object"}`
	err = os.WriteFile(validJsonSchemaFile, []byte(validJsonSchema), 0o644)
	require.NoError(t, err)

	strictJsonSchemaFile := filepath.Join(jsonSchemaDir, "strict.json")
	strictJsonSchema := `{
		"type": "object",
		"properties": {
			"vars": {
				"type": "object",
				"properties": {
					"required_var": {"type": "string"}
				},
				"required": ["required_var"]
			}
		},
		"required": ["vars"]
	}`
	err = os.WriteFile(strictJsonSchemaFile, []byte(strictJsonSchema), 0o644)
	require.NoError(t, err)

	// Create OPA policy files.
	validOpaFile := filepath.Join(opaSchemaDir, "valid.rego")
	validOpaPolicy := `package atmos

# This policy always passes - it only adds errors if this impossible condition is met
errors["this will never happen"] {
    input.vars.this_impossible_var == "impossible_value"
}`
	err = os.WriteFile(validOpaFile, []byte(validOpaPolicy), 0o644)
	require.NoError(t, err)

	strictOpaFile := filepath.Join(opaSchemaDir, "strict.rego")
	strictOpaPolicy := `package atmos

errors["vars section is missing"] {
    not input.vars
}

errors["required_var is missing"] {
    not input.vars.required_var
}`
	err = os.WriteFile(strictOpaFile, []byte(strictOpaPolicy), 0o644)
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

	tests := []struct {
		name             string
		componentName    string
		componentSection map[string]any
		schemaPath       string
		schemaType       string
		modulePaths      []string
		timeoutSeconds   int
		expectedResult   bool
		expectError      bool
		errorContains    string
	}{
		{
			name:          "direct validation with valid JSON schema",
			componentName: "test-component",
			componentSection: map[string]any{
				"vars": map[string]any{
					"environment": "dev",
				},
			},
			schemaPath:     "valid.json",
			schemaType:     "jsonschema",
			modulePaths:    []string{},
			timeoutSeconds: 30,
			expectedResult: true,
			expectError:    false,
		},
		{
			name:          "direct validation with strict JSON schema - missing required field",
			componentName: "test-component",
			componentSection: map[string]any{
				"vars": map[string]any{
					"environment": "dev",
				},
			},
			schemaPath:     "strict.json",
			schemaType:     "jsonschema",
			modulePaths:    []string{},
			timeoutSeconds: 30,
			expectedResult: false,
			expectError:    true, // JSON Schema validation returns an error when it fails
		},
		{
			name:          "direct validation with strict JSON schema - with required field",
			componentName: "test-component",
			componentSection: map[string]any{
				"vars": map[string]any{
					"environment":  "dev",
					"required_var": "test-value",
				},
			},
			schemaPath:     "strict.json",
			schemaType:     "jsonschema",
			modulePaths:    []string{},
			timeoutSeconds: 30,
			expectedResult: true,
			expectError:    false,
		},
		{
			name:          "direct validation with valid OPA policy",
			componentName: "test-component",
			componentSection: map[string]any{
				"vars": map[string]any{
					"environment": "dev",
				},
			},
			schemaPath:     "valid.rego",
			schemaType:     "opa",
			modulePaths:    []string{},
			timeoutSeconds: 30,
			expectedResult: true,
			expectError:    false,
		},
		{
			name:          "direct validation with strict OPA policy - missing required field",
			componentName: "test-component",
			componentSection: map[string]any{
				"vars": map[string]any{
					"environment": "dev",
				},
			},
			schemaPath:     "strict.rego",
			schemaType:     "opa",
			modulePaths:    []string{},
			timeoutSeconds: 30,
			expectedResult: false,
			expectError:    true, // OPA validation returns an error when it fails
		},
		{
			name:          "direct validation with strict OPA policy - with required field",
			componentName: "test-component",
			componentSection: map[string]any{
				"vars": map[string]any{
					"environment":  "dev",
					"required_var": "test-value",
				},
			},
			schemaPath:     "strict.rego",
			schemaType:     "opa",
			modulePaths:    []string{},
			timeoutSeconds: 30,
			expectedResult: true,
			expectError:    false,
		},
		{
			name:          "validation using component settings - single validation",
			componentName: "test-component",
			componentSection: map[string]any{
				"vars": map[string]any{
					"environment": "dev",
				},
				"settings": map[string]any{
					"validation": map[string]any{
						"test-validation": map[string]any{
							"schema_type": "jsonschema",
							"schema_path": "valid.json",
							"timeout":     30,
							"disabled":    false,
						},
					},
				},
			},
			schemaPath:     "", // Empty to trigger settings-based validation
			schemaType:     "",
			modulePaths:    []string{},
			timeoutSeconds: 0,
			expectedResult: true,
			expectError:    false,
		},
		{
			name:          "validation using component settings - multiple validations",
			componentName: "test-component",
			componentSection: map[string]any{
				"vars": map[string]any{
					"environment":  "dev",
					"required_var": "test-value",
				},
				"settings": map[string]any{
					"validation": map[string]any{
						"json-validation": map[string]any{
							"schema_type": "jsonschema",
							"schema_path": "strict.json",
							"timeout":     30,
							"disabled":    false,
						},
						"opa-validation": map[string]any{
							"schema_type": "opa",
							"schema_path": "strict.rego",
							"timeout":     30,
							"disabled":    false,
						},
					},
				},
			},
			schemaPath:     "",
			schemaType:     "",
			modulePaths:    []string{},
			timeoutSeconds: 0,
			expectedResult: true,
			expectError:    false,
		},
		{
			name:          "validation using component settings - one validation fails",
			componentName: "test-component",
			componentSection: map[string]any{
				"vars": map[string]any{
					"environment": "dev",
					// Missing required_var
				},
				"settings": map[string]any{
					"validation": map[string]any{
						"valid-validation": map[string]any{
							"schema_type": "jsonschema",
							"schema_path": "valid.json",
							"timeout":     30,
							"disabled":    false,
						},
						"strict-validation": map[string]any{
							"schema_type": "jsonschema",
							"schema_path": "strict.json",
							"timeout":     30,
							"disabled":    false,
						},
					},
				},
			},
			schemaPath:     "",
			schemaType:     "",
			modulePaths:    []string{},
			timeoutSeconds: 0,
			expectedResult: false,
			expectError:    true, // One validation fails with an error
		},
		{
			name:          "validation using component settings - disabled validation",
			componentName: "test-component",
			componentSection: map[string]any{
				"vars": map[string]any{
					"environment": "dev",
					// Missing required_var, but validation is disabled
				},
				"settings": map[string]any{
					"validation": map[string]any{
						"disabled-validation": map[string]any{
							"schema_type": "jsonschema",
							"schema_path": "strict.json",
							"timeout":     30,
							"disabled":    true, // This validation is disabled
						},
					},
				},
			},
			schemaPath:     "",
			schemaType:     "",
			modulePaths:    []string{},
			timeoutSeconds: 0,
			expectedResult: true, // Should pass because validation is disabled
			expectError:    false,
		},
		{
			name:          "CLI parameters override component settings",
			componentName: "test-component",
			componentSection: map[string]any{
				"vars": map[string]any{
					"environment": "dev",
				},
				"settings": map[string]any{
					"validation": map[string]any{
						"component-validation": map[string]any{
							"schema_type": "jsonschema",
							"schema_path": "strict.json", // Would fail
							"timeout":     10,
							"disabled":    false,
						},
					},
				},
			},
			schemaPath:     "valid.json", // CLI override to use valid schema
			schemaType:     "jsonschema",
			modulePaths:    []string{},
			timeoutSeconds: 60, // CLI override timeout
			expectedResult: true,
			expectError:    false,
		},
		{
			name:          "no validation configured",
			componentName: "test-component",
			componentSection: map[string]any{
				"vars": map[string]any{
					"environment": "dev",
				},
			},
			schemaPath:     "",
			schemaType:     "",
			modulePaths:    []string{},
			timeoutSeconds: 0,
			expectedResult: true, // No validation configured, should pass
			expectError:    false,
		},
		{
			name:          "invalid schema type error",
			componentName: "test-component",
			componentSection: map[string]any{
				"vars": map[string]any{
					"environment": "dev",
				},
			},
			schemaPath:     "valid.json",
			schemaType:     "invalid-schema-type",
			modulePaths:    []string{},
			timeoutSeconds: 30,
			expectedResult: false,
			expectError:    true,
			errorContains:  "invalid schema type",
		},
		{
			name:          "missing schema file error",
			componentName: "test-component",
			componentSection: map[string]any{
				"vars": map[string]any{
					"environment": "dev",
				},
			},
			schemaPath:     "nonexistent.json",
			schemaType:     "jsonschema",
			modulePaths:    []string{},
			timeoutSeconds: 30,
			expectedResult: false,
			expectError:    true,
			errorContains:  "does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateComponent(
				atmosConfig,
				tt.componentName,
				tt.componentSection,
				tt.schemaPath,
				tt.schemaType,
				tt.modulePaths,
				tt.timeoutSeconds,
			)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result, "Validation result should match expected")
			}

			// Verify that process_env section was added in cases where validation actually ran.
			// process_env is only added when validateComponentInternal is called.
			if !tt.expectError && (tt.schemaPath != "" || hasValidationSettings(tt.componentSection)) {
				assert.Contains(t, tt.componentSection, cfg.ProcessEnvSectionName, "process_env section should be added when validation runs")
			}
		})
	}
}

// Helper function to check if the component section has validation settings.
func hasValidationSettings(componentSection map[string]any) bool {
	settings, ok := componentSection["settings"].(map[string]any)
	if !ok {
		return false
	}

	validation, ok := settings["validation"].(map[string]any)
	if !ok {
		return false
	}

	// Check if there are any enabled validations
	for _, v := range validation {
		validationItem, ok := v.(map[string]any)
		if !ok {
			continue
		}

		disabled, exists := validationItem["disabled"].(bool)
		if !exists || !disabled {
			return true // Found at least one enabled validation
		}
	}

	return false
}

func TestExecuteValidateComponent(t *testing.T) {
	// Change to the test fixtures directory for stack processing.
	fixturesDir := "../../tests/fixtures/scenarios/complete"
	t.Chdir(fixturesDir)

	tests := []struct {
		name           string
		componentName  string
		stack          string
		schemaPath     string
		schemaType     string
		modulePaths    []string
		timeoutSeconds int
		expectedResult bool
		expectError    bool
		errorContains  string
	}{
		{
			name:           "component validation without schema - should pass",
			componentName:  "test/test-component",
			stack:          "tenant1-ue2-dev",
			schemaPath:     "",
			schemaType:     "",
			modulePaths:    []string{},
			timeoutSeconds: 0,
			expectedResult: true,
			expectError:    false,
		},
		{
			name:           "invalid component name",
			componentName:  "nonexistent-component",
			stack:          "tenant1-ue2-dev",
			schemaPath:     "",
			schemaType:     "",
			modulePaths:    []string{},
			timeoutSeconds: 0,
			expectedResult: false,
			expectError:    true,
			errorContains:  "Could not find the component `nonexistent-component`",
		},
		{
			name:           "invalid stack name",
			componentName:  "test/test-component",
			stack:          "nonexistent-stack",
			schemaPath:     "",
			schemaType:     "",
			modulePaths:    []string{},
			timeoutSeconds: 0,
			expectedResult: false,
			expectError:    true,
			errorContains:  "Could not find the component",
		},
		{
			name:           "invalid schema type",
			componentName:  "test/test-component",
			stack:          "tenant1-ue2-dev",
			schemaPath:     "test.json",
			schemaType:     "invalid-schema-type",
			modulePaths:    []string{},
			timeoutSeconds: 30,
			expectedResult: false,
			expectError:    true,
			errorContains:  "invalid schema type",
		},
		{
			name:           "component type auto-detection - terraform",
			componentName:  "test/test-component",
			stack:          "tenant1-ue2-dev",
			schemaPath:     "",
			schemaType:     "",
			modulePaths:    []string{},
			timeoutSeconds: 0,
			expectedResult: true,
			expectError:    false,
		},
		{
			name:           "component type auto-detection - helmfile",
			componentName:  "echo-server",
			stack:          "tenant1-ue2-dev",
			schemaPath:     "",
			schemaType:     "",
			modulePaths:    []string{},
			timeoutSeconds: 0,
			expectedResult: true,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initialize configuration.
			info := schema.ConfigAndStacksInfo{}
			atmosConfig, err := cfg.InitCliConfig(info, true)
			require.NoError(t, err)

			// Execute the function under test.
			result, err := ExecuteValidateComponent(
				&atmosConfig,
				info,
				tt.componentName,
				tt.stack,
				tt.schemaPath,
				tt.schemaType,
				tt.modulePaths,
				tt.timeoutSeconds,
			)

			// Verify results.
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result, "Validation result should match expected")
			}
		})
	}
}

func TestExecuteValidateComponent_ComponentTypeDetection(t *testing.T) {
	// Test that the function correctly tries different component types in order.

	// Change to the test fixtures directory.
	fixturesDir := "../../tests/fixtures/scenarios/complete"
	t.Chdir(fixturesDir)

	tests := []struct {
		name          string
		componentName string
		stack         string
		expectSuccess bool
	}{
		{
			name:          "terraform component detection",
			componentName: "test/test-component",
			stack:         "tenant1-ue2-dev",
			expectSuccess: true,
		},
		{
			name:          "helmfile component detection",
			componentName: "echo-server",
			stack:         "tenant1-ue2-dev",
			expectSuccess: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := schema.ConfigAndStacksInfo{}
			atmosConfig, err := cfg.InitCliConfig(info, true)
			require.NoError(t, err)

			result, err := ExecuteValidateComponent(
				&atmosConfig,
				info,
				tt.componentName,
				tt.stack,
				"", // No schema to test component type detection only
				"",
				[]string{},
				0,
			)

			if tt.expectSuccess {
				assert.NoError(t, err)
				assert.True(t, result)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestExecuteValidateComponent_WithComponentValidationSettings(t *testing.T) {
	// Test validation using component settings rather than direct parameters.

	// Change to the test fixtures directory.
	fixturesDir := "../../tests/fixtures/scenarios/complete"
	t.Chdir(fixturesDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	// This also disables parent directory search and git root discovery.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	info := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(info, true)
	require.NoError(t, err)

	// Test with empty schema parameters to trigger settings-based validation.
	// Note: This component might have validation settings defined.
	result, err := ExecuteValidateComponent(
		&atmosConfig,
		info,
		"test/test-component",
		"tenant1-ue2-dev",
		"", // Empty to trigger settings-based validation
		"", // Empty to trigger settings-based validation
		[]string{},
		0, // Default timeout
	)

	// The result depends on whether the component has validation settings.
	// If no validation is configured, it should pass.
	if err != nil {
		// If there's an error, it should be a validation error, not a configuration error.
		assert.NotContains(t, err.Error(), "invalid schema type")
		assert.NotContains(t, err.Error(), "component")
	} else {
		// If no error, validation should pass.
		assert.True(t, result)
	}
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
