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
// (e.g. "https://ghe.example.com:8443") is still recognized: Endpoints.Host preserves a
// non-default port (see pkg/github's hostOf), so comparing the raw URL's full Host authority
// via IsHost matches it, while a differently-ported or differently-hosted URL does not.
func TestIsGitHubURL_GHESWithPort(t *testing.T) {
	t.Setenv("GITHUB_SERVER_URL", "https://ghe.example.com:8443")

	assert.True(t, isGitHubURL("https://ghe.example.com:8443/owner/repo/blob/main/file.yaml"))
	assert.False(t, isGitHubURL("https://example.com/file.yaml"))
	assert.False(t, isGitHubURL("https://attacker.example.com/x?next=ghe.example.com:8443/file.yaml"))
	// Same host, different (default) port: RepoEndpoints().Host keeps the configured "8443",
	// so an explicit ":443" must not be treated as equivalent to it.
	assert.False(t, isGitHubURL("https://ghe.example.com:443/owner/repo/blob/main/file.yaml"))
}

// TestIsGitHubURL_GHESConfiguredPublicGitHubStillMatches pins that when GHES is configured
// (GITHUB_SERVER_URL points at a different host), a public "https://github.com/..." include is
// still recognized as a GitHub URL and converted to raw content: it is a link to public
// GitHub, unrelated to the caller's own GHES instance, and must not be silently skipped just
// because RepoEndpoints() now resolves to the GHES host.
func TestIsGitHubURL_GHESConfiguredPublicGitHubStillMatches(t *testing.T) {
	t.Setenv("GITHUB_SERVER_URL", "https://ghe.example.com")

	assert.True(t, isGitHubURL("https://github.com/owner/repo/blob/main/file.yaml"))
	assert.True(t, isGitHubURL("https://ghe.example.com/owner/repo/blob/main/file.yaml"))
	assert.False(t, isGitHubURL("https://example.com/file.yaml"))
}
