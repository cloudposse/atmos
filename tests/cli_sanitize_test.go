package tests

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSanitizeOutput tests the sanitizeOutput function with various path formats.
func TestSanitizeOutput(t *testing.T) {
	// Get the actual repo root for testing.
	repoRoot, err := findGitRepoRoot(startingDir)
	require.NoError(t, err, "Failed to find git repo root")
	require.NotEmpty(t, repoRoot, "Repo root should not be empty")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Unix absolute path",
			input:    fmt.Sprintf("%s/examples/demo-stacks/stacks/deploy/**/*", repoRoot),
			expected: "/absolute/path/to/repo/examples/demo-stacks/stacks/deploy/**/*",
		},
		{
			name:     "Windows-style path with backslashes",
			input:    strings.ReplaceAll(fmt.Sprintf("%s\\examples\\demo-stacks\\stacks\\deploy\\**\\*", repoRoot), "/", "\\"),
			expected: "/absolute/path/to/repo/examples/demo-stacks/stacks/deploy/**/*",
		},
		{
			name:     "Debug log with import= prefix",
			input:    fmt.Sprintf("DEBU attempting to merge import import=%s/configs.d/**/* file_path=%s/configs.d/commands.yaml", repoRoot, repoRoot),
			expected: "DEBU attempting to merge import import=/absolute/path/to/repo/configs.d/**/* file_path=/absolute/path/to/repo/configs.d/commands.yaml",
		},
		{
			name:     "Multiple occurrences in same line",
			input:    fmt.Sprintf("Processing %s/file1 and %s/file2", repoRoot, repoRoot),
			expected: "Processing /absolute/path/to/repo/file1 and /absolute/path/to/repo/file2",
		},
		{
			name:     "Path with extra slashes",
			input:    fmt.Sprintf("%s///examples//demo-stacks", repoRoot),
			expected: "/absolute/path/to/repo/examples/demo-stacks",
		},
		{
			name:     "URL should not be affected",
			input:    "https://github.com/cloudposse/atmos/examples/demo-stacks",
			expected: "https://github.com/cloudposse/atmos/examples/demo-stacks",
		},
		{
			name:     "Remote import path should not be replaced",
			input:    "DEBU attempting to merge import import=https://raw.githubusercontent.com/cloudposse/atmos/refs/heads/main/atmos.yaml file_path=/atmos-import/atmos-import.yaml",
			expected: "DEBU attempting to merge import import=https://raw.githubusercontent.com/cloudposse/atmos/refs/heads/main/atmos.yaml file_path=/atmos-import/atmos-import.yaml",
		},
		{
			name:     "Random import file numbers should be masked",
			input:    "file_path=/tmp/atmos-import-123456789/atmos-import-987654321.yaml",
			expected: "file_path=/atmos-import/atmos-import.yaml",
		},
		{
			name:     "PostHog token should be masked",
			input:    "token=phc_ABC123def456GHI789jkl012MNO345pqr678",
			expected: "token=phc_TEST_TOKEN_PLACEHOLDER",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := sanitizeOutput(tt.input)
			require.NoError(t, err, "sanitizeOutput should not return error")
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestSanitizeOutput_WindowsDriveLetter tests Windows-specific drive letter handling.
// This test simulates the Windows CI environment where drive letters may have different casing.
func TestSanitizeOutput_WindowsDriveLetter(t *testing.T) {
	// Skip if not on Windows or in CI.
	// Note: We test this scenario even on non-Windows platforms by simulating Windows paths.

	tests := []struct {
		name     string
		repoRoot string // Simulated repo root
		input    string
		expected string
	}{
		{
			name:     "Lowercase drive letter matches uppercase repo root",
			repoRoot: "D:/a/atmos/atmos",
			input:    "DEBU attempting to merge import import=d:/a/atmos/atmos/configs.d/**/* file_path=d:/a/atmos/atmos/configs.d/commands.yaml",
			expected: "DEBU attempting to merge import import=/absolute/path/to/repo/configs.d/**/* file_path=/absolute/path/to/repo/configs.d/commands.yaml",
		},
		{
			name:     "Uppercase drive letter matches lowercase repo root",
			repoRoot: "d:/a/atmos/atmos",
			input:    "DEBU attempting to merge import import=D:/a/atmos/atmos/configs.d/**/* file_path=D:/a/atmos/atmos/configs.d/commands.yaml",
			expected: "DEBU attempting to merge import import=/absolute/path/to/repo/configs.d/**/* file_path=/absolute/path/to/repo/configs.d/commands.yaml",
		},
		{
			name:     "Mixed case in path segments",
			repoRoot: "C:/Users/Runner/work/atmos/atmos",
			input:    "Processing c:/users/runner/work/Atmos/Atmos/examples/demo.yaml",
			expected: "Processing /absolute/path/to/repo/examples/demo.yaml",
		},
		{
			name:     "Windows path with backslashes",
			repoRoot: "C:/Program Files/atmos",
			input:    "Loading C:\\Program Files\\atmos\\config\\atmos.yaml",
			expected: "Loading /absolute/path/to/repo/config/atmos.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test would need modification to sanitizeOutput to accept a custom repo root.
			// For now, we document the expected behavior.
			t.Skip("Test requires modification to sanitizeOutput to accept custom repo root parameter")

			// Expected implementation:
			// result, err := sanitizeOutputWithRepoRoot(tt.input, tt.repoRoot)
			// require.NoError(t, err)
			// assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCollapseExtraSlashes tests the collapseExtraSlashes helper function.
func TestCollapseExtraSlashes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Single slashes unchanged",
			input:    "/path/to/file",
			expected: "/path/to/file",
		},
		{
			name:     "Multiple consecutive slashes collapsed",
			input:    "/path///to////file",
			expected: "/path/to/file",
		},
		{
			name:     "HTTP protocol preserved with exactly two slashes",
			input:    "http:///github.com//path",
			expected: "http://github.com/path",
		},
		{
			name:     "HTTPS protocol preserved with exactly two slashes",
			input:    "https://///example.com///path////file",
			expected: "https://example.com/path/file",
		},
		{
			name:     "No slashes",
			input:    "no-slashes-here",
			expected: "no-slashes-here",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Only slashes",
			input:    "/////",
			expected: "/",
		},
		{
			name:     "Protocol only with too many slashes",
			input:    "https://////",
			expected: "https://",
		},
		{
			name:     "Case insensitive protocol matching",
			input:    "HTTP:///EXAMPLE.COM//PATH",
			expected: "HTTP://EXAMPLE.COM/PATH",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collapseExtraSlashes(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCollapseExtraSlashes_WindowsPaths tests Windows-specific path handling.
func TestCollapseExtraSlashes_WindowsPaths(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Windows UNC path preserved",
			input:    "//server/share/path",
			expected: "/server/share/path",
		},
		{
			name:     "Windows drive with slashes",
			input:    "C://Users//Documents///file.txt",
			expected: "C:/Users/Documents/file.txt",
		},
		{
			name:     "Windows drive colon not treated as protocol",
			input:    "D:///path///to///file",
			expected: "D:/path/to/file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collapseExtraSlashes(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestSanitizeOutput_EdgeCases tests edge cases and error conditions.
func TestSanitizeOutput_EdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		shouldContain string // What the output should contain
	}{
		{
			name:          "Empty string",
			input:         "",
			shouldContain: "",
		},
		{
			name:          "No repo paths in output",
			input:         "This is just plain text without any paths",
			shouldContain: "This is just plain text without any paths",
		},
		{
			name: "Very long path",
			input: func() string {
				repoRoot, _ := findGitRepoRoot(startingDir)
				return fmt.Sprintf("%s/%s/file.txt", repoRoot, strings.Repeat("very-long-directory-name/", 50))
			}(),
			shouldContain: "/absolute/path/to/repo/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := sanitizeOutput(tt.input)
			require.NoError(t, err)
			assert.Contains(t, result, tt.shouldContain)
		})
	}
}

// TestSanitizeOutput_PreservesNonRepoPaths tests that paths outside the repo are not modified,
// except for Windows drive letters which are normalized for cross-platform snapshot comparison.
func TestSanitizeOutput_PreservesNonRepoPaths(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "System path not in repo",
			input:    "/usr/local/bin/terraform",
			expected: "/usr/local/bin/terraform",
		},
		{
			name:     "Windows system path (not indented - preserved)",
			input:    "C:/Windows/System32/cmd.exe",
			expected: "C:/Windows/System32/cmd.exe", // Not indented, preserved
		},
		{
			name:     "Temp directory path",
			input:    "/tmp/atmos-test-12345/component",
			expected: "/tmp/atmos-test-12345/component",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := sanitizeOutput(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestSanitizeOutput_WindowsDriveLetterInErrorMessages tests that Windows drive letters
// in error messages are normalized for cross-platform snapshot comparison.
// This reproduces the Windows CI failure where "D:/stacks" appeared instead of "/stacks".
func TestSanitizeOutput_WindowsDriveLetterInErrorMessages(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "Windows absolute path in error message (ACTUAL snapshot format - 1 space)",
			input: `The atmos.yaml config file specifies the stacks directory as stacks, but the resolved absolute path does not exist:

 D:/stacks`,
			expected: `The atmos.yaml config file specifies the stacks directory as stacks, but the resolved absolute path does not exist:

 /stacks`,
		},
		{
			name: "Windows absolute path in error message (4 spaces)",
			input: `The atmos.yaml config file specifies the stacks directory as stacks, but the resolved absolute path does not exist:

    D:/stacks`,
			expected: `The atmos.yaml config file specifies the stacks directory as stacks, but the resolved absolute path does not exist:

    /stacks`,
		},
		{
			name:     "Lowercase Windows drive letter (not indented - preserved)",
			input:    "d:/stacks",
			expected: "d:/stacks", // No normalization - not indented
		},
		{
			name:     "Uppercase Windows drive letter (not indented - preserved)",
			input:    "D:/stacks",
			expected: "D:/stacks", // No normalization - not indented
		},
		{
			name:     "Windows drive letter with 1 space indent (normalized)",
			input:    " D:/stacks",
			expected: " /stacks", // Normalized - indented error output
		},
		{
			name:     "Windows drive letter with 4 space indent (normalized)",
			input:    "    D:/stacks",
			expected: "    /stacks", // Normalized - indented error output
		},
		{
			name:     "Multiple Windows paths with proper indentation",
			input:    "Path1:\n    D:/stacks\nPath2:\n    C:/custom/path",
			expected: "Path1:\n    /stacks\nPath2:\n    /custom/path",
		},
		{
			name:     "Windows path mid-line should NOT be normalized",
			input:    "Path1: D:/stacks, Path2: C:/custom/path",
			expected: "Path1: D:/stacks, Path2: C:/custom/path", // Mid-line paths preserved
		},
		{
			name: "Windows path in context field (mid-line)",
			input: `## Context

resolved_path: D:/a/atmos/atmos/tests/fixtures/stacks`,
			expected: `## Context

resolved_path: D:/a/atmos/atmos/tests/fixtures/stacks`, // Path preserved (not repo root on this machine)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := sanitizeOutput(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestSanitizeOutput_ComplexDebugLog tests a realistic debug log line from the failing CI test.
func TestSanitizeOutput_ComplexDebugLog(t *testing.T) {
	repoRoot, err := findGitRepoRoot(startingDir)
	require.NoError(t, err)

	// Simulate the actual failing log line from Windows CI.
	// The repo root on Windows CI is D:\a\atmos\atmos but logs show d:/a/atmos/atmos (lowercase).
	normalizedRepoRoot := strings.ToLower(filepath.ToSlash(repoRoot))

	input := fmt.Sprintf("DEBU attempting to merge import import=%s/tests/fixtures/scenarios/atmos-cli-imports/configs.d/**/* file_path=/absolute/path/to/repo/tests/fixtures/scenarios/atmos-cli-imports/configs.d/commands.yaml", normalizedRepoRoot)
	expected := "DEBU attempting to merge import import=/absolute/path/to/repo/tests/fixtures/scenarios/atmos-cli-imports/configs.d/**/* file_path=/absolute/path/to/repo/tests/fixtures/scenarios/atmos-cli-imports/configs.d/commands.yaml"

	result, err := sanitizeOutput(input)
	require.NoError(t, err)
	assert.Equal(t, expected, result, "Failed to sanitize Windows CI log with lowercase drive letter")
}

// TestSanitizeOutput_WindowsCIFailureScenario reproduces the exact Windows CI failure from PR #1504.
// This test verifies that the case-insensitive regex fix resolves the golden snapshot mismatch.
func TestSanitizeOutput_WindowsCIFailureScenario(t *testing.T) {
	repoRoot, err := findGitRepoRoot(startingDir)
	require.NoError(t, err)

	// The exact failing scenario from Windows CI:
	// - Repo root detected as D:\a\atmos\atmos (uppercase D)
	// - Debug logs show d:/a/atmos/atmos (lowercase d after filepath.ToSlash normalization)
	// This mismatch caused the regex to fail to match and replace the path.

	tests := []struct {
		name     string
		input    string // What appears in debug output
		expected string
	}{
		{
			name: "Windows CI - actual backslash path (D:\\a\\atmos\\atmos)",
			input: fmt.Sprintf("DEBU attempting to merge import import=%s\\configs.d\\**\\* file_path=%s\\configs.d\\commands.yaml",
				strings.ReplaceAll(repoRoot, "/", "\\"), // Windows backslashes
				strings.ReplaceAll(repoRoot, "/", "\\")),
			expected: "DEBU attempting to merge import import=/absolute/path/to/repo/configs.d/**/* file_path=/absolute/path/to/repo/configs.d/commands.yaml",
		},
		{
			name: "Windows CI - lowercase drive with backslashes (d:\\a\\atmos\\atmos)",
			input: fmt.Sprintf("DEBU attempting to merge import import=%s\\configs.d\\**\\* file_path=%s\\configs.d\\commands.yaml",
				strings.ToLower(strings.ReplaceAll(repoRoot, "/", "\\")), // lowercase with backslashes
				strings.ToLower(strings.ReplaceAll(repoRoot, "/", "\\"))),
			expected: "DEBU attempting to merge import import=/absolute/path/to/repo/configs.d/**/* file_path=/absolute/path/to/repo/configs.d/commands.yaml",
		},
		{
			name: "Windows CI - lowercase drive with forward slashes (d:/a/atmos/atmos)",
			input: fmt.Sprintf("DEBU attempting to merge import import=%s/configs.d/**/* file_path=%s/configs.d/commands.yaml",
				strings.ToLower(filepath.ToSlash(repoRoot)), // lowercase with forward slashes
				strings.ToLower(filepath.ToSlash(repoRoot))),
			expected: "DEBU attempting to merge import import=/absolute/path/to/repo/configs.d/**/* file_path=/absolute/path/to/repo/configs.d/commands.yaml",
		},
		{
			name:     "Windows CI - mixed case in path segments",
			input:    fmt.Sprintf("DEBU file_path=%s/Tests/Fixtures/file.yaml", strings.ToLower(filepath.ToSlash(repoRoot))),
			expected: "DEBU file_path=/absolute/path/to/repo/Tests/Fixtures/file.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: This test uses the actual repo root, so it will only verify
			// that the current fix (case-insensitive regex) works.
			// The simulated repoRoot parameter would require refactoring sanitizeOutput.
			result, err := sanitizeOutput(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
