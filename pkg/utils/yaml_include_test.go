package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestIncludeWithHashCharacter verifies that strings starting with '#' are properly handled with !include.
func TestIncludeWithHashCharacter(t *testing.T) {
	// Create a temporary directory for test files
	tempDir := t.TempDir()

	// Create a test file with values including strings starting with '#'
	testValuesFile := filepath.Join(tempDir, "test_values.yaml")
	testValuesContent := `---
regular_string: 'regular string'
hash_at_start: '#something'
hash_at_start_single_quoted: '#something'
hash_at_start_double_quoted: "#something"
hash_in_middle: 'value#with#hash'
comment_at_end: 'value' # with comment
comment_at_end_no_quotes: value # with comment
`
	err := os.WriteFile(testValuesFile, []byte(testValuesContent), 0o644)
	assert.NoError(t, err)

	// Create a test file that includes values from the first file
	testIncludeFile := filepath.Join(tempDir, "test_include.yaml")
	testIncludeContent := `---
components:
  terraform:
    test_component:
      metadata:
        component: test_component
      vars:
        regular_string: !include test_values.yaml .regular_string
        hash_at_start: !include test_values.yaml .hash_at_start
        hash_at_start_single_quoted: !include test_values.yaml .hash_at_start_single_quoted
        hash_at_start_double_quoted: !include test_values.yaml .hash_at_start_double_quoted
        hash_in_middle: !include test_values.yaml .hash_in_middle
        comment_at_end: !include test_values.yaml .comment_at_end
        comment_at_end_no_quotes: !include test_values.yaml .comment_at_end_no_quotes
`
	err = os.WriteFile(testIncludeFile, []byte(testIncludeContent), 0o644)
	assert.NoError(t, err)

	// Change to the temp directory to make relative paths work
	t.Chdir(tempDir)

	// Read the include file
	yamlFileContent, err := os.ReadFile("test_include.yaml")
	assert.NoError(t, err)

	// Create Atmos config
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: ".",
		Logs: schema.Logs{
			Level: "Info",
		},
	}

	// Parse the YAML file
	manifest, err := UnmarshalYAMLFromFile[schema.AtmosSectionMapType](atmosConfig, string(yamlFileContent), "test_include.yaml")
	assert.NoError(t, err)

	// Get the component vars
	componentVars := manifest["components"].(map[string]any)["terraform"].(map[string]any)["test_component"].(map[string]any)["vars"].(map[string]any)

	assert.Equal(t, "regular string", componentVars["regular_string"], "Regular string should be included correctly")
	assert.Equal(t, "#something", componentVars["hash_at_start"], "String starting with # should be included correctly")
	assert.Equal(t, "#something", componentVars["hash_at_start_single_quoted"], "Single-quoted string starting with # should be included correctly")
	assert.Equal(t, "#something", componentVars["hash_at_start_double_quoted"], "Double-quoted string starting with # should be included correctly")
	assert.Equal(t, "value#with#hash", componentVars["hash_in_middle"], "String with # in the middle should be included correctly")
	assert.Equal(t, "value", componentVars["comment_at_end"], "String with comment at the end should be included correctly")
	assert.Equal(t, "value", componentVars["comment_at_end_no_quotes"], "String with comment at the end (no quotes) should be included correctly")
}
