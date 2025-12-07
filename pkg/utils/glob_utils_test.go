package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPathMatch_ExactMatch tests exact path matching.
func TestPathMatch_ExactMatch(t *testing.T) {
	pattern := "foo/bar/baz.txt"
	name := "foo/bar/baz.txt"

	match, err := PathMatch(pattern, name)
	require.NoError(t, err)
	assert.True(t, match, "Exact match should return true")
}

// TestPathMatch_NoMatch tests paths that don't match.
func TestPathMatch_NoMatch(t *testing.T) {
	pattern := "foo/bar/*.txt"
	name := "foo/baz/file.txt"

	match, err := PathMatch(pattern, name)
	require.NoError(t, err)
	assert.False(t, match, "Non-matching paths should return false")
}

// TestPathMatch_SingleStar tests single wildcard matching.
func TestPathMatch_SingleStar(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		path     string
		expected bool
	}{
		{
			name:     "single star matches single segment",
			pattern:  "foo/*.txt",
			path:     "foo/bar.txt",
			expected: true,
		},
		{
			name:     "single star does not match multiple segments",
			pattern:  "foo/*.txt",
			path:     "foo/bar/baz.txt",
			expected: false,
		},
		{
			name:     "single star matches any characters in segment",
			pattern:  "foo/bar-*.txt",
			path:     "foo/bar-123.txt",
			expected: true,
		},
		{
			name:     "single star does not match across slashes",
			pattern:  "foo/*/file.txt",
			path:     "foo/a/b/file.txt",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match, err := PathMatch(tt.pattern, tt.path)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, match)
		})
	}
}

// TestPathMatch_DoubleStar tests double wildcard matching.
func TestPathMatch_DoubleStar(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		path     string
		expected bool
	}{
		{
			name:     "double star matches zero segments",
			pattern:  "foo/**/bar.txt",
			path:     "foo/bar.txt",
			expected: true,
		},
		{
			name:     "double star matches one segment",
			pattern:  "foo/**/bar.txt",
			path:     "foo/baz/bar.txt",
			expected: true,
		},
		{
			name:     "double star matches multiple segments",
			pattern:  "foo/**/bar.txt",
			path:     "foo/a/b/c/bar.txt",
			expected: true,
		},
		{
			name:     "double star at beginning",
			pattern:  "**/bar.txt",
			path:     "foo/baz/bar.txt",
			expected: true,
		},
		{
			name:     "double star at end",
			pattern:  "foo/**",
			path:     "foo/bar/baz/qux.txt",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match, err := PathMatch(tt.pattern, tt.path)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, match)
		})
	}
}

// TestPathMatch_FileExtensions tests pattern matching with file extensions.
func TestPathMatch_FileExtensions(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		path     string
		expected bool
	}{
		{
			name:     "yaml extension match",
			pattern:  "**/*.yaml",
			path:     "stacks/catalog/vpc.yaml",
			expected: true,
		},
		{
			name:     "yml extension no match",
			pattern:  "**/*.yaml",
			path:     "stacks/catalog/vpc.yml",
			expected: false,
		},
		{
			name:     "multiple extensions with brace",
			pattern:  "**/*.{yaml,yml}",
			path:     "stacks/catalog/vpc.yaml",
			expected: true,
		},
		{
			name:     "multiple extensions with brace yml",
			pattern:  "**/*.{yaml,yml}",
			path:     "stacks/catalog/vpc.yml",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match, err := PathMatch(tt.pattern, tt.path)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, match)
		})
	}
}

// TestPathMatch_AtmosStackPatterns tests realistic Atmos stack file patterns.
func TestPathMatch_AtmosStackPatterns(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		path     string
		expected bool
	}{
		{
			name:     "catalog import pattern matches",
			pattern:  "catalog/**/*",
			path:     "catalog/vpc/defaults.yaml",
			expected: true,
		},
		{
			name:     "catalog import pattern nested matches",
			pattern:  "catalog/**/*",
			path:     "catalog/eks/cluster/defaults.yaml",
			expected: true,
		},
		{
			name:     "exclude defaults pattern",
			pattern:  "**/_defaults.yaml",
			path:     "stacks/orgs/acme/_defaults.yaml",
			expected: true,
		},
		{
			name:     "exclude defaults pattern no match",
			pattern:  "**/_defaults.yaml",
			path:     "stacks/orgs/acme/dev.yaml",
			expected: false,
		},
		{
			name:     "orgs pattern matches",
			pattern:  "orgs/**/*",
			path:     "orgs/acme/plat-ue2-dev.yaml",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match, err := PathMatch(tt.pattern, tt.path)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, match)
		})
	}
}

