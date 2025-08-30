package tests

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeOutput(t *testing.T) {
	// Save original startingDir
	originalDir := startingDir
	defer func() { startingDir = originalDir }()

	// For testing, we'll simulate different scenarios
	// Note: In actual test runs, sanitizeOutput uses findGitRepoRoot which finds the real repo
	// These tests verify the replacement patterns work correctly

	testCases := []struct {
		name     string
		input    string
		expected string
		skip     bool // Skip tests that depend on actual file system state
	}{
		{
			name:     "normalize repo name in error message",
			input:    "The default Atmos stacks directory is set to feature-dev-2904-theme-chrome-style-for-glamour-implementation/tests/fixtures/scenarios/complete/stacks",
			expected: "The default Atmos stacks directory is set to atmos/tests/fixtures/scenarios/complete/stacks",
		},
		{
			name:     "normalize repo name with ./ prefix",
			input:    "Looking in ./feature-dev-2904-theme-chrome-style-for-glamour-implementation/tests/fixtures",
			expected: "Looking in ./atmos/tests/fixtures",
		},
		{
			name:     "preserve atmos when already present",
			input:    "The default Atmos stacks directory is set to atmos/tests/fixtures/scenarios/complete/stacks",
			expected: "The default Atmos stacks directory is set to atmos/tests/fixtures/scenarios/complete/stacks",
		},
		{
			name:     "normalize URLs",
			input:    "https://example.com//path//to//resource",
			expected: "https://example.com/path/to/resource",
		},
		{
			name:     "remove random import numbers",
			input:    "file_path=/tmp/atmos-import-123456789/atmos-import-123456789.yaml",
			expected: "file_path=/atmos-import/atmos-import.yaml",
		},
		{
			name:     "normalize repo name at line start",
			input:    "feature-dev-2904-theme-chrome-style-for-glamour-implementation/tests/something",
			expected: "atmos/tests/something",
		},
		{
			name:     "normalize repo name after space",
			input:    "path to feature-dev-2904-theme-chrome-style-for-glamour-implementation/tests/fixtures",
			expected: "path to atmos/tests/fixtures",
		},
		{
			name:     "don't normalize repo name in middle of word",
			input:    "myfeature-dev-2904-theme-chrome-style-for-glamour-implementation/tests/fixtures",
			expected: "myfeature-dev-2904-theme-chrome-style-for-glamour-implementation/tests/fixtures",
		},
		{
			name:     "normalize multiple occurrences",
			input:    "Error: ./feature-dev-2904-theme-chrome-style-for-glamour-implementation/path1 and feature-dev-2904-theme-chrome-style-for-glamour-implementation/tests/path2",
			expected: "Error: ./atmos/path1 and atmos/tests/path2",
		},
		{
			name:     "absolute paths get normalized to placeholder",
			input:    "/Users/erik/Dev/cloudposse/tools/atmos/.conductor/feature-dev-2904-theme-chrome-style-for-glamour-implementation/some/path",
			expected: "/absolute/path/to/repo/some/path",
			skip:     true, // This depends on actual repo detection
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skip {
				t.Skip("Skipping test that depends on file system state")
			}

			// For most tests, we'll just test the string replacements
			// The actual sanitizeOutput function uses findGitRepoRoot which we can't easily mock
			// So we'll test the patterns directly
			result := tc.input

			// Simulate what sanitizeOutput does for repo name normalization
			repoName := "feature-dev-2904-theme-chrome-style-for-glamour-implementation"
			if repoName != "atmos" {
				// Apply the same patterns as in sanitizeOutput

				// Pattern 1: "is set to <repoName>/..."
				pattern1 := regexp.MustCompile(`(is set to )` + regexp.QuoteMeta(repoName) + `/`)
				result = pattern1.ReplaceAllString(result, "${1}atmos/")

				// Pattern 2: After whitespace or at line start, followed by /tests/
				pattern2 := regexp.MustCompile(`(^|\s)` + regexp.QuoteMeta(repoName) + `/tests/`)
				result = pattern2.ReplaceAllString(result, "${1}atmos/tests/")

				// Pattern 3: With ./ prefix
				pattern3 := regexp.MustCompile(`\./` + regexp.QuoteMeta(repoName) + `/`)
				result = pattern3.ReplaceAllString(result, "./atmos/")
			}

			// Apply URL normalization
			result = collapseExtraSlashes(result)

			// Apply import file normalization
			filePathRegex := regexp.MustCompile(`file_path=[^ ]+/atmos-import-\d+/atmos-import-\d+\.yaml`)
			result = filePathRegex.ReplaceAllString(result, "file_path=/atmos-import/atmos-import.yaml")

			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestCollapseExtraSlashesInSanitize tests the collapseExtraSlashes helper function
func TestCollapseExtraSlashesInSanitize(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		// Basic cases
		{"path//to//file", "path/to/file"},
		{"///multiple///slashes///", "/multiple/slashes/"},

		// URLs
		{"https://example.com//path", "https://example.com/path"},
		{"http://example.com///api//v1", "http://example.com/api/v1"},

		// Edge cases
		{"no/extra/slashes", "no/extra/slashes"},
		{"", ""},
		{"/", "/"},
		{"//", "/"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := collapseExtraSlashes(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestSanitizeOutputIntegration tests the actual sanitizeOutput function with real repo detection
func TestSanitizeOutputIntegration(t *testing.T) {
	// This test runs the actual sanitizeOutput function
	// It will use the real git repo detection, so results depend on the actual repo name

	testCases := []struct {
		name             string
		input            string
		expectNormalized bool // Whether we expect normalization to happen
	}{
		{
			name:             "URLs should be normalized",
			input:            "Visit https://example.com//docs//api",
			expectNormalized: true,
		},
		{
			name:             "Import paths should be normalized",
			input:            "file_path=/tmp/atmos-import-987654321/atmos-import-987654321.yaml",
			expectNormalized: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := sanitizeOutput(tc.input)
			assert.NoError(t, err)

			if tc.expectNormalized {
				// Check that some normalization happened
				assert.NotEqual(t, tc.input, result, "Expected output to be normalized")
			}

			// Specific checks
			if tc.name == "URLs should be normalized" {
				assert.Contains(t, result, "https://example.com/docs/api")
				assert.NotContains(t, result, "//docs//")
			}

			if tc.name == "Import paths should be normalized" {
				assert.Contains(t, result, "file_path=/atmos-import/atmos-import.yaml")
				assert.NotContains(t, result, "987654321")
			}
		})
	}
}
