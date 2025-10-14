package utils

import (
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
// This indirectly validates that caching (P5.1 optimization) doesn't break behavior.
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
	// This test requires actual files to exist, so we'll test with a pattern that should work
	// in the Atmos repository structure
	pattern := "*.go"

	matches, err := GetGlobMatches(pattern)
	require.NoError(t, err)
	assert.NotNil(t, matches)
	// We can't assert exact matches since it depends on the directory contents
	// but we can verify the function completes without error
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