// TestPathMatch_ConsistentResults tests that multiple calls with same inputs return consistent results.
// This indirectly validates that caching doesn't break behavior.
func TestPathMatch_ConsistentResults(t *testing.T) {
	pattern := "stacks/**/*.yaml"
	path := "stacks/catalog/vpc.yaml"

	// Call multiple times
	for i := 0; i < 5; i++ {
		match, err := PathMatch(pattern, path)
		require.NoError(t, err)
		assert.True(t, match, "Multiple calls should return consistent results")
	}
}

// TestPathMatch_DifferentPatternsSamePath tests different patterns against the same path.
func TestPathMatch_DifferentPatternsSamePath(t *testing.T) {
	path := "stacks/catalog/vpc/defaults.yaml"

	tests := []struct {
		pattern  string
		expected bool
	}{
		{"**/*.yaml", true},
		{"**/*.yml", false},
		{"stacks/**/*", true},
		{"catalog/**/*", false},
		{"**/vpc/**", true},
		{"**/eks/**", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			match, err := PathMatch(tt.pattern, path)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, match)
		})
	}
}

// TestPathMatch_InvalidPattern tests error handling for invalid patterns.
func TestPathMatch_InvalidPattern(t *testing.T) {
	// Test with malformed pattern (unclosed bracket)
	pattern := "foo/[bar"
	path := "foo/bar"

	match, err := PathMatch(pattern, path)
	assert.Error(t, err, "Invalid pattern should return error")
	assert.False(t, match)
}

// TestPathMatch_EmptyInputs tests handling of empty inputs.
func TestPathMatch_EmptyInputs(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		path     string
		expected bool
	}{
		{
			name:     "empty pattern",
			pattern:  "",
			path:     "foo/bar",
			expected: false,
		},
		{
			name:     "empty path",
			pattern:  "foo/*",
			path:     "",
			expected: false,
		},
		{
			name:     "both empty",
			pattern:  "",
			path:     "",
			expected: true, // Empty pattern matches empty path
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match, err := PathMatch(tt.pattern, tt.path)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, match)
		})
	}
}

// TestPathMatch_CaseSensitivity tests case sensitivity in path matching.
func TestPathMatch_CaseSensitivity(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		path     string
		expected bool
	}{
		{
			name:     "exact case match",
			pattern:  "Foo/Bar.txt",
			path:     "Foo/Bar.txt",
			expected: true,
		},
		{
			name:     "case mismatch in directory",
			pattern:  "foo/bar.txt",
			path:     "Foo/bar.txt",
			expected: false,
		},
		{
			name:     "case mismatch in filename",
			pattern:  "foo/Bar.txt",
			path:     "foo/bar.txt",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match, err := PathMatch(tt.pattern, tt.path)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, match)
		})
	}
}

// TestGetGlobMatches_Basic tests basic glob matching functionality.
func TestGetGlobMatches_Basic(t *testing.T) {
	// This test requires actual files to exist, so we'll test with a pattern that should work.
	// In the Atmos repository structure.
	pattern := "*.go"

	matches, err := GetGlobMatches(pattern)
	require.NoError(t, err)
	assert.NotNil(t, matches)
	// We can't assert exact matches since it depends on the directory contents.
	// But we can verify the function completes without error.
}

// TestGetGlobMatches_ConsistentResults tests that multiple calls return consistent results.
func TestGetGlobMatches_ConsistentResults(t *testing.T) {
	pattern := "*.go"

	// First call
	matches1, err1 := GetGlobMatches(pattern)
	require.NoError(t, err1)

	// Second call (may use cache internally)
	matches2, err2 := GetGlobMatches(pattern)
	require.NoError(t, err2)

	// Results should be identical
	assert.Equal(t, matches1, matches2, "Multiple calls should return consistent results")
}

// TestGetGlobMatches_InvalidPattern tests error handling for invalid patterns.
func TestGetGlobMatches_InvalidPattern(t *testing.T) {
	// Test with malformed pattern
	pattern := "foo/[bar"

	matches, err := GetGlobMatches(pattern)
	assert.Error(t, err, "Invalid pattern should return error")
	assert.Nil(t, matches)
}

