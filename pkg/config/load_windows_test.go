//go:build windows
// +build windows

package config

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// TestShouldExcludePathForTesting_WindowsCaseInsensitive verifies that Windows path matching is case-insensitive.
func TestShouldExcludePathForTesting_WindowsCaseInsensitive(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name     string
		dirPath  string
		envValue string
		expected bool
	}{
		{
			name:     "lowercase_env_uppercase_path",
			dirPath:  strings.ToUpper(tempDir),
			envValue: strings.ToLower(tempDir),
			expected: true,
		},
		{
			name:     "uppercase_env_lowercase_path",
			dirPath:  strings.ToLower(tempDir),
			envValue: strings.ToUpper(tempDir),
			expected: true,
		},
		{
			name:     "mixed_case_match",
			dirPath:  tempDir,
			envValue: strings.ToUpper(tempDir),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TEST_EXCLUDE_ATMOS_D", tt.envValue)

			result := shouldExcludePathForTesting(tt.dirPath)
			assert.Equal(t, tt.expected, result, "Windows should match paths case-insensitively")
		})
	}
}

// TestMergeDefaultImports_WindowsCaseInsensitive verifies that Windows path matching is case-insensitive for imports.
func TestMergeDefaultImports_WindowsCaseInsensitive(t *testing.T) {
	// Create temp directory.
	tempDir := t.TempDir()

	// Test case-insensitive matching by setting exclude with different case.
	upperCasePath := strings.ToUpper(tempDir)
	lowerCasePath := strings.ToLower(tempDir)

	// Set the environment variable with lowercase path.
	t.Setenv("TEST_EXCLUDE_ATMOS_D", lowerCasePath)

	// Call the function with the path in uppercase.
	v := viper.New()
	v.SetConfigType("yaml") // Set config type as done in production code.
	err := mergeDefaultImports(upperCasePath, v)

	// Should skip and return nil since paths match case-insensitively on Windows.
	assert.NoError(t, err, "Should match case-insensitively on Windows")
}
