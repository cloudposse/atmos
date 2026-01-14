package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsGitHubURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{
			name:     "https github URL",
			url:      "https://github.com/owner/repo/blob/main/file.yaml",
			expected: true,
		},
		{
			name:     "http github URL",
			url:      "http://github.com/owner/repo/tree/main",
			expected: true,
		},
		{
			name:     "github scheme URL",
			url:      "github://owner/repo/file.yaml@main",
			expected: true,
		},
		{
			name:     "non-github URL",
			url:      "https://example.com/file.yaml",
			expected: false,
		},
		{
			name:     "s3 URL",
			url:      "s3://bucket/file.yaml",
			expected: false,
		},
		{
			name:     "local file path",
			url:      "./local/file.yaml",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isGitHubURL(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}
