package downloader

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestCustomGitDetector_EndToEnd_GitHubTokenFallback tests the complete flow
// from URL detection through token injection to final URL generation.
// This reproduces the exact scenario reported by the user.
func TestCustomGitDetector_EndToEnd_GitHubTokenFallback(t *testing.T) {
	tests := []struct {
		name                string
		githubToken         string
		atmosGithubToken    string
		sourceURL           string
		expectTokenInjected bool
		expectedUsername    string
	}{
		{
			name:                "GITHUB_TOKEN set, ATMOS_GITHUB_TOKEN empty - should inject token",
			githubToken:         "ghp_user_github_token",
			atmosGithubToken:    "",
			sourceURL:           "github.com/test-org/test-repo.git?ref=main",
			expectTokenInjected: true,
			expectedUsername:    "x-access-token",
		},
		{
			name:                "ATMOS_GITHUB_TOKEN set - should use ATMOS token",
			githubToken:         "ghp_user_github_token",
			atmosGithubToken:    "ghp_atmos_token",
			sourceURL:           "github.com/test-org/test-repo.git?ref=main",
			expectTokenInjected: true,
			expectedUsername:    "x-access-token",
		},
		{
			name:                "Both tokens empty - no injection",
			githubToken:         "",
			atmosGithubToken:    "",
			sourceURL:           "github.com/test-org/test-repo.git?ref=main",
			expectTokenInjected: false,
			expectedUsername:    "",
		},
		{
			name:                "GITHUB_TOKEN with git:: prefix",
			githubToken:         "ghp_with_prefix",
			atmosGithubToken:    "",
			sourceURL:           "git::https://github.com/test-org/test-repo.git?ref=main",
			expectTokenInjected: true,
			expectedUsername:    "x-access-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup config with tokens
			atmosConfig := &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					GithubToken:       tt.githubToken,
					AtmosGithubToken:  tt.atmosGithubToken,
					InjectGithubToken: true, // Default value
				},
			}

			// Create detector and run Detect
			detector := NewCustomGitDetector(atmosConfig, tt.sourceURL)
			finalURL, detected, err := detector.Detect(tt.sourceURL, "")

			// Verify no errors
			require.NoError(t, err, "Detect should not error")

			if !tt.expectTokenInjected {
				// If no token expected, detection might still succeed but without credentials
				if detected {
					assert.NotContains(t, finalURL, "@", "URL should not contain credentials")
				}
				return
			}

			// Verify detection succeeded
			require.True(t, detected, "Should detect GitHub URL")
			require.NotEmpty(t, finalURL, "Final URL should not be empty")

			// Verify URL starts with git::
			assert.True(t, strings.HasPrefix(finalURL, "git::"), "Final URL should start with git::")

			// Verify URL contains credentials
			assert.Contains(t, finalURL, "@", "Final URL should contain @ indicating credentials")

			// Verify username
			assert.Contains(t, finalURL, tt.expectedUsername+":", "Final URL should contain correct username")

			// Verify token is in URL (one of the two tokens)
			hasGithubToken := strings.Contains(finalURL, tt.githubToken)
			hasAtmosToken := strings.Contains(finalURL, tt.atmosGithubToken)
			assert.True(t, hasGithubToken || hasAtmosToken,
				"Final URL should contain either GITHUB_TOKEN or ATMOS_GITHUB_TOKEN")

			// Verify depth=1 was added
			assert.Contains(t, finalURL, "depth=1", "Final URL should include depth=1 for shallow clone")

			// Log the final URL for debugging (masked)
			maskedURL, _ := maskBasicAuth(strings.TrimPrefix(finalURL, "git::"))
			t.Logf("Final URL (masked): git::%s", maskedURL)
		})
	}
}

// TestCustomGitDetector_EndToEnd_PreExistingCredentials tests that when
// credentials are already in the URL (from template processing), user-provided
// credentials take precedence and automatic injection is skipped.
func TestCustomGitDetector_EndToEnd_PreExistingCredentials(t *testing.T) {
	// This test verifies behavior when template processing has already added credentials
	sourceWithCreds := "https://manual_token@github.com/test-org/test-repo.git?ref=main"

	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			GithubToken:       "ghp_automatic_token",
			AtmosGithubToken:  "",
			InjectGithubToken: true,
		},
	}

	detector := NewCustomGitDetector(atmosConfig, sourceWithCreds)
	finalURL, detected, err := detector.Detect(sourceWithCreds, "")

	require.NoError(t, err, "Detect should not error")
	require.True(t, detected, "Should detect GitHub URL")

	// User-provided credentials in the URL take precedence over automatic injection.
	// The manual_token should be preserved, not replaced with the configured token.
	assert.Contains(t, finalURL, "manual_token", "Should preserve user-provided credentials")
	assert.NotContains(t, finalURL, "ghp_automatic_token", "Should not inject automatic token when credentials exist")

	t.Logf("URL with pre-existing creds was preserved correctly")
}

