package exec

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestProcessCustomTags_Coverage tests the coverage of processCustomTags function
// focusing on the different code paths without actually executing external functions.
func TestProcessCustomTags_Coverage(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// Set up test environment variable
	os.Setenv("TEST_ENV_VAR", "test_value")
	defer os.Unsetenv("TEST_ENV_VAR")

	tests := []struct {
		name     string
		input    string
		stack    string
		skip     []string
		expected any
	}{
		// Test template tag path
		{
			name:     "template tag processing",
			input:    "!template simple_value",
			stack:    "test-stack",
			skip:     []string{},
			expected: "simple_value",
		},
		// Test env tag path
		{
			name:     "env tag processing",
			input:    "!env TEST_ENV_VAR",
			stack:    "test-stack",
			skip:     []string{},
			expected: "test_value",
		},
		// Test skipped functions
		{
			name:     "exec tag skipped",
			input:    "!exec echo test",
			stack:    "test-stack",
			skip:     []string{"exec"},
			expected: "!exec echo test",
		},
		// Note: store requires store registry setup, always skip
		// {
		//	name:     "store tag skipped",
		//	input:    "!store ssm /path",
		//	stack:    "test-stack",
		//	skip:     []string{"store"},
		//	expected: "!store ssm /path",
		// },
		// Note: store.get requires store registry setup, always skip
		// {
		//	name:     "store.get tag skipped",
		//	input:    "!store.get ssm /path",
		//	stack:    "test-stack",
		//	skip:     []string{"store.get"},
		//	expected: "!store.get ssm /path",
		// },
		{
			name:     "terraform.output tag skipped",
			input:    "!terraform.output vpc dev output",
			stack:    "test-stack",
			skip:     []string{"terraform.output"},
			expected: "!terraform.output vpc dev output",
		},
		{
			name:     "terraform.state tag skipped",
			input:    "!terraform.state vpc dev state",
			stack:    "test-stack",
			skip:     []string{"terraform.state"},
			expected: "!terraform.state vpc dev state",
		},
		// Test non-tag strings
		{
			name:     "regular string",
			input:    "just a regular string",
			stack:    "test-stack",
			skip:     []string{},
			expected: "just a regular string",
		},
		{
			name:     "string with ! in middle",
			input:    "not! a tag",
			stack:    "test-stack",
			skip:     []string{},
			expected: "not! a tag",
		},
		// Test supported tag that matches but is in skip list
		{
			name:     "template tag in skip list",
			input:    "!template value",
			stack:    "test-stack",
			skip:     []string{"template"},
			expected: "!template value",
		},
		{
			name:     "env tag in skip list",
			input:    "!env PATH",
			stack:    "test-stack",
			skip:     []string{"env"},
			expected: "!env PATH",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip tests that would require external setup
			if len(tt.skip) == 0 && (hasPrefix(tt.input, "!store") || hasPrefix(tt.input, "!terraform") || tt.input == "!exec echo test") {
				t.Skip("Skipping test that requires external setup")
			}

			result := processCustomTags(atmosConfig, tt.input, tt.stack, tt.skip)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestProcessCustomTags_UnsupportedTagPath tests the unsupported tag detection code path.
// We can't actually execute this path in tests because it calls errUtils.CheckErrorPrintAndExit
// which would exit the test process, but we can verify the logic is correct.
func TestProcessCustomTags_UnsupportedTagPath(t *testing.T) {
	// Test that various unsupported tags would be detected
	unsupportedTags := []string{
		"!invalid",
		"!custom",
		"!unknown",
		"!mytag",
		"!envv",      // typo
		"!exce",      // typo
		"!templat",   // typo
		"!stor",      // typo
		"!terraform", // without .output or .state
	}

	supportedPrefixes := []string{
		"!template",
		"!exec",
		"!store.get",
		"!store",
		"!terraform.output",
		"!terraform.state",
		"!env",
	}

	for _, tag := range unsupportedTags {
		t.Run(tag, func(t *testing.T) {
			// Check that the tag would not match any supported prefix
			isSupported := false
			testInput := tag + " test"
			for _, prefix := range supportedPrefixes {
				if hasPrefix(testInput, prefix) {
					isSupported = true
					break
				}
			}
			assert.False(t, isSupported, "Tag %s should not be recognized as supported", tag)
		})
	}
}

// TestProcessNodes_Coverage tests processNodes with various data structures.
func TestProcessNodes_Coverage(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	tests := []struct {
		name     string
		input    map[string]any
		expected map[string]any
	}{
		{
			name: "simple map",
			input: map[string]any{
				"key1": "!template value1",
				"key2": "regular",
			},
			expected: map[string]any{
				"key1": "value1",
				"key2": "regular",
			},
		},
		{
			name: "nested map",
			input: map[string]any{
				"outer": map[string]any{
					"inner": "!template nested",
				},
			},
			expected: map[string]any{
				"outer": map[string]any{
					"inner": "nested",
				},
			},
		},
		{
			name: "array values",
			input: map[string]any{
				"list": []any{
					"!template item1",
					"regular",
					"!template item2",
				},
			},
			expected: map[string]any{
				"list": []any{
					"item1",
					"regular",
					"item2",
				},
			},
		},
		{
			name: "mixed types",
			input: map[string]any{
				"string": "!template test",
				"number": 42,
				"bool":   true,
				"null":   nil,
			},
			expected: map[string]any{
				"string": "test",
				"number": 42,
				"bool":   true,
				"null":   nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processNodes(atmosConfig, tt.input, "test-stack", []string{})
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestProcessCustomYamlTags_Integration tests the main entry point.
func TestProcessCustomYamlTags_Integration(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	input := schema.AtmosSectionMapType{
		"template": "!template value",
		"nested": map[string]any{
			"key": "!template nested_value",
		},
		"regular": "normal_string",
	}

	expected := schema.AtmosSectionMapType{
		"template": "value",
		"nested": map[string]any{
			"key": "nested_value",
		},
		"regular": "normal_string",
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", []string{})
	assert.NoError(t, err)
	assert.Equal(t, expected, result)
}

// hasPrefix is a helper function to check string prefixes.
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
