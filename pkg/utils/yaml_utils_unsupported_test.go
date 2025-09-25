package utils

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestProcessCustomTagsWithUnsupportedTags(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	tests := []struct {
		name        string
		yamlContent string
		shouldError bool
		errorMsg    string
	}{
		{
			name: "Valid YAML with supported !env tag",
			yamlContent: `
key: !env ENV_VAR
other: value`,
			shouldError: false,
		},
		{
			name: "Valid YAML with supported !exec tag",
			yamlContent: `
command: !exec echo hello
other: value`,
			shouldError: false,
		},
		{
			name: "Valid YAML with supported !include tag",
			yamlContent: `
data: !include file.yaml
other: value`,
			shouldError: false,
		},
		{
			name: "Valid YAML with supported !include.raw tag",
			yamlContent: `
content: !include.raw file.txt
other: value`,
			shouldError: false,
		},
		{
			name: "Valid YAML with supported !repo-root tag",
			yamlContent: `
path: !repo-root
other: value`,
			shouldError: false,
		},
		{
			name: "Valid YAML with supported !template tag",
			yamlContent: `
value: !template "{{ .value }}"
other: value`,
			shouldError: false,
		},
		{
			name: "Valid YAML with supported !terraform.output tag",
			yamlContent: `
output: !terraform.output vpc dev output_name
other: value`,
			shouldError: false,
		},
		{
			name: "Valid YAML with supported !terraform.state tag",
			yamlContent: `
state: !terraform.state vpc dev state_path
other: value`,
			shouldError: false,
		},
		{
			name: "Valid YAML with supported !store tag",
			yamlContent: `
secret: !store ssm /path/to/secret
other: value`,
			shouldError: false,
		},
		{
			name: "Valid YAML with supported !store.get tag",
			yamlContent: `
secret: !store.get ssm /path/to/secret
other: value`,
			shouldError: false,
		},
		{
			name: "Invalid YAML with unsupported !invalid tag",
			yamlContent: `
key: !invalid some_value
other: value`,
			shouldError: true,
			errorMsg:    "unsupported YAML tag: '!invalid'",
		},
		{
			name: "Invalid YAML with unsupported !custom tag",
			yamlContent: `
data: !custom custom_value
other: value`,
			shouldError: true,
			errorMsg:    "unsupported YAML tag: '!custom'",
		},
		{
			name: "Invalid YAML with unsupported !unknown tag",
			yamlContent: `
field: !unknown unknown_value
other: value`,
			shouldError: true,
			errorMsg:    "unsupported YAML tag: '!unknown'",
		},
		{
			name: "Invalid YAML with typo in tag !envv",
			yamlContent: `
key: !envv ENV_VAR
other: value`,
			shouldError: true,
			errorMsg:    "unsupported YAML tag: '!envv'",
		},
		{
			name: "Invalid YAML with typo in tag !exce",
			yamlContent: `
command: !exce echo hello
other: value`,
			shouldError: true,
			errorMsg:    "unsupported YAML tag: '!exce'",
		},
		{
			name: "Invalid YAML with typo in tag !inlcude",
			yamlContent: `
data: !inlcude file.yaml
other: value`,
			shouldError: true,
			errorMsg:    "unsupported YAML tag: '!inlcude'",
		},
		{
			name: "Valid YAML with standard !!str tag",
			yamlContent: `
key: !!str "123"
other: !!int 456`,
			shouldError: false,
		},
		{
			name: "Valid YAML with no tags",
			yamlContent: `
key: value
nested:
  field: data`,
			shouldError: false,
		},
		{
			name: "Valid YAML with multiple supported tags",
			yamlContent: `
env_var: !env HOME
exec_result: !exec pwd
include_data: !include config.yaml`,
			shouldError: false,
		},
		{
			name: "Invalid YAML with multiple unsupported tags",
			yamlContent: `
invalid1: !bad value
invalid2: !wrong data`,
			shouldError: true,
			errorMsg:    "unsupported YAML tag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var node yaml.Node
			err := yaml.Unmarshal([]byte(tt.yamlContent), &node)
			assert.NoError(t, err, "Failed to unmarshal YAML")

			err = processCustomTags(atmosConfig, &node, "test.yaml")

			if tt.shouldError {
				assert.Error(t, err, "Expected an error for unsupported tag")
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg, "Error message should contain expected text")
				}
				// Check that the error message includes the list of supported tags
				assert.Contains(t, err.Error(), "Supported tags are:", "Error should list supported tags")
			} else {
				assert.NoError(t, err, "Expected no error for valid YAML")
			}
		})
	}
}

