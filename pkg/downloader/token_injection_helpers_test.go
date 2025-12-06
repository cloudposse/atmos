package downloader

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestIsSupportedHost tests the isSupportedHost pure function.
func TestIsSupportedHost(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		expected bool
	}{
		{
			name:     "GitHub is supported",
			host:     "github.com",
			expected: true,
		},
		{
			name:     "GitLab is supported",
			host:     "gitlab.com",
			expected: true,
		},
		{
			name:     "Bitbucket is supported",
			host:     "bitbucket.org",
			expected: true,
		},
		{
			name:     "Unknown host is not supported",
			host:     "example.com",
			expected: false,
		},
		{
			name:     "Empty host is not supported",
			host:     "",
			expected: false,
		},
		{
			name:     "Custom GitLab host is not supported",
			host:     "gitlab.example.com",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSupportedHost(tt.host)
			assert.Equal(t, tt.expected, result, "isSupportedHost(%q) should return %v", tt.host, tt.expected)
		})
	}
}

// TestShouldInjectTokenForHost tests the shouldInjectTokenForHost pure function.
func TestShouldInjectTokenForHost(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		settings schema.AtmosSettings
		expected bool
	}{
		{
			name: "GitHub with inject enabled",
			host: hostGitHub,
			settings: schema.AtmosSettings{
				InjectGithubToken: true,
			},
			expected: true,
		},
		{
			name: "GitHub with inject disabled",
			host: hostGitHub,
			settings: schema.AtmosSettings{
				InjectGithubToken: false,
			},
			expected: false,
		},
		{
			name: "Bitbucket with inject enabled",
			host: hostBitbucket,
			settings: schema.AtmosSettings{
				InjectBitbucketToken: true,
			},
			expected: true,
		},
		{
			name: "Bitbucket with inject disabled",
			host: hostBitbucket,
			settings: schema.AtmosSettings{
				InjectBitbucketToken: false,
			},
			expected: false,
		},
		{
			name: "GitLab with inject enabled",
			host: hostGitLab,
			settings: schema.AtmosSettings{
				InjectGitlabToken: true,
			},
			expected: true,
		},
		{
			name: "GitLab with inject disabled",
			host: hostGitLab,
			settings: schema.AtmosSettings{
				InjectGitlabToken: false,
			},
			expected: false,
		},
		{
			name: "Unknown host always returns false",
			host: "example.com",
			settings: schema.AtmosSettings{
				InjectGithubToken:    true,
				InjectBitbucketToken: true,
				InjectGitlabToken:    true,
			},
			expected: false,
		},
		{
			name:     "Empty host returns false",
			host:     "",
			settings: schema.AtmosSettings{},
			expected: false,
		},
		{
			name: "All providers enabled - GitHub",
			host: hostGitHub,
			settings: schema.AtmosSettings{
				InjectGithubToken:    true,
				InjectBitbucketToken: true,
				InjectGitlabToken:    true,
			},
			expected: true,
		},
		{
			name: "All providers enabled - Bitbucket",
			host: hostBitbucket,
			settings: schema.AtmosSettings{
				InjectGithubToken:    true,
				InjectBitbucketToken: true,
				InjectGitlabToken:    true,
			},
			expected: true,
		},
		{
			name: "All providers enabled - GitLab",
			host: hostGitLab,
			settings: schema.AtmosSettings{
				InjectGithubToken:    true,
				InjectBitbucketToken: true,
				InjectGitlabToken:    true,
			},
			expected: true,
		},
		{
			name: "All providers disabled - GitHub",
			host: hostGitHub,
			settings: schema.AtmosSettings{
				InjectGithubToken:    false,
				InjectBitbucketToken: false,
				InjectGitlabToken:    false,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldInjectTokenForHost(tt.host, &tt.settings)
			assert.Equal(t, tt.expected, result,
				"shouldInjectTokenForHost(%q, settings) should return %v", tt.host, tt.expected)
		})
	}
}

// TestNeedsTokenInjection tests the needsTokenInjection pure function.
func TestNeedsTokenInjection(t *testing.T) {
	tests := []struct {
		name     string
		setupURL func() *url.URL
		expected bool
	}{
		{
			name: "URL without credentials needs injection",
			setupURL: func() *url.URL {
				u, _ := url.Parse("https://github.com/user/repo.git")
				return u
			},
			expected: true,
		},
		{
			name: "URL with username and password does not need injection",
			setupURL: func() *url.URL {
				u, _ := url.Parse("https://myuser:mypass@github.com/user/repo.git")
				return u
			},
			expected: false,
		},
		{
			name: "URL with username only does not need injection",
			setupURL: func() *url.URL {
				u, _ := url.Parse("https://myuser@github.com/user/repo.git")
				return u
			},
			expected: false,
		},
		{
			name: "URL with x-access-token does not need injection",
			setupURL: func() *url.URL {
				u, _ := url.Parse("https://x-access-token:ghp_token@github.com/user/repo.git")
				return u
			},
			expected: false,
		},
		{
			name: "SSH URL without credentials needs injection",
			setupURL: func() *url.URL {
				u, _ := url.Parse("ssh://github.com/user/repo.git")
				return u
			},
			expected: true,
		},
		{
			name: "SSH URL with git@ user does not need injection",
			setupURL: func() *url.URL {
				u, _ := url.Parse("ssh://git@github.com/user/repo.git")
				return u
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsedURL := tt.setupURL()
			result := needsTokenInjection(parsedURL)
			assert.Equal(t, tt.expected, result,
				"needsTokenInjection should return %v for URL: %s", tt.expected, parsedURL.String())
		})
	}
}

// TestShouldInjectTokenForHost_Consistency tests that the function is deterministic.
func TestShouldInjectTokenForHost_Consistency(t *testing.T) {
	settings := schema.AtmosSettings{
		InjectGithubToken:    true,
		InjectBitbucketToken: false,
		InjectGitlabToken:    true,
	}

	// Call multiple times to ensure consistency.
	for i := 0; i < 10; i++ {
		assert.True(t, shouldInjectTokenForHost(hostGitHub, &settings), "GitHub should always return true")
		assert.False(t, shouldInjectTokenForHost(hostBitbucket, &settings), "Bitbucket should always return false")
		assert.True(t, shouldInjectTokenForHost(hostGitLab, &settings), "GitLab should always return true")
		assert.False(t, shouldInjectTokenForHost("example.com", &settings), "Unknown host should always return false")
	}
}

// TestIsSupportedHost_CaseInsensitivity tests that host comparison handles different cases.
func TestIsSupportedHost_CaseInsensitivity(t *testing.T) {
	// Note: The Detect method converts hosts to lowercase before calling isSupportedHost,
	// so this function expects lowercase input.
	tests := []struct {
		name     string
		host     string
		expected bool
	}{
		{
			name:     "Lowercase github.com is supported",
			host:     "github.com",
			expected: true,
		},
		{
			name:     "Uppercase GitHub.com is not supported (expects lowercase)",
			host:     "GitHub.com",
			expected: false,
		},
		{
			name:     "Mixed case GitLab.com is not supported (expects lowercase)",
			host:     "GitLab.com",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSupportedHost(tt.host)
			assert.Equal(t, tt.expected, result)
		})
	}
}