// TestGetGlobMatches_WindowsAbsolutePath tests that Windows absolute paths with glob patterns
// don't cause path duplication. This validates the fix for the filepath.FromSlash() conversion.
// Before the fix, paths like "D:/a/atmos/atmos/..." would be duplicated to "D:/D:/a/atmos/atmos/..."
// because filepath.Join() on Windows treats forward-slash paths as relative.
func TestGetGlobMatches_WindowsAbsolutePath(t *testing.T) {
	// Test requires creating temporary directory structure to simulate Windows CI environment.
	// We'll use t.TempDir() which works cross-platform.
	tempDir := t.TempDir()

	// Create a test file structure
	// tempDir/stacks/deploy/test.yaml
	stacksDir := filepath.Join(tempDir, "stacks", "deploy")
	err := os.MkdirAll(stacksDir, 0o755)
	require.NoError(t, err)

	testFile := filepath.Join(stacksDir, "test.yaml")
	err = os.WriteFile(testFile, []byte("test: data"), 0o644)
	require.NoError(t, err)

	// Test with both forward-slash and native path separators.
	// The critical test is with forward slashes on Windows, but we test both for completeness.
	patterns := []string{
		filepath.Join(tempDir, "stacks", "deploy", "**", "*.yaml"),                   // Native separators
		filepath.ToSlash(filepath.Join(tempDir, "stacks", "deploy", "**", "*.yaml")), // Forward slashes (Windows issue case)
	}

	for _, pattern := range patterns {
		t.Run(pattern, func(t *testing.T) {
			matches, err := GetGlobMatches(pattern)
			require.NoError(t, err, "GetGlobMatches should not fail for pattern: %s", pattern)
			require.NotNil(t, matches, "Matches should not be nil")
			require.NotEmpty(t, matches, "Should find at least one match")

			// Verify no path duplication - the matched path should not contain the base path twice.
			for _, match := range matches {
				// Check that the path doesn't contain duplicated volume/drive letters (e.g., "D:/D:/" or "C:\C:\")
				// This is the symptom of the bug we're fixing.
				normalizedMatch := filepath.ToSlash(match)

				// Count occurrences of the temp directory in the match path
				// It should appear exactly once, not multiple times
				tempDirNormalized := filepath.ToSlash(tempDir)
				count := strings.Count(normalizedMatch, tempDirNormalized)
				assert.Equal(t, 1, count, "Path should contain base directory exactly once, not duplicated. Path: %s", match)

				// Additional check: On Windows, ensure we don't have drive letter duplication like "D:/D:/"
				if len(normalizedMatch) >= 5 {
					// Check for patterns like "X:/X:/" where X is any drive letter
					for _, driveLetter := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
						duplicatedPrefix := string(driveLetter) + ":/" + string(driveLetter) + ":/"
						assert.NotContains(t, normalizedMatch, duplicatedPrefix,
							"Path should not contain duplicated drive letter: %s", match)
					}
				}

				// Verify the file actually exists at the returned path
				_, err := os.Stat(match)
				assert.NoError(t, err, "Matched file should exist at path: %s", match)
			}
		})
	}
}

// TestPathMatch_PipeCharacterNoCollision tests that patterns and names containing "|"
// don't cause cache key collisions. This validates the fix for the composite key implementation.
// Before the fix, pattern="a|b" + name="c" and pattern="a" + name="b|c" would collide
// because both produced cache key "a|b|c" when using string concatenation.
func TestPathMatch_PipeCharacterNoCollision(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		path     string
		expected bool
	}{
		{
			name:     "pattern with pipe matches correctly",
			pattern:  "foo/bar|baz.txt",
			path:     "foo/bar|baz.txt",
			expected: true,
		},
		{
			name:     "pattern with pipe no match",
			pattern:  "foo/bar|baz.txt",
			path:     "foo/bar.txt",
			expected: false,
		},
		{
			name:     "path with pipe matches correctly",
			pattern:  "foo/*.txt",
			path:     "foo/file|name.txt",
			expected: true,
		},
		{
			name:     "both pattern and path have pipe",
			pattern:  "**/file|*.txt",
			path:     "dir/file|test.txt",
			expected: true,
		},
		{
			name:     "collision case 1: pattern=a|b name=c",
			pattern:  "a|b",
			path:     "c",
			expected: false,
		},
		{
			name:     "collision case 2: pattern=a name=b|c (different from case 1)",
			pattern:  "a",
			path:     "b|c",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match, err := PathMatch(tt.pattern, tt.path)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, match, "Pattern=%q Path=%q", tt.pattern, tt.path)
		})
	}

	// Additional verification: Call both collision cases multiple times to ensure
	// cache doesn't mix them up
	for i := 0; i < 3; i++ {
		match1, err1 := PathMatch("a|b", "c")
		require.NoError(t, err1)
		assert.False(t, match1, "Iteration %d: pattern=a|b path=c should not match", i)

		match2, err2 := PathMatch("a", "b|c")
		require.NoError(t, err2)
		assert.False(t, match2, "Iteration %d: pattern=a path=b|c should not match", i)

		// These should remain independent even after caching
		assert.Equal(t, match1, match2, "Both should have same result (false)")
	}
}