func TestProcessCustomTagsErrorMessages(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	tests := []struct {
		name             string
		yamlContent      string
		expectedTag      string
		expectedFileName string
	}{
		{
			name: "Error message includes tag name and file name",
			yamlContent: `
key: !invalid_tag value`,
			expectedTag:      "!invalid_tag",
			expectedFileName: "stack.yaml",
		},
		{
			name: "Error message for nested unsupported tag",
			yamlContent: `
parent:
  child: !bad_tag value`,
			expectedTag:      "!bad_tag",
			expectedFileName: "nested.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var node yaml.Node
			err := yaml.Unmarshal([]byte(tt.yamlContent), &node)
			assert.NoError(t, err, "Failed to unmarshal YAML")

			err = processCustomTags(atmosConfig, &node, tt.expectedFileName)
			assert.Error(t, err, "Expected an error for unsupported tag")

			// Check error message contains the tag
			assert.Contains(t, err.Error(), tt.expectedTag, "Error should mention the unsupported tag")

			// Check error message contains the file name
			assert.Contains(t, err.Error(), tt.expectedFileName, "Error should mention the file name")

			// Check error message lists supported tags
			assert.Contains(t, err.Error(), "Supported tags are:", "Error should list supported tags")
			assert.Contains(t, err.Error(), "!env", "Error should list !env as supported")
			assert.Contains(t, err.Error(), "!exec", "Error should list !exec as supported")
			assert.Contains(t, err.Error(), "!include", "Error should list !include as supported")
		})
	}
}

func TestAllSupportedYamlTagsList(t *testing.T) {
	// Test that AllSupportedYamlTags contains all expected tags
	expectedTags := []string{
		"!exec",
		"!store",
		"!store.get",
		"!template",
		"!terraform.output",
		"!terraform.state",
		"!env",
		"!include",
		"!include.raw",
		"!repo-root",
	}

	for _, expectedTag := range expectedTags {
		found := false
		for _, tag := range AllSupportedYamlTags {
			if tag == expectedTag {
				found = true
				break
			}
		}
		assert.True(t, found, "AllSupportedYamlTags should contain %s", expectedTag)
	}

	// Ensure AllSupportedYamlTags has exactly the expected number of tags
	assert.Equal(t, len(expectedTags), len(AllSupportedYamlTags), "AllSupportedYamlTags should have exactly %d tags", len(expectedTags))
}

func TestProcessCustomTagsWithMixedValidAndInvalid(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// Test that the first unsupported tag causes an error, even if there are valid tags
	yamlContent := `
valid_env: !env HOME
invalid_tag: !unsupported value
valid_exec: !exec pwd`

	var node yaml.Node
	err := yaml.Unmarshal([]byte(yamlContent), &node)
	assert.NoError(t, err, "Failed to unmarshal YAML")

	err = processCustomTags(atmosConfig, &node, "mixed.yaml")
	assert.Error(t, err, "Expected an error when unsupported tag is present")
	assert.Contains(t, err.Error(), "!unsupported", "Error should mention the unsupported tag")
}

func TestProcessCustomTagsWithSimilarUnsupportedTags(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// Test tags that are similar to supported ones but not exact matches
	similarTags := []string{
		"!env2",        // Similar to !env
		"!environment", // Similar to !env
		"!execute",     // Similar to !exec
		"!includes",    // Similar to !include
		"!store.put",   // Similar to !store.get
		"!terraform",   // Similar to !terraform.output
	}

	for _, tag := range similarTags {
		t.Run(strings.TrimPrefix(tag, "!"), func(t *testing.T) {
			yamlContent := "key: " + tag + " value"

			var node yaml.Node
			err := yaml.Unmarshal([]byte(yamlContent), &node)
			assert.NoError(t, err, "Failed to unmarshal YAML")

			err = processCustomTags(atmosConfig, &node, "test.yaml")
			assert.Error(t, err, "Expected an error for unsupported tag %s", tag)
			assert.Contains(t, err.Error(), tag, "Error should mention the unsupported tag %s", tag)
		})
	}
}
