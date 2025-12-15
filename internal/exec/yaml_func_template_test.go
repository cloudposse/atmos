package exec

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// TestProcessTagTemplate_UnitTests tests the core processTagTemplate function with various inputs.
func TestProcessTagTemplate_UnitTests(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected interface{}
		wantType string
	}{
		{
			name:     "simple string (no JSON)",
			input:    "!template hello-world",
			expected: "hello-world",
			wantType: "string",
		},
		{
			name:     "JSON-encoded string",
			input:    `!template "hello-world"`,
			expected: "hello-world",
			wantType: "string",
		},
		{
			name:     "JSON-encoded number",
			input:    "!template 42",
			expected: float64(42), // JSON numbers decode to float64
			wantType: "number",
		},
		{
			name:     "JSON-encoded boolean true",
			input:    "!template true",
			expected: true,
			wantType: "boolean",
		},
		{
			name:     "JSON-encoded boolean false",
			input:    "!template false",
			expected: false,
			wantType: "boolean",
		},
		{
			name:     "JSON-encoded null",
			input:    "!template null",
			expected: nil,
			wantType: "null",
		},
		{
			name:     "JSON-encoded list",
			input:    `!template ["item-1", "item-2", "item-3"]`,
			expected: []interface{}{"item-1", "item-2", "item-3"},
			wantType: "list",
		},
		{
			name:     "JSON-encoded empty list",
			input:    `!template []`,
			expected: []interface{}{},
			wantType: "list",
		},
		{
			name:  "JSON-encoded map",
			input: `!template {"key1": "value1", "key2": "value2"}`,
			expected: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
			wantType: "map",
		},
		{
			name:     "JSON-encoded empty map",
			input:    `!template {}`,
			expected: map[string]interface{}{},
			wantType: "map",
		},
		{
			name:  "JSON-encoded nested structure",
			input: `!template {"name": "test", "count": 5, "tags": ["tag1", "tag2"], "metadata": {"env": "prod"}}`,
			expected: map[string]interface{}{
				"name":  "test",
				"count": float64(5),
				"tags":  []interface{}{"tag1", "tag2"},
				"metadata": map[string]interface{}{
					"env": "prod",
				},
			},
			wantType: "map",
		},
		{
			name:     "invalid JSON - graceful degradation",
			input:    "!template {this is not valid json}",
			expected: "{this is not valid json}",
			wantType: "string",
		},
		{
			name:     "JSON with escaped quotes",
			input:    `!template {"message": "He said \"hello\""}`,
			expected: map[string]interface{}{"message": `He said "hello"`},
			wantType: "map",
		},
		{
			name:  "JSON with unicode characters",
			input: `!template {"emoji": "🚀", "text": "Hello 世界"}`,
			expected: map[string]interface{}{
				"emoji": "🚀",
				"text":  "Hello 世界",
			},
			wantType: "map",
		},
		{
			name:  "JSON array of objects",
			input: `!template [{"id": 1, "name": "first"}, {"id": 2, "name": "second"}]`,
			expected: []interface{}{
				map[string]interface{}{"id": float64(1), "name": "first"},
				map[string]interface{}{"id": float64(2), "name": "second"},
			},
			wantType: "list",
		},
		{
			name:     "JSON mixed types array",
			input:    `!template [1, "two", true, null, {"five": 5}]`,
			expected: []interface{}{float64(1), "two", true, nil, map[string]interface{}{"five": float64(5)}},
			wantType: "list",
		},
		{
			name:  "JSON deep nested structure",
			input: `!template {"level1": {"level2": {"level3": {"level4": {"value": "deep"}}}}}`,
			expected: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"level3": map[string]interface{}{
							"level4": map[string]interface{}{
								"value": "deep",
							},
						},
					},
				},
			},
			wantType: "map",
		},
		{
			name:     "JSON-encoded empty string",
			input:    `!template ""`,
			expected: "",
			wantType: "string",
		},
		{
			name:     "whitespace before JSON",
			input:    `!template    ["a", "b"]`,
			expected: []interface{}{"a", "b"},
			wantType: "list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processTagTemplate(tt.input)
			assert.Equal(t, tt.expected, result, "Result should match expected value")
		})
	}
}

