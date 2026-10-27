package filesystem

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeFileName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
		skipOS   string // Skip test on this OS
	}{
		{
			name:     "simple filename",
			input:    "file.txt",
			expected: "file.txt",
		},
		{
			name:     "filename with path",
			input:    "/path/to/file.txt",
			expected: "file.txt",
		},
		{
			name:     "uri with query string",
			input:    "https://example.com/file.txt?version=1",
			expected: "file.txt",
		},
		{
			name:  "invalid windows chars - colon",
			input: "file:name.txt",
			// Colon makes it parse as URL scheme "file:", so path is empty, returns "."
			expected: ".",
		},
		{
			name:  "invalid windows chars - asterisk",
			input: "file*name.txt",
			expected: func() string {
				if runtime.GOOS == "windows" {
					return "file_name.txt"
				}
				return "file*name.txt"
			}(),
		},
		{
			name:  "invalid windows chars - question mark",
			input: "file?name.txt",
			// Question mark is query string delimiter, so returns base "file"
			expected: "file",
		},
		{
			name:  "invalid windows chars - quotes",
			input: `file"name.txt`,
			expected: func() string {
				if runtime.GOOS == "windows" {
					return "file_name.txt"
				}
				return `file"name.txt`
			}(),
		},
		{
			name:  "invalid windows chars - pipe",
			input: "file|name.txt",
			expected: func() string {
				if runtime.GOOS == "windows" {
					return "file_name.txt"
				}
				return "file|name.txt"
			}(),
		},
		{
			name:     "multiple slashes",
			input:    "path/to/nested/file.txt",
			expected: "file.txt",
		},
		{
			name:  "backslashes on windows",
			input: `path\to\file.txt`,
			// On Unix, backslashes are not path separators, so whole string is the basename
			// On Windows, they are path separators, so returns "file.txt"
			expected: func() string {
				if runtime.GOOS == "windows" {
					return "file.txt"
				}
				return `path\to\file.txt`
			}(),
		},
		{
			name:  "invalid url - fallback to base",
			input: "://invalid",
			// Falls back to filepath.Base which returns "invalid"
			expected: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.skipOS != "" && runtime.GOOS == tt.skipOS {
				t.Skipf("Skipping test on %s", tt.skipOS)
			}

			result := SanitizeFileName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
