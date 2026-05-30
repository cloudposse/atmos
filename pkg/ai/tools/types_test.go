package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParamType_Constants(t *testing.T) {
	tests := []struct {
		name      string
		paramType ParamType
		expected  string
		reason    string
	}{
		{
			name:      "ParamTypeString",
			paramType: ParamTypeString,
			expected:  "string",
			reason:    "JSON Schema type for strings",
		},
		{
			name:      "ParamTypeInt",
			paramType: ParamTypeInt,
			expected:  "integer",
			reason:    "JSON Schema requires 'integer', not 'int'",
		},
		{
			name:      "ParamTypeBool",
			paramType: ParamTypeBool,
			expected:  "boolean",
			reason:    "JSON Schema requires 'boolean', not 'bool'",
		},
		{
			name:      "ParamTypeArray",
			paramType: ParamTypeArray,
			expected:  "array",
			reason:    "JSON Schema type for arrays",
		},
		{
			name:      "ParamTypeObject",
			paramType: ParamTypeObject,
			expected:  "object",
			reason:    "JSON Schema type for objects",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.paramType), tt.reason)
		})
	}
}

func TestParameter_Structure(t *testing.T) {
	param := Parameter{
		Name:        "test_param",
		Description: "A test parameter",
		Type:        ParamTypeString,
		Required:    true,
		Default:     "default_value",
	}

	assert.Equal(t, "test_param", param.Name)
	assert.Equal(t, "A test parameter", param.Description)
	assert.Equal(t, ParamTypeString, param.Type)
	assert.True(t, param.Required)
	assert.Equal(t, "default_value", param.Default)
}

func TestParameter_IntegerType(t *testing.T) {
	param := Parameter{
		Name:        "count",
		Description: "Number of items",
		Type:        ParamTypeInt,
		Required:    false,
		Default:     10,
	}

	// Verify the type is "integer" for JSON Schema compatibility
	assert.Equal(t, "integer", string(param.Type))
	assert.Equal(t, 10, param.Default)
}

func TestParameter_BooleanType(t *testing.T) {
	param := Parameter{
		Name:        "enabled",
		Description: "Whether feature is enabled",
		Type:        ParamTypeBool,
		Required:    false,
		Default:     true,
	}

	// Verify the type is "boolean" for JSON Schema compatibility
	assert.Equal(t, "boolean", string(param.Type))
	assert.Equal(t, true, param.Default)
}

func TestResult_SuccessWithData(t *testing.T) {
	result := &Result{
		Success: true,
		Output:  "Operation completed successfully",
		Error:   nil,
		Data: map[string]interface{}{
			"id":     123,
			"name":   "test",
			"active": true,
		},
	}

	assert.True(t, result.Success)
	assert.Equal(t, "Operation completed successfully", result.Output)
	assert.Nil(t, result.Error)
	assert.Contains(t, result.Data, "id")
	assert.Equal(t, 123, result.Data["id"])
	assert.Equal(t, "test", result.Data["name"])
	assert.Equal(t, true, result.Data["active"])
}

func TestResult_Failure(t *testing.T) {
	err := assert.AnError
	result := &Result{
		Success: false,
		Output:  "Operation failed",
		Error:   err,
		Data:    nil,
	}

	assert.False(t, result.Success)
	assert.Equal(t, "Operation failed", result.Output)
	assert.Error(t, result.Error)
	assert.Nil(t, result.Data)
}

func TestCategory_Constants(t *testing.T) {
	tests := []struct {
		name     string
		category Category
		expected string
	}{
		{
			name:     "CategoryAtmos",
			category: CategoryAtmos,
			expected: "atmos",
		},
		{
			name:     "CategoryFile",
			category: CategoryFile,
			expected: "file",
		},
		{
			name:     "CategorySystem",
			category: CategorySystem,
			expected: "system",
		},
		{
			name:     "CategoryMCP",
			category: CategoryMCP,
			expected: "mcp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.category))
		})
	}
}

// TestParameterTypes_JSONSchemaCompliance verifies all parameter types match JSON Schema spec.
// This is critical for AI provider compatibility (Anthropic, OpenAI, Google, etc.).
func TestParameterTypes_JSONSchemaCompliance(t *testing.T) {
	// Valid JSON Schema types from draft 2020-12
	validTypes := map[string]bool{
		"string":  true,
		"integer": true, // NOT "int"
		"number":  true,
		"boolean": true, // NOT "bool"
		"array":   true,
		"object":  true,
		"null":    true,
	}

	types := []ParamType{
		ParamTypeString,
		ParamTypeInt,
		ParamTypeBool,
		ParamTypeArray,
		ParamTypeObject,
	}

	for _, paramType := range types {
		t.Run(string(paramType), func(t *testing.T) {
			typeStr := string(paramType)
			assert.True(t, validTypes[typeStr],
				"Parameter type '%s' is not a valid JSON Schema type. "+
					"Valid types are: string, integer, number, boolean, array, object, null",
				typeStr)
		})
	}
}

// TestParameterTypes_CommonMistakes ensures we don't use invalid type names.
func TestParameterTypes_CommonMistakes(t *testing.T) {
	invalidTypes := []string{"int", "bool", "str", "dict", "list"}

	for _, invalidType := range invalidTypes {
		t.Run("reject_"+invalidType, func(t *testing.T) {
			// Verify none of our ParamType constants match invalid types
			assert.NotEqual(t, invalidType, string(ParamTypeString), "Should not use '%s'", invalidType)
			assert.NotEqual(t, invalidType, string(ParamTypeInt), "Should not use '%s'", invalidType)
			assert.NotEqual(t, invalidType, string(ParamTypeBool), "Should not use '%s'", invalidType)
			assert.NotEqual(t, invalidType, string(ParamTypeArray), "Should not use '%s'", invalidType)
			assert.NotEqual(t, invalidType, string(ParamTypeObject), "Should not use '%s'", invalidType)
		})
	}
}
