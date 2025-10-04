package downloader

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestResolveToken_GitHub tests token resolution for GitHub.
//
//nolint:dupl // Test structure intentionally similar to TestResolveToken_Bitbucket and TestResolveToken_GitLab.
func TestResolveToken_GitHub(t *testing.T) {
	tests := []struct {
		name                string
		injectGithubToken   bool
		atmosGithubToken    string
		githubToken         string
		expectedToken       string
		expectedTokenSource string
	}{
		{
			name:                "InjectGithubToken true uses ATMOS_GITHUB_TOKEN",
			injectGithubToken:   true,
			atmosGithubToken:    "atmos-token-123",
			githubToken:         "github-token-456",
			expectedToken:       "atmos-token-123",
			expectedTokenSource: "ATMOS_GITHUB_TOKEN",
		},
		{
			name:                "InjectGithubToken false uses GITHUB_TOKEN",
			injectGithubToken:   false,
			atmosGithubToken:    "atmos-token-123",
			githubToken:         "github-token-456",
			expectedToken:       "github-token-456",
			expectedTokenSource: "GITHUB_TOKEN",
		},
		{
			name:                "InjectGithubToken true with empty ATMOS_GITHUB_TOKEN uses ATMOS_GITHUB_TOKEN",
			injectGithubToken:   true,
			atmosGithubToken:    "",
			githubToken:         "github-token-456",
			expectedToken:       "",
			expectedTokenSource: "ATMOS_GITHUB_TOKEN",
		},
		{
			name:                "InjectGithubToken false with empty GITHUB_TOKEN",
			injectGithubToken:   false,
			atmosGithubToken:    "atmos-token-123",
			githubToken:         "",
			expectedToken:       "",
			expectedTokenSource: "GITHUB_TOKEN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := &CustomGitDetector{
				atmosConfig: &schema.AtmosConfiguration{
					Settings: schema.AtmosSettings{
						InjectGithubToken: tt.injectGithubToken,
						AtmosGithubToken:  tt.atmosGithubToken,
						GithubToken:       tt.githubToken,
					},
				},
			}

			token, tokenSource := detector.resolveToken(hostGitHub)
			assert.Equal(t, tt.expectedToken, token)
			assert.Equal(t, tt.expectedTokenSource, tokenSource)
		})
	}
}

// TestResolveToken_Bitbucket tests token resolution for Bitbucket.
//
//nolint:dupl // Test structure intentionally similar to TestResolveToken_GitHub and TestResolveToken_GitLab.
func TestResolveToken_Bitbucket(t *testing.T) {
	tests := []struct {
		name                 string
		injectBitbucketToken bool
		atmosBitbucketToken  string
		bitbucketToken       string
		expectedToken        string
		expectedTokenSource  string
	}{
		{
			name:                 "InjectBitbucketToken true uses ATMOS_BITBUCKET_TOKEN",
			injectBitbucketToken: true,
			atmosBitbucketToken:  "atmos-bb-token-123",
			bitbucketToken:       "bb-token-456",
			expectedToken:        "atmos-bb-token-123",
			expectedTokenSource:  "ATMOS_BITBUCKET_TOKEN",
		},
		{
			name:                 "InjectBitbucketToken false uses BITBUCKET_TOKEN",
			injectBitbucketToken: false,
			atmosBitbucketToken:  "atmos-bb-token-123",
			bitbucketToken:       "bb-token-456",
			expectedToken:        "bb-token-456",
			expectedTokenSource:  "BITBUCKET_TOKEN",
		},
		{
			name:                 "InjectBitbucketToken true with empty ATMOS_BITBUCKET_TOKEN",
			injectBitbucketToken: true,
			atmosBitbucketToken:  "",
			bitbucketToken:       "bb-token-456",
			expectedToken:        "",
			expectedTokenSource:  "ATMOS_BITBUCKET_TOKEN",
		},
		{
			name:                 "InjectBitbucketToken false with empty BITBUCKET_TOKEN",
			injectBitbucketToken: false,
			atmosBitbucketToken:  "atmos-bb-token-123",
			bitbucketToken:       "",
			expectedToken:        "",
			expectedTokenSource:  "BITBUCKET_TOKEN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := &CustomGitDetector{
				atmosConfig: &schema.AtmosConfiguration{
					Settings: schema.AtmosSettings{
						InjectBitbucketToken: tt.injectBitbucketToken,
						AtmosBitbucketToken:  tt.atmosBitbucketToken,
						BitbucketToken:       tt.bitbucketToken,
					},
				},
			}

			token, tokenSource := detector.resolveToken(hostBitbucket)
			assert.Equal(t, tt.expectedToken, token)
			assert.Equal(t, tt.expectedTokenSource, tokenSource)
		})
	}
}