// TestProcessTagTemplate_TypePreservation tests that types are correctly preserved during JSON decoding.
func TestProcessTagTemplate_TypePreservation(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		validator func(t *testing.T, result interface{})
	}{
		{
			name:  "preserves string type",
			input: `!template "hello"`,
			validator: func(t *testing.T, result interface{}) {
				str, ok := result.(string)
				assert.True(t, ok, "Result should be a string")
				assert.Equal(t, "hello", str)
			},
		},
		{
			name:  "preserves number type",
			input: "!template 42",
			validator: func(t *testing.T, result interface{}) {
				num, ok := result.(float64)
				assert.True(t, ok, "Result should be a float64")
				assert.Equal(t, float64(42), num)
			},
		},
		{
			name:  "preserves boolean type",
			input: "!template true",
			validator: func(t *testing.T, result interface{}) {
				b, ok := result.(bool)
				assert.True(t, ok, "Result should be a bool")
				assert.True(t, b)
			},
		},
		{
			name:  "preserves null as nil",
			input: "!template null",
			validator: func(t *testing.T, result interface{}) {
				assert.Nil(t, result, "Result should be nil")
			},
		},
		{
			name:  "preserves list type",
			input: `!template ["a", "b", "c"]`,
			validator: func(t *testing.T, result interface{}) {
				list, ok := result.([]interface{})
				assert.True(t, ok, "Result should be a slice")
				assert.Len(t, list, 3)
			},
		},
		{
			name:  "preserves map type",
			input: `!template {"key": "value"}`,
			validator: func(t *testing.T, result interface{}) {
				m, ok := result.(map[string]interface{})
				assert.True(t, ok, "Result should be a map")
				assert.Equal(t, "value", m["key"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processTagTemplate(tt.input)
			tt.validator(t, result)
		})
	}
}

// TestProcessTagTemplate_ErrorHandling tests error cases and graceful degradation.
func TestProcessTagTemplate_ErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectStr   string
		description string
	}{
		{
			name:        "invalid JSON returns original string",
			input:       "!template {not valid json}",
			expectStr:   "{not valid json}",
			description: "Should return original string when JSON parsing fails",
		},
		{
			name:        "malformed JSON object",
			input:       `!template {"unclosed": "object"`,
			expectStr:   `{"unclosed": "object"`,
			description: "Should return original string for malformed JSON",
		},
		{
			name:        "malformed JSON array",
			input:       `!template ["unclosed", "array"`,
			expectStr:   `["unclosed", "array"`,
			description: "Should return original string for malformed array",
		},
		{
			name:        "partial JSON",
			input:       "!template {",
			expectStr:   "{",
			description: "Should return original string for partial JSON",
		},
		{
			name:        "random text",
			input:       "!template this is just random text",
			expectStr:   "this is just random text",
			description: "Should return original string for non-JSON text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processTagTemplate(tt.input)
			str, ok := result.(string)
			assert.True(t, ok, "Result should be a string (graceful degradation)")
			assert.Equal(t, tt.expectStr, str, tt.description)
		})
	}
}

