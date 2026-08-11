package downloader

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	execpkg "github.com/cloudposse/atmos/pkg/exec"
	"github.com/cloudposse/atmos/pkg/github"
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
			name:                "ATMOS_GITHUB_TOKEN takes precedence when both are set",
			injectGithubToken:   true,
			atmosGithubToken:    "atmos-token-123",
			githubToken:         "github-token-456",
			expectedToken:       "atmos-token-123",
			expectedTokenSource: "ATMOS_GITHUB_TOKEN",
		},
		{
			name:                "Falls back to GITHUB_TOKEN when ATMOS_GITHUB_TOKEN is empty",
			injectGithubToken:   true,
			atmosGithubToken:    "",
			githubToken:         "github-token-456",
			expectedToken:       "github-token-456",
			expectedTokenSource: "GITHUB_TOKEN",
		},
		{
			name:                "Uses GITHUB_TOKEN when only GITHUB_TOKEN is set",
			injectGithubToken:   true,
			atmosGithubToken:    "",
			githubToken:         "github-token-456",
			expectedToken:       "github-token-456",
			expectedTokenSource: "GITHUB_TOKEN",
		},
		{
			name:                "Returns empty when both tokens and the CLI fallback are empty",
			injectGithubToken:   true,
			atmosGithubToken:    "",
			githubToken:         "",
			expectedToken:       "",
			expectedTokenSource: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear the live broker-set env so this test exercises struct-field precedence only,
			// and disable the gh-CLI fallback (tier 4) so this table only exercises the three
			// settings-based tiers deterministically, regardless of the host's gh session.
			t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")
			t.Setenv("ATMOS_GITHUB_CLI", "")
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

// TestHelperProcess is not a real test. It is invoked as a subprocess by fakeCLICmd to
// emulate the GitHub CLI: it prints HELPER_STDOUT and exits with HELPER_EXIT_CODE. Mirrors
// pkg/github/token_test.go's helper of the same name (each package needs its own, since
// os.Args[0] is that package's own test binary).
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	fmt.Fprint(os.Stdout, os.Getenv("HELPER_STDOUT"))
	if os.Getenv("HELPER_EXIT_CODE") == "1" {
		os.Exit(1)
	}
	os.Exit(0)
}

// fakeCLICmd returns an *exec.Cmd that re-invokes this test binary as TestHelperProcess,
// emulating a CLI that prints stdout and exits with the given code.
func fakeCLICmd(stdout string, exitCode int) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess")
	cmd.Env = append(
		os.Environ(),
		"GO_WANT_HELPER_PROCESS=1",
		"HELPER_STDOUT="+stdout,
		fmt.Sprintf("HELPER_EXIT_CODE=%d", exitCode),
	)
	return cmd
}

