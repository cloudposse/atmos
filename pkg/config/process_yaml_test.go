package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessYAMLFunctionString(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		setupEnv    func()
		expectError bool
		validate    func(t *testing.T, result string)
	}{
		{
			name:  "literal string without YAML function",
			input: "/path/to/directory",
			validate: func(t *testing.T, result string) {
				assert.Equal(t, "/path/to/directory", result)
			},
		},
		{
			name:  "empty string",
			input: "",
			validate: func(t *testing.T, result string) {
				assert.Equal(t, "", result)
			},
		},
		{
			name:  "string with whitespace",
			input: "  /path/to/directory  ",
			validate: func(t *testing.T, result string) {
				assert.Equal(t, "/path/to/directory", result)
			},
		},
		{
			name:  "!repo-root function",
			input: "!repo-root",
			setupEnv: func() {
				// Set TEST_GIT_ROOT for testing
				wd, _ := os.Getwd()
				t.Setenv("TEST_GIT_ROOT", wd)
			},
			validate: func(t *testing.T, result string) {
				// Should return an absolute path
				assert.True(t, filepath.IsAbs(result), "Expected absolute path, got: %s", result)
				// Should not contain the literal "!repo-root"
				assert.NotContains(t, result, "!repo-root")
			},
		},
		{
			name:  "!repo-root with trailing space",
			input: "!repo-root ",
			setupEnv: func() {
				wd, _ := os.Getwd()
				t.Setenv("TEST_GIT_ROOT", wd)
			},
			validate: func(t *testing.T, result string) {
				assert.True(t, filepath.IsAbs(result))
				assert.NotContains(t, result, "!repo-root")
			},
		},
		{
			name:  "!env function with existing variable",
			input: "!env TEST_VAR",
			setupEnv: func() {
				t.Setenv("TEST_VAR", "/custom/path")
			},
			validate: func(t *testing.T, result string) {
				assert.Equal(t, "/custom/path", result)
			},
		},
		{
			name:  "!env function with non-existent variable",
			input: "!env NONEXISTENT_VAR",
			validate: func(t *testing.T, result string) {
				// Should return empty string for non-existent env var
				assert.Equal(t, "", result)
			},
		},
		{
			name:  "!env function with whitespace",
			input: "!env TEST_VAR  ",
			setupEnv: func() {
				t.Setenv("TEST_VAR", "/another/path")
			},
			validate: func(t *testing.T, result string) {
				assert.Equal(t, "/another/path", result)
			},
		},
		{
			name:  "literal string that looks like YAML function but isn't",
			input: "repo-root",
			validate: func(t *testing.T, result string) {
				assert.Equal(t, "repo-root", result)
			},
		},
		{
			name:  "string starting with exclamation but not a known function",
			input: "!unknown-function",
			validate: func(t *testing.T, result string) {
				// Should return as-is since it's not a recognized function
				assert.Equal(t, "!unknown-function", result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment if needed
			if tt.setupEnv != nil {
				tt.setupEnv()
			}

			// Cleanup after test
			// Run the function
			result, err := ProcessYAMLFunctionString(tt.input)

			// Check error expectation
			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Validate result
			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestProcessYAMLFunctionString_RepoRoot(t *testing.T) {
	// Test specifically that !repo-root resolves correctly
	t.Run("resolves to current working directory in test", func(t *testing.T) {
		wd, err := os.Getwd()
		require.NoError(t, err)

		// Set TEST_GIT_ROOT to current directory
		t.Setenv("TEST_GIT_ROOT", wd)

		result, err := ProcessYAMLFunctionString("!repo-root")
		require.NoError(t, err)

		// Should resolve to the TEST_GIT_ROOT value
		assert.Equal(t, wd, result)
	})
}

func TestProcessYAMLFunctionString_EnvVarChaining(t *testing.T) {
	// Test that we can use !env to reference another path
	t.Run("environment variable with path", func(t *testing.T) {
		testPath := "/infrastructure/environments/prod"
		t.Setenv("INFRA_BASE", testPath)

		result, err := ProcessYAMLFunctionString("!env INFRA_BASE")
		require.NoError(t, err)
		assert.Equal(t, testPath, result)
	})
}