// TestResolveToken_GitLab tests token resolution for GitLab.
//
//nolint:dupl // Test structure intentionally similar to TestResolveToken_GitHub and TestResolveToken_Bitbucket.
func TestResolveToken_GitLab(t *testing.T) {
	tests := []struct {
		name                string
		injectGitlabToken   bool
		atmosGitlabToken    string
		gitlabToken         string
		expectedToken       string
		expectedTokenSource string
	}{
		{
			name:                "InjectGitlabToken true uses ATMOS_GITLAB_TOKEN",
			injectGitlabToken:   true,
			atmosGitlabToken:    "atmos-gl-token-123",
			gitlabToken:         "gl-token-456",
			expectedToken:       "atmos-gl-token-123",
			expectedTokenSource: "ATMOS_GITLAB_TOKEN",
		},
		{
			name:                "InjectGitlabToken false uses GITLAB_TOKEN",
			injectGitlabToken:   false,
			atmosGitlabToken:    "atmos-gl-token-123",
			gitlabToken:         "gl-token-456",
			expectedToken:       "gl-token-456",
			expectedTokenSource: "GITLAB_TOKEN",
		},
		{
			name:                "InjectGitlabToken true with empty ATMOS_GITLAB_TOKEN",
			injectGitlabToken:   true,
			atmosGitlabToken:    "",
			gitlabToken:         "gl-token-456",
			expectedToken:       "",
			expectedTokenSource: "ATMOS_GITLAB_TOKEN",
		},
		{
			name:                "InjectGitlabToken false with empty GITLAB_TOKEN",
			injectGitlabToken:   false,
			atmosGitlabToken:    "atmos-gl-token-123",
			gitlabToken:         "",
			expectedToken:       "",
			expectedTokenSource: "GITLAB_TOKEN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := &CustomGitDetector{
				atmosConfig: &schema.AtmosConfiguration{
					Settings: schema.AtmosSettings{
						InjectGitlabToken: tt.injectGitlabToken,
						AtmosGitlabToken:  tt.atmosGitlabToken,
						GitlabToken:       tt.gitlabToken,
					},
				},
			}

			token, tokenSource := detector.resolveToken(hostGitLab)
			assert.Equal(t, tt.expectedToken, token)
			assert.Equal(t, tt.expectedTokenSource, tokenSource)
		})
	}
}

// TestResolveToken_UnknownHost tests token resolution for unknown hosts.
func TestResolveToken_UnknownHost(t *testing.T) {
	detector := &CustomGitDetector{
		atmosConfig: &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				InjectGithubToken:    true,
				AtmosGithubToken:     "atmos-token-123",
				GithubToken:          "github-token-456",
				InjectBitbucketToken: true,
				AtmosBitbucketToken:  "atmos-bb-token-123",
				BitbucketToken:       "bb-token-456",
				InjectGitlabToken:    true,
				AtmosGitlabToken:     "atmos-gl-token-123",
				GitlabToken:          "gl-token-456",
			},
		},
	}

	token, tokenSource := detector.resolveToken("example.com")
	assert.Empty(t, token, "Unknown host should return empty token")
	assert.Empty(t, tokenSource, "Unknown host should return empty token source")
}

