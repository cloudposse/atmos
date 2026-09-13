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

// TestIsGitHubURL_GHESWithPort pins that a GITHUB_SERVER_URL carrying a non-default port
// (e.g. "https://ghe.example.com:8443") is still recognized: RepoEndpoints().Host never
// carries a port (it is derived from url.URL.Hostname()), so a literal string-prefix match
// against the raw URL (which does carry the port) would always miss it.
func TestIsGitHubURL_GHESWithPort(t *testing.T) {
	t.Setenv("GITHUB_SERVER_URL", "https://ghe.example.com:8443")

	assert.True(t, isGitHubURL("https://ghe.example.com:8443/owner/repo/blob/main/file.yaml"))
	assert.False(t, isGitHubURL("https://example.com/file.yaml"))
	assert.False(t, isGitHubURL("https://attacker.example.com/x?next=ghe.example.com:8443/file.yaml"))
}