// TestYamlFuncTemplate_Integration tests the !template function in a real stack context.
func TestYamlFuncTemplate_Integration(t *testing.T) {
	err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	assert.NoError(t, err, "Failed to unset 'ATMOS_CLI_CONFIG_PATH'")

	err = os.Unsetenv("ATMOS_BASE_PATH")
	assert.NoError(t, err, "Failed to unset 'ATMOS_BASE_PATH'")

	log.SetLevel(log.InfoLevel)
	log.SetOutput(os.Stdout)

	stack := "nonprod"

	// Define the working directory
	workDir := "../../tests/fixtures/scenarios/atmos-template-yaml-function"
	t.Chdir(workDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	// This also disables parent directory search and git root discovery.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	info := schema.ConfigAndStacksInfo{
		StackFromArg:     "",
		Stack:            stack,
		StackFile:        "",
		ComponentType:    "terraform",
		ComponentFromArg: "test-basic-template",
		SubCommand:       "describe",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	_, err = cfg.InitCliConfig(info, true)
	assert.NoError(t, err)

	// Test basic template functionality
	t.Run("basic JSON decoding", func(t *testing.T) {
		res, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
			Component:            "test-basic-template",
			Stack:                stack,
			ProcessTemplates:     true,
			ProcessYamlFunctions: true,
			Skip:                 nil,
			AuthManager:          nil,
		})
		assert.NoError(t, err)

		y, err := u.ConvertToYAML(res)
		assert.NoError(t, err)

		// Verify simple string (no JSON encoding)
		assert.Contains(t, y, "simple_string: hello-world")

		// Verify JSON-encoded primitives are decoded
		assert.Contains(t, y, "json_string: hello-world")
		assert.Contains(t, y, "json_number: 42")
		assert.Contains(t, y, "json_boolean: true")

		// Verify JSON-encoded list is decoded to YAML list
		assert.Contains(t, y, `json_list:
    - item-1
    - item-2
    - item-3`)

		// Verify JSON-encoded map is decoded to YAML map
		assert.Contains(t, y, "key1: value1")
		assert.Contains(t, y, "key2: value2")

		// Verify invalid JSON returns original string
		assert.Contains(t, y, "invalid_json: '{this is not valid json}'")
	})

	// Test template with Go template expressions
	t.Run("template expressions", func(t *testing.T) {
		res, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
			Component:            "test-template-with-expressions",
			Stack:                stack,
			ProcessTemplates:     true,
			ProcessYamlFunctions: true,
			Skip:                 nil,
			AuthManager:          nil,
		})
		assert.NoError(t, err)

		y, err := u.ConvertToYAML(res)
		assert.NoError(t, err)

		// Verify template string
		assert.Contains(t, y, "template_string: hello-world")

		// Verify template list
		assert.Contains(t, y, `template_list:
    - a
    - b
    - c`)

		// Verify stack variable access
		assert.Contains(t, y, "stack_var: nonprod")
	})

	// Test template with atmos.Component() integration
	t.Run("atmos.Component integration", func(t *testing.T) {
		res, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
			Component:            "test-template-with-atmos-component",
			Stack:                stack,
			ProcessTemplates:     true,
			ProcessYamlFunctions: true,
			Skip:                 nil,
			AuthManager:          nil,
		})
		assert.NoError(t, err)

		y, err := u.ConvertToYAML(res)
		assert.NoError(t, err)

		// Verify component string output (no !template needed)
		assert.Contains(t, y, "component_string: hello-world")

		// Verify component list output is decoded to YAML list
		assert.Contains(t, y, `component_list:
    - item-1
    - item-2
    - item-3`)

		// Verify component map output is decoded to YAML map
		assert.Contains(t, y, "key1: value1")
		assert.Contains(t, y, "key2: value2")

		// Verify nested path extraction
		assert.Contains(t, y, `component_nested_path:
    - tag1
    - tag2`)

		// Verify specific value extraction
		assert.Contains(t, y, "component_nested_value: test")
	})

	// Test template in lists
	t.Run("template in lists", func(t *testing.T) {
		res, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
			Component:            "test-template-in-lists",
			Stack:                stack,
			ProcessTemplates:     true,
			ProcessYamlFunctions: true,
			Skip:                 nil,
			AuthManager:          nil,
		})
		assert.NoError(t, err)

		y, err := u.ConvertToYAML(res)
		assert.NoError(t, err)

		// Verify multiple !template results in a list are processed independently
		assert.Contains(t, y, "template_results_list:")

		// Verify mixed list (static + !template)
		assert.Contains(t, y, "static-value-start")
		assert.Contains(t, y, "static-value-middle")
		assert.Contains(t, y, "static-value-end")
	})

	// Test template in maps
	t.Run("template in maps", func(t *testing.T) {
		res, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
			Component:            "test-template-in-maps",
			Stack:                stack,
			ProcessTemplates:     true,
			ProcessYamlFunctions: true,
			Skip:                 nil,
			AuthManager:          nil,
		})
		assert.NoError(t, err)

		y, err := u.ConvertToYAML(res)
		assert.NoError(t, err)

		// Verify map with multiple !template values
		assert.Contains(t, y, "template_results_map:")
		assert.Contains(t, y, "number_result: 42")
	})

	// Test edge cases
	t.Run("edge cases", func(t *testing.T) {
		res, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
			Component:            "test-template-edge-cases",
			Stack:                stack,
			ProcessTemplates:     true,
			ProcessYamlFunctions: true,
			Skip:                 nil,
			AuthManager:          nil,
		})
		assert.NoError(t, err)

		y, err := u.ConvertToYAML(res)
		assert.NoError(t, err)

		// Verify whitespace handling
		assert.Contains(t, y, "whitespace_before:")
		assert.Contains(t, y, "- a")
		assert.Contains(t, y, "- b")

		// Verify empty JSON structures
		assert.Contains(t, y, "empty_array: []")
		assert.Contains(t, y, "empty_object: {}")

		// Verify unicode handling (may be escaped in YAML output)
		assert.True(t, strings.Contains(y, "emoji: 🚀") || strings.Contains(y, "emoji: \"\\U0001F680\"") || strings.Contains(y, `emoji: "\U0001F680"`), "Should contain emoji in some form")
	})
}

