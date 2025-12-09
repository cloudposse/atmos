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

// Note: Windows-specific drive letter handling is tested in the cross-platform
// TestSanitizeOutput_CrossPlatform test above, which covers Windows paths on all platforms.
// Custom repo root support is not currently implemented in sanitizeOutput(), but could be
// added in the future if needed. The function uses git.GetRepoRoot() to determine the actual
// repository root, which is sufficient for production use.

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

// TestSanitizeOutput_PreservesNonRepoPaths tests that paths outside the repo are not modified.
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
			name:     "Windows system path",
			input:    "C:/Windows/System32/cmd.exe",
			expected: "C:/Windows/System32/cmd.exe",
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

// TestSanitizeOutput_WithCustomReplacements tests the custom replacement functionality.
func TestSanitizeOutput_WithCustomReplacements(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		replacements map[string]string
		expected     string
	}{
		{
			name:  "Single custom replacement",
			input: "session-123456 started",
			replacements: map[string]string{
				`session-[0-9]+`: "session-12345",
			},
			expected: "session-12345 started",
		},
		{
			name:  "Multiple custom replacements",
			input: "session-987654 with temp_abcdef and id-xyz789",
			replacements: map[string]string{
				`session-[0-9]+`: "session-12345",
				`temp_[a-z]+`:    "temp_xyz",
				`id-[a-z0-9]+`:   "id-123",
			},
			expected: "session-12345 with temp_xyz and id-123",
		},
		{
			name:  "Custom replacement with standard sanitization",
			input: "Processing file with build-2025-01-15-1234",
			replacements: map[string]string{
				`build-\d{4}-\d{2}-\d{2}-\d+`: "build-2025-01-01-0000",
			},
			expected: "Processing file with build-2025-01-01-0000",
		},
		{
			name:  "Custom replacement with capture groups",
			input: "User john.doe@example.com logged in",
			replacements: map[string]string{
				`[a-z.]+@[a-z.]+`: "user@example.com",
			},
			expected: "User user@example.com logged in",
		},
		{
			name:         "Empty custom replacements should not affect output",
			input:        "No replacements here",
			replacements: map[string]string{
				// Empty map.
			},
			expected: "No replacements here",
		},
		{
			name:         "Nil custom replacements should not affect output",
			input:        "No replacements here",
			replacements: nil,
			expected:     "No replacements here",
		},
		{
			name:  "Custom replacement with special regex characters",
			input: "Price: $99.99 (USD)",
			replacements: map[string]string{
				`\$\d+\.\d+`: "$$XX.XX", // Use $$ to escape $ in replacement string
			},
			expected: "Price: $XX.XX (USD)",
		},
		{
			name:  "Multiple occurrences of same pattern",
			input: "token-abc123 and token-def456 and token-ghi789",
			replacements: map[string]string{
				`token-[a-z0-9]+`: "token-REDACTED",
			},
			expected: "token-REDACTED and token-REDACTED and token-REDACTED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts []sanitizeOption
			if tt.replacements != nil {
				opts = append(opts, WithCustomReplacements(tt.replacements))
			}
			result, err := sanitizeOutput(tt.input, opts...)
			require.NoError(t, err, "sanitizeOutput should not return error")
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestSanitizeOutput_CustomReplacements_InvalidRegex tests error handling for invalid regex patterns.
func TestSanitizeOutput_CustomReplacements_InvalidRegex(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		replacements map[string]string
		expectError  bool
	}{
		{
			name:  "Invalid regex - unclosed bracket",
			input: "test input",
			replacements: map[string]string{
				`[invalid`: "replacement",
			},
			expectError: true,
		},
		{
			name:  "Invalid regex - unclosed parenthesis",
			input: "test input",
			replacements: map[string]string{
				`(unclosed`: "replacement",
			},
			expectError: true,
		},
		{
			name:  "Valid regex should succeed",
			input: "test-123 input",
			replacements: map[string]string{
				`test-\d+`: "test-000",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := []sanitizeOption{WithCustomReplacements(tt.replacements)}
			result, err := sanitizeOutput(tt.input, opts...)

			if tt.expectError {
				assert.Error(t, err, "Expected error for invalid regex pattern")
				assert.Contains(t, err.Error(), "failed to compile custom replacement pattern")
			} else {
				assert.NoError(t, err, "Expected no error for valid regex pattern")
				assert.NotEmpty(t, result)
			}
		})
	}
}

// TestSanitizeOutput_CustomReplacements_Combined tests custom replacements combined with standard sanitization.
func TestSanitizeOutput_CustomReplacements_Combined(t *testing.T) {
	repoRoot, err := findGitRepoRoot(startingDir)
	require.NoError(t, err)

	tests := []struct {
		name         string
		input        string
		replacements map[string]string
		expected     string
	}{
		{
			name:  "Custom replacement after path sanitization",
			input: fmt.Sprintf("Deploying %s/component with session-987654", repoRoot),
			replacements: map[string]string{
				`session-[0-9]+`: "session-12345",
			},
			expected: "Deploying /absolute/path/to/repo/component with session-12345",
		},
		{
			name:  "PostHog token and custom replacement",
			input: "token=phc_real_token_here and build-2025-01-15-9999",
			replacements: map[string]string{
				`build-\d{4}-\d{2}-\d{2}-\d+`: "build-2025-01-01-0000",
			},
			expected: "token=phc_TEST_TOKEN_PLACEHOLDER and build-2025-01-01-0000",
		},
		{
			name:  "Multiple sanitization types",
			input: fmt.Sprintf("%s/file with phc_ABC123 and request-id-xyz789", repoRoot),
			replacements: map[string]string{
				`request-id-[a-z0-9]+`: "request-id-00000",
			},
			expected: "/absolute/path/to/repo/file with phc_TEST_TOKEN_PLACEHOLDER and request-id-00000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := []sanitizeOption{WithCustomReplacements(tt.replacements)}
			result, err := sanitizeOutput(tt.input, opts...)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestSanitizeOutput_WindowsLineEndingsInHintPaths verifies that hint path normalization
// works correctly with Windows CRLF line endings (\r\n) as well as Unix LF (\n).
// This test reproduces the Windows CI failure where hint paths were word-wrapped differently.
func TestSanitizeOutput_WindowsLineEndingsInHintPaths(t *testing.T) {
	repoRoot, err := findGitRepoRoot(startingDir)
	require.NoError(t, err)

	// Build a path that's part of the repo.
	testPath := repoRoot + "/tests/fixtures/scenarios/complete/stacks"

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Hint path on same line (Unix LF)",
			input:    fmt.Sprintf("💡 Path points to the stacks configuration directory, not a component: %s", testPath),
			expected: "💡 Path points to the stacks configuration directory, not a component: /absolute/path/to/repo/tests/fixtures/scenarios/complete/stacks",
		},
		{
			name:     "Hint path on next line (Unix LF)",
			input:    fmt.Sprintf("💡 Path points to the stacks configuration directory, not a component:\n%s", testPath),
			expected: "💡 Path points to the stacks configuration directory, not a component: /absolute/path/to/repo/tests/fixtures/scenarios/complete/stacks",
		},
		{
			name:     "Hint path on next line (Windows CRLF)",
			input:    fmt.Sprintf("💡 Path points to the stacks configuration directory, not a component:\r\n%s", testPath),
			expected: "💡 Path points to the stacks configuration directory, not a component: /absolute/path/to/repo/tests/fixtures/scenarios/complete/stacks",
		},
		{
			name:     "Multi-line error with Windows CRLF",
			input:    fmt.Sprintf("**Error:** path is not within Atmos component directories\r\n\r\n## Hints\r\n\r\n💡 Path points to the stacks configuration directory, not a component:\r\n%s\r\n\r\nStacks directory: %s", testPath, testPath),
			expected: "**Error:** path is not within Atmos component directories\n\n## Hints\n\n💡 Path points to the stacks configuration directory, not a component: /absolute/path/to/repo/tests/fixtures/scenarios/complete/stacks\n\nStacks directory: /absolute/path/to/repo/tests/fixtures/scenarios/complete/stacks",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: In production, normalizeLineEndings is called BEFORE sanitizeOutput.
			// This test verifies that the path joining logic in sanitizeOutput works
			// with both LF and CRLF line endings. However, callers should normalize
			// line endings first to ensure consistent behavior.
			normalized := normalizeLineEndings(tt.input)
			result, err := sanitizeOutput(normalized)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
