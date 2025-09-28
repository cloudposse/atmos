package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