// TestYamlFuncTemplate_ReturnTypes tests that !template properly handles different return types.
// Note: This test verifies the function behavior when it IS processed (during stack processing).
func TestYamlFuncTemplate_ReturnTypes(t *testing.T) {
	// Test direct function calls with different return types
	tests := []struct {
		name     string
		input    string
		validate func(t *testing.T, result interface{})
	}{
		{
			name:  "returns list",
			input: `!template ["item-1", "item-2", "item-3"]`,
			validate: func(t *testing.T, result interface{}) {
				list, ok := result.([]interface{})
				assert.True(t, ok, "Should be a list")
				assert.Len(t, list, 3)
			},
		},
		{
			name:  "returns map",
			input: `!template {"key1": "value1", "key2": "value2"}`,
			validate: func(t *testing.T, result interface{}) {
				m, ok := result.(map[string]interface{})
				assert.True(t, ok, "Should be a map")
				assert.Equal(t, "value1", m["key1"])
			},
		},
		{
			name:  "returns string",
			input: `!template "hello-world"`,
			validate: func(t *testing.T, result interface{}) {
				str, ok := result.(string)
				assert.True(t, ok, "Should be a string")
				assert.Equal(t, "hello-world", str)
			},
		},
		{
			name:  "returns number",
			input: "!template 42",
			validate: func(t *testing.T, result interface{}) {
				num, ok := result.(float64)
				assert.True(t, ok, "Should be a number")
				assert.Equal(t, float64(42), num)
			},
		},
		{
			name:  "returns boolean",
			input: "!template true",
			validate: func(t *testing.T, result interface{}) {
				b, ok := result.(bool)
				assert.True(t, ok, "Should be a boolean")
				assert.True(t, b)
			},
		},
		{
			name:  "returns null",
			input: "!template null",
			validate: func(t *testing.T, result interface{}) {
				assert.Nil(t, result, "Should be nil")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processTagTemplate(tt.input)
			tt.validate(t, result)
		})
	}
}

// TestYamlFuncTemplate_WithSkip tests that !template is handled correctly during parsing.
// Note: The actual skipping logic is tested in integration tests.
func TestYamlFuncTemplate_WithSkip(t *testing.T) {
	// This test verifies that the !template function works when called directly
	t.Run("direct function call", func(t *testing.T) {
		result := processTagTemplate(`!template ["a", "b", "c"]`)
		list, ok := result.([]interface{})
		assert.True(t, ok, "Should be a list")
		assert.Len(t, list, 3, "List should have 3 items")
	})

	// Note: Testing with skip parameter requires processCustomYamlTags which is tested
	// in integration tests with ExecuteDescribeComponent
}

// TestYamlFuncTemplate_Documentation tests examples from the documentation.
func TestYamlFuncTemplate_Documentation(t *testing.T) {
	// Test the exact example from the documentation
	t.Run("documentation example - list output", func(t *testing.T) {
		input := `!template ["item-1", "item-2", "item-3"]`
		result := processTagTemplate(input)

		list, ok := result.([]interface{})
		assert.True(t, ok, "Should decode to list")
		assert.Len(t, list, 3)
		assert.Equal(t, "item-1", list[0])
		assert.Equal(t, "item-2", list[1])
		assert.Equal(t, "item-3", list[2])
	})

	t.Run("documentation example - map output", func(t *testing.T) {
		input := `!template {"key": "value"}`
		result := processTagTemplate(input)

		m, ok := result.(map[string]interface{})
		assert.True(t, ok, "Should decode to map")
		assert.Equal(t, "value", m["key"])
	})

	// Test the primary use case: atmos.Component() + toJson + !template
	// Note: Full integration testing of this pattern is in TestYamlFuncTemplate_Integration
	t.Run("documentation pattern - simulated result", func(t *testing.T) {
		// Simulate the result after toJson from atmos.Component() output
		input := `!template ["subnet-abc", "subnet-def", "subnet-ghi"]`
		result := processTagTemplate(input)

		// Verify we get a native list, not a JSON string
		subnetIDs, ok := result.([]interface{})
		assert.True(t, ok, "Should be a list")
		assert.Len(t, subnetIDs, 3)
		assert.Equal(t, "subnet-abc", subnetIDs[0])
		assert.Equal(t, "subnet-def", subnetIDs[1])
		assert.Equal(t, "subnet-ghi", subnetIDs[2])
	})
}