// TestCustomGitDetector_EndToEnd_NonGitHubHost verifies that token injection
// only happens for supported hosts (GitHub, GitLab, Bitbucket).
func TestCustomGitDetector_EndToEnd_NonGitHubHost(t *testing.T) {
	sourceURL := "custom-git-host.com/org/repo.git?ref=main"

	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			GithubToken:       "ghp_token",
			InjectGithubToken: true,
		},
	}

	detector := NewCustomGitDetector(atmosConfig, sourceURL)
	finalURL, detected, err := detector.Detect(sourceURL, "")

	require.NoError(t, err, "Detect should not error")

	// For non-GitHub/GitLab/Bitbucket hosts, detection returns false
	assert.False(t, detected, "Should not detect non-supported host")
	assert.Empty(t, finalURL, "Should not return a URL for unsupported host")
}

// TestCustomGitDetector_EndToEnd_UserSpecifiedCredentials tests that user-specified credentials in the URL are preserved.
func TestCustomGitDetector_EndToEnd_UserSpecifiedCredentials(t *testing.T) {
	tests := []struct {
		name             string
		sourceURL        string
		githubToken      string
		expectedUsername string
		expectedPassword string
		shouldHaveUser   bool
	}{
		{
			name:             "User-specified credentials preserved, automatic injection skipped",
			sourceURL:        "https://myuser:mypass@github.com/test-org/test-repo.git?ref=main",
			githubToken:      "ghp_should_not_be_used",
			expectedUsername: "myuser",
			expectedPassword: "mypass",
			shouldHaveUser:   true,
		},
		{
			name:             "User-specified token format preserved",
			sourceURL:        "https://x-access-token:ghp_usertoken@github.com/test-org/test-repo.git?ref=main",
			githubToken:      "ghp_should_not_be_used",
			expectedUsername: "x-access-token",
			expectedPassword: "ghp_usertoken",
			shouldHaveUser:   true,
		},
		{
			name:             "User-specified username only preserved",
			sourceURL:        "https://myuser@github.com/test-org/test-repo.git?ref=main",
			githubToken:      "ghp_should_not_be_used",
			expectedUsername: "myuser",
			expectedPassword: "",
			shouldHaveUser:   true,
		},
		{
			name:             "No user credentials and no token - no injection",
			sourceURL:        "https://github.com/test-org/test-repo.git?ref=main",
			githubToken:      "",
			expectedUsername: "",
			expectedPassword: "",
			shouldHaveUser:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					InjectGithubToken: true,
					GithubToken:       tt.githubToken,
				},
			}

			detector := NewCustomGitDetector(atmosConfig, tt.sourceURL)
			result, detected, err := detector.Detect(tt.sourceURL, "")

			assert.NoError(t, err)
			assert.True(t, detected)
			assert.NotEmpty(t, result)

			// Parse the resulting URL to verify credentials
			resultURL := strings.TrimPrefix(result, "git::")
			parsedResult, err := url.Parse(resultURL)
			assert.NoError(t, err)

			if tt.shouldHaveUser {
				assert.NotNil(t, parsedResult.User, "URL should have user credentials")
				username := parsedResult.User.Username()
				password, hasPassword := parsedResult.User.Password()

				assert.Equal(t, tt.expectedUsername, username, "Username should match expected")
				if tt.expectedPassword != "" {
					assert.True(t, hasPassword, "Password should be present")
					assert.Equal(t, tt.expectedPassword, password, "Password should match expected")
				}
			} else {
				assert.Nil(t, parsedResult.User, "URL should not have user credentials when none specified and no token available")
			}

			// Verify URL structure is preserved
			assert.Equal(t, "github.com", parsedResult.Host)
			assert.Contains(t, parsedResult.Path, "/test-org/test-repo.git")
		})
	}
}
