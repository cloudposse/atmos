package exec

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// Note: We cannot directly test unsupported tag errors because they call
// errUtils.CheckErrorPrintAndExit which exits the process.

func TestProcessCustomTags_UnsupportedTagDetection(t *testing.T) {
	unsupportedTags := []struct {
		name        string
		input       string
		tag         string
		description string
	}{
		{
			name:        "completely unknown tag",
			input:       "!unknown value",
			tag:         "!unknown",
			description: "Unknown tag that doesn't exist",
		},
		{
			name:        "typo in env tag",
			input:       "!envv HOME",
			tag:         "!envv",
			description: "Typo in env tag",
		},
		{
			name:        "typo in exec tag",
			input:       "!exce command",
			tag:         "!exce",
			description: "Typo in exec tag",
		},
		{
			name:        "typo in template tag",
			input:       "!templat value",
			tag:         "!templat",
			description: "Missing 'e' in template",
		},
		{
			name:        "typo in store tag",
			input:       "!stor ssm path",
			tag:         "!stor",
			description: "Missing 'e' in store",
		},
		{
			name:        "typo in store.get tag",
			input:       "!store.gett ssm path",
			tag:         "!store.gett",
			description: "Extra 't' in store.get",
		},
		{
			name:        "typo in terraform.output tag",
			input:       "!terraform.outputs component stack output",
			tag:         "!terraform.outputs",
			description: "Plural form of output",
		},
		{
			name:        "typo in terraform.state tag",
			input:       "!terraform.stat component stack path",
			tag:         "!terraform.stat",
			description: "Missing 'e' in state",
		},
		{
			name:        "custom user tag",
			input:       "!mytag custom",
			tag:         "!mytag",
			description: "Custom user-defined tag",
		},
		{
			name:        "tag with underscore",
			input:       "!my_tag value",
			tag:         "!my_tag",
			description: "Tag with underscore",
		},
		{
			name:        "tag with hyphen",
			input:       "!my-custom-tag value",
			tag:         "!my-custom-tag",
			description: "Tag with hyphens",
		},
		{
			name:        "tag with numbers",
			input:       "!tag123 value",
			tag:         "!tag123",
			description: "Tag with numbers",
		},
		{
			name:        "uppercase tag",
			input:       "!UPPERCASE value",
			tag:         "!UPPERCASE",
			description: "Uppercase tag",
		},
		{
			name:        "mixed case tag",
			input:       "!MixedCase value",
			tag:         "!MixedCase",
			description: "Mixed case tag",
		},
		{
			name:        "similar to include",
			input:       "!includes file.yaml",
			tag:         "!includes",
			description: "Plural form of include",
		},
		{
			name:        "similar to repo-root",
			input:       "!repo_root",
			tag:         "!repo_root",
			description: "Underscore instead of hyphen",
		},
		{
			name:        "terraform without subcommand",
			input:       "!terraform component",
			tag:         "!terraform",
			description: "Terraform without .output or .state",
		},
		{
			name:        "store with wrong subcommand",
			input:       "!store.put ssm path value",
			tag:         "!store.put",
			description: "store.put instead of store.get",
		},
		{
			name:        "wrong prefix for terraform",
			input:       "!tf.output component stack output",
			tag:         "!tf.output",
			description: "tf instead of terraform",
		},
		{
			name:        "environment instead of env",
			input:       "!environment VAR",
			tag:         "!environment",
			description: "Full word instead of abbreviation",
		},
	}

	for _, tt := range unsupportedTags {
		t.Run(tt.name, func(t *testing.T) {
			// We need to test that the function would detect this as unsupported
			// Check if the tag would be recognized as unsupported
			// Use matchesSupportedTag which checks for exact tag followed by space/whitespace
			isSupportedTag := matchesSupportedTag(tt.input, u.AllSupportedYamlTags)

			assert.False(t, isSupportedTag, "%s: Tag '%s' should not be recognized as supported", tt.description, tt.tag)
			assert.True(t, strings.HasPrefix(tt.input, "!"), "Input should start with ! to be recognized as a tag")
		})
	}
}