// TestYamlFuncTemplate_Regression tests specific bug fixes and regressions.
func TestYamlFuncTemplate_Regression(t *testing.T) {
	t.Run("empty JSON string edge case", func(t *testing.T) {
		// Regression test: Empty JSON string should decode to empty string, not error
		input := `!template ""`
		result := processTagTemplate(input)
		assert.Equal(t, "", result)
	})

	t.Run("large JSON structure", func(t *testing.T) {
		// Regression test: Large nested structures should work
		input := `!template {"a":{"b":{"c":{"d":{"e":{"f":{"g":"deep"}}}}}}}`
		result := processTagTemplate(input)

		m, ok := result.(map[string]interface{})
		assert.True(t, ok, "Should decode deep structure")

		// Navigate to deepest level using require guards to prevent panics on type mismatch.
		a, ok := m["a"].(map[string]interface{})
		require.True(t, ok, "a should be a map")
		b, ok := a["b"].(map[string]interface{})
		require.True(t, ok, "b should be a map")
		c, ok := b["c"].(map[string]interface{})
		require.True(t, ok, "c should be a map")
		d, ok := c["d"].(map[string]interface{})
		require.True(t, ok, "d should be a map")
		e, ok := d["e"].(map[string]interface{})
		require.True(t, ok, "e should be a map")
		f, ok := e["f"].(map[string]interface{})
		require.True(t, ok, "f should be a map")

		assert.Equal(t, "deep", f["g"])
	})

	t.Run("numbers preserve precision", func(t *testing.T) {
		// Regression test: Numbers should preserve their values
		tests := []struct {
			input    string
			expected float64
		}{
			{"!template 0", 0},
			{"!template 1", 1},
			{"!template -1", -1},
			{"!template 3.14159", 3.14159},
			{"!template 1.23e10", 1.23e10},
		}

		for _, tt := range tests {
			result := processTagTemplate(tt.input)
			num, ok := result.(float64)
			assert.True(t, ok, "Should decode to number")
			assert.Equal(t, tt.expected, num)
		}
	})

	t.Run("special characters in strings", func(t *testing.T) {
		// Regression test: Special characters should be preserved
		input := `!template {"special": "!@#$%^&*()_+-=[]{}|;:',.<>?/~"}`
		result := processTagTemplate(input)

		m, ok := result.(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "!@#$%^&*()_+-=[]{}|;:',.<>?/~", m["special"])
	})
}

// Benchmark tests for performance.
func BenchmarkProcessTagTemplate_String(b *testing.B) {
	input := `!template "hello-world"`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		processTagTemplate(input)
	}
}

func BenchmarkProcessTagTemplate_List(b *testing.B) {
	input := `!template ["item-1", "item-2", "item-3", "item-4", "item-5"]`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		processTagTemplate(input)
	}
}

func BenchmarkProcessTagTemplate_Map(b *testing.B) {
	input := `!template {"key1": "value1", "key2": "value2", "key3": "value3", "key4": "value4", "key5": "value5"}`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		processTagTemplate(input)
	}
}

func BenchmarkProcessTagTemplate_NestedStructure(b *testing.B) {
	input := `!template {"name": "test", "count": 5, "tags": ["tag1", "tag2", "tag3"], "metadata": {"env": "prod", "region": "us-east-1"}}`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		processTagTemplate(input)
	}
}

func BenchmarkProcessTagTemplate_InvalidJSON(b *testing.B) {
	input := "!template {this is not valid json}"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		processTagTemplate(input)
	}
}

