package downloader

import (
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
// credentials are already in the URL (from template processing), automatic
// injection should preserve them or handle them correctly.
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

	// With the fixed logic, automatic injection should use the configured token
	// (the one from atmosConfig) rather than what was in the URL
	assert.Contains(t, finalURL, "x-access-token:", "Should use x-access-token as username")
	assert.Contains(t, finalURL, "ghp_automatic_token", "Should inject configured token")

	t.Logf("URL with pre-existing creds was transformed correctly")
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
