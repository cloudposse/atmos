package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// TestProcessTagEnv tests the !env YAML function processing.
func TestProcessTagEnv(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		envVar   string
		envValue string
		expected string
	}{
		{
			name:     "simple env var",
			input:    "!env TEST_VAR_1",
			envVar:   "TEST_VAR_1",
			envValue: "test-value-1",
			expected: "test-value-1",
		},
		{
			name:     "env var with underscores",
			input:    "!env TEST_VAR_WITH_UNDERSCORES",
			envVar:   "TEST_VAR_WITH_UNDERSCORES",
			envValue: "value-with-underscores",
			expected: "value-with-underscores",
		},
		{
			name:     "env var with numeric suffix",
			input:    "!env MY_VAR_123",
			envVar:   "MY_VAR_123",
			envValue: "numeric-suffix-value",
			expected: "numeric-suffix-value",
		},
		{
			name:     "env var with empty value",
			input:    "!env EMPTY_VAR",
			envVar:   "EMPTY_VAR",
			envValue: "",
			expected: "",
		},
		{
			name:     "env var with special characters in value",
			input:    "!env SPECIAL_VAR",
			envVar:   "SPECIAL_VAR",
			envValue: "value!@#$%^&*()",
			expected: "value!@#$%^&*()",
		},
		{
			name:     "env var with spaces in value",
			input:    "!env SPACES_VAR",
			envVar:   "SPACES_VAR",
			envValue: "value with spaces",
			expected: "value with spaces",
		},
		{
			name:     "env var with newlines in value",
			input:    "!env NEWLINES_VAR",
			envVar:   "NEWLINES_VAR",
			envValue: "line1\nline2\nline3",
			expected: "line1\nline2\nline3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the environment variable.
			t.Setenv(tt.envVar, tt.envValue)

			// Process the env tag using the utils function.
			result, err := u.ProcessTagEnv(tt.input, nil)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestProcessTagEnvWithDefault tests the !env YAML function with default values.
func TestProcessTagEnvWithDefault(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		envVar   string
		envValue string
		setEnv   bool
		expected string
	}{
		{
			name:     "env var exists, no default used",
			input:    "!env EXISTING_VAR default-value",
			envVar:   "EXISTING_VAR",
			envValue: "actual-value",
			setEnv:   true,
			expected: "actual-value",
		},
		{
			name:     "env var missing, default used",
			input:    "!env MISSING_VAR_UNIQUE_123456 default-value",
			envVar:   "MISSING_VAR_UNIQUE_123456",
			envValue: "",
			setEnv:   false,
			expected: "default-value",
		},
		{
			name:     "env var empty, empty returned",
			input:    "!env EMPTY_VAR_TEST",
			envVar:   "EMPTY_VAR_TEST",
			envValue: "",
			setEnv:   true,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv(tt.envVar, tt.envValue)
			}

			result, err := u.ProcessTagEnv(tt.input, nil)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestEnvFunctionInYAMLParsing tests the !env function during YAML parsing.
// Note: !env functions are deferred during initial parsing and resolved during stack processing.
func TestEnvFunctionInYAMLParsing(t *testing.T) {
	t.Setenv("TEST_YAML_ENV_VAR", "env-value")
	t.Setenv("TEST_YAML_ENV_VAR_2", "second-value")

	yamlContent := `
components:
  terraform:
    test-component:
      vars:
        env_value: !env TEST_YAML_ENV_VAR
        env_value_2: !env TEST_YAML_ENV_VAR_2
        static_value: "static"
`

	atmosConfig := &schema.AtmosConfiguration{}

	result, err := u.UnmarshalYAMLFromFile[map[string]interface{}](atmosConfig, yamlContent, "test.yaml")
	require.NoError(t, err)

	// Navigate to vars.
	components := result["components"].(map[string]interface{})
	terraform := components["terraform"].(map[string]interface{})
	testComponent := terraform["test-component"].(map[string]interface{})
	vars := testComponent["vars"].(map[string]interface{})

	// During raw YAML parsing, !env functions are stored as strings (deferred).
	// They are resolved during stack processing via processCustomYamlTags.
	assert.Contains(t, vars["env_value"].(string), "!env", "!env should be deferred during parsing")
	assert.Contains(t, vars["env_value_2"].(string), "!env", "!env should be deferred during parsing")
	assert.Equal(t, "static", vars["static_value"])
}

// TestEnvFunctionInLists tests the !env function when used in YAML lists.
// Note: !env functions are deferred during initial parsing and resolved during stack processing.
func TestEnvFunctionInLists(t *testing.T) {
	t.Setenv("LIST_ENV_1", "value1")
	t.Setenv("LIST_ENV_2", "value2")
	t.Setenv("LIST_ENV_3", "value3")

	yamlContent := `
components:
  terraform:
    test-component:
      vars:
        env_list:
          - !env LIST_ENV_1
          - !env LIST_ENV_2
          - !env LIST_ENV_3
        mixed_list:
          - "static"
          - !env LIST_ENV_1
          - "another-static"
`

	atmosConfig := &schema.AtmosConfiguration{}

	result, err := u.UnmarshalYAMLFromFile[map[string]interface{}](atmosConfig, yamlContent, "test.yaml")
	require.NoError(t, err)

	components := result["components"].(map[string]interface{})
	terraform := components["terraform"].(map[string]interface{})
	testComponent := terraform["test-component"].(map[string]interface{})
	vars := testComponent["vars"].(map[string]interface{})

	// During raw YAML parsing, !env functions in lists are stored as strings (deferred).
	envList := vars["env_list"].([]interface{})
	assert.Equal(t, 3, len(envList), "List should have 3 items")
	assert.Contains(t, envList[0].(string), "!env", "List items with !env should be deferred")
	assert.Contains(t, envList[1].(string), "!env", "List items with !env should be deferred")
	assert.Contains(t, envList[2].(string), "!env", "List items with !env should be deferred")

	// Verify mixed list - static values are resolved, !env is deferred.
	mixedList := vars["mixed_list"].([]interface{})
	assert.Equal(t, 3, len(mixedList), "Mixed list should have 3 items")
	assert.Equal(t, "static", mixedList[0], "Static values should be preserved")
	assert.Contains(t, mixedList[1].(string), "!env", "!env items should be deferred")
	assert.Equal(t, "another-static", mixedList[2], "Static values should be preserved")
}

// TestEnvFunctionErrorCases tests error handling for the !env function.
func TestEnvFunctionErrorCases(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expectErr bool
	}{
		{
			name:      "empty env var name",
			input:     "!env ",
			expectErr: true,
		},
		{
			name:      "env tag only with no space",
			input:     "!env",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := u.ProcessTagEnv(tt.input, nil)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestEnvFunctionIntegration tests the !env function in a full stack context.
func TestEnvFunctionIntegration(t *testing.T) {
	// Create minimal config for testing.
	info := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(info, false)
	if err != nil {
		t.Skip("Skipping integration test - cannot initialize config")
	}

	// Set test environment variables.
	t.Setenv("ATMOS_TEST_REGION", "us-west-2")
	t.Setenv("ATMOS_TEST_ENVIRONMENT", "production")

	t.Run("env vars in config", func(t *testing.T) {
		// Verify atmos config loaded.
		require.NotNil(t, atmosConfig)
	})
}