// TestProcessTemplateTagsOnly tests the ProcessTemplateTagsOnly function with various input structures.
func TestProcessTemplateTagsOnly(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected map[string]any
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty map",
			input:    map[string]any{},
			expected: map[string]any{},
		},
		{
			name: "simple string without template tag",
			input: map[string]any{
				"key": "value",
			},
			expected: map[string]any{
				"key": "value",
			},
		},
		{
			name: "template tag with JSON string",
			input: map[string]any{
				"key": `!template "hello"`,
			},
			expected: map[string]any{
				"key": "hello",
			},
		},
		{
			name: "template tag with JSON number",
			input: map[string]any{
				"count": "!template 42",
			},
			expected: map[string]any{
				"count": float64(42),
			},
		},
		{
			name: "template tag with JSON boolean",
			input: map[string]any{
				"enabled": "!template true",
			},
			expected: map[string]any{
				"enabled": true,
			},
		},
		{
			name: "template tag with JSON array",
			input: map[string]any{
				"tags": `!template ["tag1", "tag2", "tag3"]`,
			},
			expected: map[string]any{
				"tags": []any{"tag1", "tag2", "tag3"},
			},
		},
		{
			name: "template tag with JSON object",
			input: map[string]any{
				"config": `!template {"name": "test", "count": 5}`,
			},
			expected: map[string]any{
				"config": map[string]any{
					"name":  "test",
					"count": float64(5),
				},
			},
		},
		{
			name: "nested map with template tags",
			input: map[string]any{
				"outer": map[string]any{
					"inner": `!template "nested-value"`,
				},
			},
			expected: map[string]any{
				"outer": map[string]any{
					"inner": "nested-value",
				},
			},
		},
		{
			name: "array with template tags",
			input: map[string]any{
				"items": []any{
					`!template "item1"`,
					`!template 42`,
					`!template true`,
				},
			},
			expected: map[string]any{
				"items": []any{
					"item1",
					float64(42),
					true,
				},
			},
		},
		{
			name: "mixed structure with template tags",
			input: map[string]any{
				"string":           "plain",
				"number":           42,
				"boolean":          true,
				"templated_string": `!template "from-template"`,
				"templated_number": "!template 100",
				"nested": map[string]any{
					"value": `!template ["a", "b", "c"]`,
				},
				"array": []any{
					"plain",
					`!template "templated"`,
					map[string]any{
						"key": `!template {"nested": "object"}`,
					},
				},
			},
			expected: map[string]any{
				"string":           "plain",
				"number":           42,
				"boolean":          true,
				"templated_string": "from-template",
				"templated_number": float64(100),
				"nested": map[string]any{
					"value": []any{"a", "b", "c"},
				},
				"array": []any{
					"plain",
					"templated",
					map[string]any{
						"key": map[string]any{"nested": "object"},
					},
				},
			},
		},
		{
			name: "non-template YAML functions left unchanged",
			input: map[string]any{
				"terraform_output": "!terraform.output vpc_id",
				"store_value":      "!store.get secret/key",
				"exec_result":      "!exec echo hello",
			},
			expected: map[string]any{
				"terraform_output": "!terraform.output vpc_id",
				"store_value":      "!store.get secret/key",
				"exec_result":      "!exec echo hello",
			},
		},
		{
			name: "template tag with plain string (no JSON)",
			input: map[string]any{
				"simple": "!template plain-value",
			},
			expected: map[string]any{
				"simple": "plain-value",
			},
		},
		{
			name: "deeply nested structures",
			input: map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"level3": []any{
							map[string]any{
								"value": `!template "deep"`,
							},
						},
					},
				},
			},
			expected: map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"level3": []any{
							map[string]any{
								"value": "deep",
							},
						},
					},
				},
			},
		},
		{
			name: "array of maps with template tags",
			input: map[string]any{
				"items": []any{
					map[string]any{
						"name":  `!template "item1"`,
						"count": "!template 10",
					},
					map[string]any{
						"name":  `!template "item2"`,
						"count": "!template 20",
					},
				},
			},
			expected: map[string]any{
				"items": []any{
					map[string]any{
						"name":  "item1",
						"count": float64(10),
					},
					map[string]any{
						"name":  "item2",
						"count": float64(20),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ProcessTemplateTagsOnly(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestProcessTemplateTagsOnly_PreservesOriginal verifies that the original input is not modified.
func TestProcessTemplateTagsOnly_PreservesOriginal(t *testing.T) {
	original := map[string]any{
		"key": `!template "value"`,
		"nested": map[string]any{
			"inner": "!template 42",
		},
	}

	// Make a copy to compare later.
	originalCopy := map[string]any{
		"key": `!template "value"`,
		"nested": map[string]any{
			"inner": "!template 42",
		},
	}

	result := ProcessTemplateTagsOnly(original)

	// Verify original is unchanged.
	assert.Equal(t, originalCopy, original)

	// Verify result has processed templates.
	assert.Equal(t, "value", result["key"])
	assert.Equal(t, map[string]any{"inner": float64(42)}, result["nested"])
}