// TestResolveToken_GitHub_CLIFallback covers tier 4 of resolveToken's GitHub case: falling
// back to `gh auth token` (via github.GetGitHubTokenFromCLI) when none of the three
// settings-based tokens are set.
func TestResolveToken_GitHub_CLIFallback(t *testing.T) {
	newDetector := func() *CustomGitDetector {
		return &CustomGitDetector{
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{InjectGithubToken: true},
			},
		}
	}

	t.Run("falls back to gh auth token when all settings-based tokens are empty", func(t *testing.T) {
		t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")
		t.Setenv("ATMOS_GITHUB_CLI", "gh")
		ctrl := gomock.NewController(t)
		mock := execpkg.NewMockCommandExecutor(ctrl)
		mock.EXPECT().
			CommandContext(gomock.Any(), "gh", "auth", "token").
			Return(fakeCLICmd("ghp_from_cli\n", 0))
		t.Cleanup(github.SetCommanderForTesting(mock))

		token, tokenSource := newDetector().resolveToken(hostGitHub)
		assert.Equal(t, "ghp_from_cli", token)
		assert.Equal(t, "GH_CLI", tokenSource)
	})

	t.Run("ATMOS_GITHUB_CLI empty disables the fallback without invoking the commander", func(t *testing.T) {
		t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")
		t.Setenv("ATMOS_GITHUB_CLI", "")
		ctrl := gomock.NewController(t)
		// No EXPECT: any call to the commander fails the test.
		mock := execpkg.NewMockCommandExecutor(ctrl)
		t.Cleanup(github.SetCommanderForTesting(mock))

		token, tokenSource := newDetector().resolveToken(hostGitHub)
		assert.Empty(t, token)
		assert.Empty(t, tokenSource)
	})

	t.Run("an explicit GITHUB_TOKEN short-circuits the CLI fallback", func(t *testing.T) {
		t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")
		t.Setenv("ATMOS_GITHUB_CLI", "gh")
		ctrl := gomock.NewController(t)
		// No EXPECT: the commander must not be invoked when an explicit token is present.
		mock := execpkg.NewMockCommandExecutor(ctrl)
		t.Cleanup(github.SetCommanderForTesting(mock))

		detector := newDetector()
		detector.atmosConfig.Settings.GithubToken = "explicit-github-token"

		token, tokenSource := detector.resolveToken(hostGitHub)
		assert.Equal(t, "explicit-github-token", token)
		assert.Equal(t, "GITHUB_TOKEN", tokenSource)
	})
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
			name:                 "ATMOS_BITBUCKET_TOKEN takes precedence when both are set",
			injectBitbucketToken: true,
			atmosBitbucketToken:  "atmos-bb-token-123",
			bitbucketToken:       "bb-token-456",
			expectedToken:        "atmos-bb-token-123",
			expectedTokenSource:  "ATMOS_BITBUCKET_TOKEN",
		},
		{
			name:                 "Falls back to BITBUCKET_TOKEN when ATMOS_BITBUCKET_TOKEN is empty",
			injectBitbucketToken: true,
			atmosBitbucketToken:  "",
			bitbucketToken:       "bb-token-456",
			expectedToken:        "bb-token-456",
			expectedTokenSource:  "BITBUCKET_TOKEN",
		},
		{
			name:                 "Uses BITBUCKET_TOKEN when only BITBUCKET_TOKEN is set",
			injectBitbucketToken: true,
			atmosBitbucketToken:  "",
			bitbucketToken:       "bb-token-456",
			expectedToken:        "bb-token-456",
			expectedTokenSource:  "BITBUCKET_TOKEN",
		},
		{
			name:                 "Returns empty when both tokens are empty",
			injectBitbucketToken: true,
			atmosBitbucketToken:  "",
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
			name:                "ATMOS_GITLAB_TOKEN takes precedence when both are set",
			injectGitlabToken:   true,
			atmosGitlabToken:    "atmos-gl-token-123",
			gitlabToken:         "gl-token-456",
			expectedToken:       "atmos-gl-token-123",
			expectedTokenSource: "ATMOS_GITLAB_TOKEN",
		},
		{
			name:                "Falls back to GITLAB_TOKEN when ATMOS_GITLAB_TOKEN is empty",
			injectGitlabToken:   true,
			atmosGitlabToken:    "",
			gitlabToken:         "gl-token-456",
			expectedToken:       "gl-token-456",
			expectedTokenSource: "GITLAB_TOKEN",
		},
		{
			name:                "Uses GITLAB_TOKEN when only GITLAB_TOKEN is set",
			injectGitlabToken:   true,
			atmosGitlabToken:    "",
			gitlabToken:         "gl-token-456",
			expectedToken:       "gl-token-456",
			expectedTokenSource: "GITLAB_TOKEN",
		},
		{
			name:                "Returns empty when both tokens are empty",
			injectGitlabToken:   true,
			atmosGitlabToken:    "",
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
	// Disable the gh-CLI fallback (tier 4) so "no token available" is deterministic
	// regardless of whether the host running this test has an authenticated gh session.
	t.Setenv("ATMOS_GITHUB_CLI", "")

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

// TestInjectToken_UserSpecifiedCredentials tests that user-specified credentials take precedence over automatic injection.
func TestInjectToken_UserSpecifiedCredentials(t *testing.T) {
	tests := []struct {
		name             string
		urlString        string
		host             string
		expectedUsername string
		expectedPassword string
		tokenAvailable   bool
	}{
		{
			name:             "User-specified username and password take precedence over GitHub token",
			urlString:        "https://myuser:mypass@github.com/user/repo.git",
			host:             hostGitHub,
			expectedUsername: "myuser",
			expectedPassword: "mypass",
			tokenAvailable:   true,
		},
		{
			name:             "User-specified username only (no password) takes precedence",
			urlString:        "https://myuser@github.com/user/repo.git",
			host:             hostGitHub,
			expectedUsername: "myuser",
			expectedPassword: "",
			tokenAvailable:   true,
		},
		{
			name:             "User-specified token format takes precedence over automatic injection",
			urlString:        "https://x-access-token:ghp_usertoken@github.com/user/repo.git",
			host:             hostGitHub,
			expectedUsername: "x-access-token",
			expectedPassword: "ghp_usertoken",
			tokenAvailable:   true,
		},
		{
			name:             "GitLab user-specified credentials take precedence",
			urlString:        "https://myuser:glpat-usertoken@gitlab.com/user/repo.git",
			host:             hostGitLab,
			expectedUsername: "myuser",
			expectedPassword: "glpat-usertoken",
			tokenAvailable:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsedURL, err := url.Parse(tt.urlString)
			assert.NoError(t, err)

			// Store original user info for comparison
			originalUser := parsedURL.User

			detector := &CustomGitDetector{
				atmosConfig: &schema.AtmosConfiguration{
					Settings: schema.AtmosSettings{
						InjectGithubToken: tt.tokenAvailable && tt.host == hostGitHub,
						AtmosGithubToken:  "should-not-be-used",
						InjectGitlabToken: tt.tokenAvailable && tt.host == hostGitLab,
						AtmosGitlabToken:  "should-not-be-used",
					},
				},
			}

			detector.injectToken(parsedURL, tt.host)

			// Verify user credentials were NOT overwritten
			assert.NotNil(t, parsedURL.User, "User should not be nil when URL has pre-existing credentials")
			assert.Equal(t, originalUser, parsedURL.User, "User credentials should not be modified")

			username := parsedURL.User.Username()
			password, hasPassword := parsedURL.User.Password()

			assert.Equal(t, tt.expectedUsername, username, "Username should match original")
			if tt.expectedPassword != "" {
				assert.True(t, hasPassword, "Password should be present")
				assert.Equal(t, tt.expectedPassword, password, "Password should match original")
			} else {
				assert.False(t, hasPassword, "Password should not be present")
			}
		})
	}
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