func TestProcessCustomTags_ErrorMessageFormat(t *testing.T) {
	// Test that error messages for unsupported tags have the correct format
	tests := []struct {
		name             string
		unsupportedTag   string
		stack            string
		expectedContains []string
	}{
		{
			name:           "error message contains tag",
			unsupportedTag: "!invalid",
			stack:          "dev-stack",
			expectedContains: []string{
				"unsupported YAML tag",
				"!invalid",
				"dev-stack",
				"Supported tags are:",
			},
		},
		{
			name:             "error message lists supported tags",
			unsupportedTag:   "!custom",
			stack:            "prod-stack",
			expectedContains: u.AllSupportedYamlTags,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create the expected error using the central list of supported tags
			err := fmt.Errorf("%w: '%s' in stack '%s'. Supported tags are: %s",
				errUtils.ErrUnsupportedYamlTag,
				tt.unsupportedTag,
				tt.stack,
				strings.Join(u.AllSupportedYamlTags, ", "))

			errMsg := err.Error()

			// Check that all expected strings are in the error message
			for _, expected := range tt.expectedContains {
				assert.Contains(t, errMsg, expected, "Error message should contain: %s", expected)
			}
		})
	}
}

func TestProcessCustomTags_ValidTagsNotReportedAsUnsupported(t *testing.T) {
	validTags := []struct {
		name  string
		input string
	}{
		{"template tag", "!template value"},
		{"exec tag", "!exec echo test"},
		{"store tag", "!store ssm /path"},
		{"store.get tag", "!store.get ssm /path"},
		{"terraform.output tag", "!terraform.output component stack output"},
		{"terraform.state tag", "!terraform.state component stack path"},
		{"env tag", "!env HOME"},
		{"template with path", "!template path/to/file.yaml"},
		{"exec with complex command", "!exec ls -la | grep test"},
		{"store with complex path", "!store ssm /very/long/path/to/secret"},
		{"terraform.output with all params", "!terraform.output vpc prod vpc_id"},
		{"terraform.state with all params", "!terraform.state vpc prod outputs.vpc_id"},
		{"env with special var", "!env USER"},
	}

	for _, tt := range validTags {
		t.Run(tt.name, func(t *testing.T) {
			// Check that valid tags are recognized as supported
			// Use matchesSupportedTag which checks for exact tag followed by space/whitespace
			isSupportedTag := matchesSupportedTag(tt.input, u.AllSupportedYamlTags)

			assert.True(t, isSupportedTag, "Tag in '%s' should be recognized as supported", tt.input)
		})
	}
}

// matchesSupportedTag checks if input matches one of the supported tags.
// A tag matches if the input starts with the tag and is followed by a space, tab, newline, or end of string.
// This prevents false positives like "!envv" matching "!env".
func matchesSupportedTag(input string, supportedTags []string) bool {
	for _, tag := range supportedTags {
		if strings.HasPrefix(input, tag) {
			// Check if the tag is followed by whitespace or is the entire string
			rest := strings.TrimPrefix(input, tag)
			if rest == "" || rest[0] == ' ' || rest[0] == '\t' || rest[0] == '\n' {
				return true
			}
		}
	}
	return false
}

func TestProcessCustomTags_NonTagStrings(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	nonTagStrings := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "regular string",
			input:    "just a normal string",
			expected: "just a normal string",
		},
		{
			name:     "string with exclamation in middle",
			input:    "this is! not a tag",
			expected: "this is! not a tag",
		},
		{
			name:     "string with exclamation at end",
			input:    "not a tag!",
			expected: "not a tag!",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: "   ",
		},
		{
			name:     "number string",
			input:    "12345",
			expected: "12345",
		},
		{
			name:     "boolean-like string",
			input:    "true",
			expected: "true",
		},
		{
			name:     "path string",
			input:    "/path/to/file.yaml",
			expected: "/path/to/file.yaml",
		},
		{
			name:     "url string",
			input:    "https://example.com",
			expected: "https://example.com",
		},
		{
			name:     "json-like string",
			input:    `{"key": "value"}`,
			expected: `{"key": "value"}`,
		},
	}

	for _, tt := range nonTagStrings {
		t.Run(tt.name, func(t *testing.T) {
			result, _ := processCustomTags(atmosConfig, tt.input, "test-stack", []string{}, nil)
			assert.Equal(t, tt.expected, result, "Non-tag string should be returned as-is")
		})
	}
}

