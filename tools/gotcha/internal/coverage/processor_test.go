package coverage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractPackageFromPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "full path with prefix",
			path:     "github.com/cloudposse/atmos/tools/gotcha/pkg/cache/cache.go",
			expected: "pkg/cache",
		},
		{
			name:     "path without prefix",
			path:     "internal/coverage/processor.go",
			expected: "internal/coverage",
		},
		{
			name:     "root level file",
			path:     "main.go",
			expected: "main",
		},
		{
			name:     "dot directory",
			path:     "./file.go",
			expected: "main",
		},
		{
			name:     "nested package",
			path:     "github.com/cloudposse/atmos/tools/gotcha/pkg/ci/github/client.go",
			expected: "pkg/ci/github",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractPackageFromPath(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "string shorter than max",
			input:    "short",
			maxLen:   10,
			expected: "short",
		},
		{
			name:     "string equal to max",
			input:    "exactly10!",
			maxLen:   10,
			expected: "exactly10!",
		},
		{
			name:     "string longer than max",
			input:    "this is a very long string",
			maxLen:   10,
			expected: "this is...",
		},
		{
			name:     "empty string",
			input:    "",
			maxLen:   5,
			expected: "",
		},
		{
			name:     "very small max length",
			input:    "hello",
			maxLen:   3,
			expected: "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateString(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestShortenPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "long path",
			path:     "very/long/path/to/some/file.go",
			expected: ".../some/file.go",
		},
		{
			name:     "short path",
			path:     "pkg/file.go",
			expected: "pkg/file.go",
		},
		{
			name:     "exactly 3 parts",
			path:     "one/two/three",
			expected: "one/two/three",
		},
		{
			name:     "single file",
			path:     "file.go",
			expected: "file.go",
		},
		{
			name:     "4 parts path",
			path:     "one/two/three/four",
			expected: ".../three/four",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shortenPath(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestShouldExcludeMocks(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		expected bool
	}{
		{
			name:     "contains mock pattern",
			patterns: []string{"test", "mock", "vendor"},
			expected: true,
		},
		{
			name:     "contains mock in pattern",
			patterns: []string{"test", "**/mock_*.go", "vendor"},
			expected: true,
		},
		{
			name:     "no mock patterns",
			patterns: []string{"test", "vendor", "generated"},
			expected: false,
		},
		{
			name:     "empty patterns",
			patterns: []string{},
			expected: false,
		},
		{
			name:     "mock as substring",
			patterns: []string{"mocking", "unmocked"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldExcludeMocks(tt.patterns)
			assert.Equal(t, tt.expected, result)
		})
	}
}