// TestInjectToken_WithToken tests injectToken when token is available.
func TestInjectToken_WithToken(t *testing.T) {
	tests := []struct {
		name             string
		host             string
		token            string
		expectedUsername string
	}{
		{
			name:             "GitHub token injection",
			host:             hostGitHub,
			token:            "github-token-123",
			expectedUsername: "x-access-token",
		},
		{
			name:             "GitLab token injection",
			host:             hostGitLab,
			token:            "gitlab-token-456",
			expectedUsername: "oauth2",
		},
		{
			name:             "Bitbucket token injection with custom username",
			host:             hostBitbucket,
			token:            "bitbucket-token-789",
			expectedUsername: "custom-bb-user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsedURL, err := url.Parse("https://" + tt.host + "/user/repo.git")
			assert.NoError(t, err)

			detector := &CustomGitDetector{
				atmosConfig: &schema.AtmosConfiguration{
					Settings: schema.AtmosSettings{
						InjectGithubToken:    tt.host == hostGitHub,
						AtmosGithubToken:     tt.token,
						InjectGitlabToken:    tt.host == hostGitLab,
						AtmosGitlabToken:     tt.token,
						InjectBitbucketToken: tt.host == hostBitbucket,
						AtmosBitbucketToken:  tt.token,
						BitbucketUsername:    "custom-bb-user",
					},
				},
			}

			detector.injectToken(parsedURL, tt.host)

			// Verify token was injected
			assert.NotNil(t, parsedURL.User)
			username := parsedURL.User.Username()
			password, _ := parsedURL.User.Password()

			assert.Equal(t, tt.expectedUsername, username)
			assert.Equal(t, tt.token, password)
		})
	}
}

// TestInjectToken_NoToken tests injectToken when no token is available.
func TestInjectToken_NoToken(t *testing.T) {
	parsedURL, err := url.Parse("https://github.com/user/repo.git")
	assert.NoError(t, err)

	detector := &CustomGitDetector{
		atmosConfig: &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				InjectGithubToken: false,
				GithubToken:       "", // No token available
			},
		},
	}

	detector.injectToken(parsedURL, hostGitHub)

	// Verify no token was injected
	assert.Nil(t, parsedURL.User, "User should be nil when no token is available")
}

// TestInjectToken_UnknownHost tests injectToken with an unknown host.
func TestInjectToken_UnknownHost(t *testing.T) {
	parsedURL, err := url.Parse("https://example.com/user/repo.git")
	assert.NoError(t, err)

	detector := &CustomGitDetector{
		atmosConfig: &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				InjectGithubToken: true,
				AtmosGithubToken:  "github-token-123",
			},
		},
	}

	detector.injectToken(parsedURL, "example.com")

	// Verify no token was injected for unknown host
	assert.Nil(t, parsedURL.User, "User should be nil for unknown host")
}

// TestNewCustomGitDetector tests the NewCustomGitDetector constructor.
func TestNewCustomGitDetector(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			InjectGithubToken: true,
			AtmosGithubToken:  "test-token",
		},
	}
	source := "https://github.com/user/repo.git"

	detector := NewCustomGitDetector(atmosConfig, source)

	assert.NotNil(t, detector)
	assert.Equal(t, atmosConfig, detector.atmosConfig)
	assert.Equal(t, source, detector.source)
}

// TestInjectToken_AllHosts tests token injection for all supported hosts.
func TestInjectToken_AllHosts(t *testing.T) {
	hosts := []struct {
		name             string
		host             string
		expectedUsername string
	}{
		{"GitHub", hostGitHub, "x-access-token"},
		{"GitLab", hostGitLab, "oauth2"},
		{"Bitbucket", hostBitbucket, "x-token-auth"}, // Default when no BitbucketUsername set
	}

	for _, h := range hosts {
		t.Run(h.name, func(t *testing.T) {
			parsedURL, err := url.Parse("https://" + h.host + "/user/repo.git")
			assert.NoError(t, err)

			detector := &CustomGitDetector{
				atmosConfig: &schema.AtmosConfiguration{
					Settings: schema.AtmosSettings{
						InjectGithubToken:    true,
						AtmosGithubToken:     "github-token",
						InjectGitlabToken:    true,
						AtmosGitlabToken:     "gitlab-token",
						InjectBitbucketToken: true,
						AtmosBitbucketToken:  "bitbucket-token",
					},
				},
			}

			detector.injectToken(parsedURL, h.host)

			assert.NotNil(t, parsedURL.User)
			assert.Equal(t, h.expectedUsername, parsedURL.User.Username())
		})
	}
}