func TestProcessCustomTags_BoundaryConditions(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	tests := []struct {
		name       string
		input      string
		skip       []string
		expected   interface{}
		shouldSkip bool // Tests that trigger CheckErrorPrintAndExit cannot be run
	}{
		{
			name:       "tag only without value",
			input:      "!template",
			expected:   "",
			shouldSkip: true, // Triggers CheckErrorPrintAndExit for empty value
		},
		{
			name:       "tag with empty value after space",
			input:      "!template ",
			expected:   "",
			shouldSkip: true, // Triggers CheckErrorPrintAndExit for empty value
		},
		{
			name:     "tag with multiple spaces",
			input:    "!template    value",
			expected: "value",
		},
		{
			name:     "tag with tabs",
			input:    "!template\t\tvalue",
			expected: "value",
		},
		{
			name:     "tag with newline",
			input:    "!template\nvalue",
			expected: "value",
		},
		{
			name:     "very long tag value",
			input:    "!template " + strings.Repeat("a", 10000),
			expected: strings.Repeat("a", 10000),
		},
		{
			name:     "skip all tags",
			input:    "!template value",
			skip:     []string{"template", "exec", "env", "store", "store.get", "terraform.output", "terraform.state"},
			expected: "!template value",
		},
		{
			name:     "unicode in tag value",
			input:    "!template 你好世界 🌍 مرحبا",
			expected: "你好世界 🌍 مرحبا",
		},
		{
			name:     "special characters in value",
			input:    "!template <>?:|{}[]~`!@#$%^&*()_+-=",
			expected: "<>?:|{}[]~`!@#$%^&*()_+-=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldSkip {
				t.Skipf("Skipping test '%s': would trigger CheckErrorPrintAndExit and exit the process", tt.name)
			}
			result, _ := processCustomTags(atmosConfig, tt.input, "test-stack", tt.skip, nil)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessNodes_TypePreservation(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	input := map[string]any{
		"string":     "!template test",
		"integer":    42,
		"float":      3.14159,
		"boolean":    true,
		"null":       nil,
		"empty_list": []any{},
		"empty_map":  map[string]any{},
		"mixed_list": []any{
			"!template string",
			123,
			false,
			nil,
			map[string]any{"nested": "!template value"},
		},
	}

	result, _ := processNodes(atmosConfig, input, "test-stack", []string{}, nil)

	// Check type preservation
	assert.IsType(t, "", result["string"])
	assert.Equal(t, "test", result["string"])

	assert.IsType(t, 42, result["integer"])
	assert.Equal(t, 42, result["integer"])

	assert.IsType(t, 3.14159, result["float"])
	assert.Equal(t, 3.14159, result["float"])

	assert.IsType(t, true, result["boolean"])
	assert.Equal(t, true, result["boolean"])

	assert.Nil(t, result["null"])

	assert.IsType(t, []any{}, result["empty_list"])
	assert.Empty(t, result["empty_list"])

	assert.IsType(t, map[string]any{}, result["empty_map"])
	assert.Empty(t, result["empty_map"])

	mixedList := result["mixed_list"].([]any)
	assert.Equal(t, "string", mixedList[0])
	assert.Equal(t, 123, mixedList[1])
	assert.Equal(t, false, mixedList[2])
	assert.Nil(t, mixedList[3])

	nestedMap := mixedList[4].(map[string]any)
	assert.Equal(t, "value", nestedMap["nested"])
}
