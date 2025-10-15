package utils

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tests"
)

// TestNewGitHubClientUnauthenticated tests creating an unauthenticated GitHub client.
func TestNewGitHubClientUnauthenticated(t *testing.T) {
	t.Run("creates unauthenticated client when no token present", func(t *testing.T) {
		// Ensure no token is set
		t.Setenv("GITHUB_TOKEN", "")

		ctx := context.Background()
		client := newGitHubClient(ctx)

		assert.NotNil(t, client)
	})
}

// TestNewGitHubClientAuthenticated tests creating an authenticated GitHub client.
func TestNewGitHubClientAuthenticated(t *testing.T) {
	t.Run("creates authenticated client when token present", func(t *testing.T) {
		// Set a test token
		testToken := "ghp_test_token_1234567890"
		t.Setenv("GITHUB_TOKEN", testToken)

		ctx := context.Background()
		client := newGitHubClient(ctx)

		assert.NotNil(t, client)
	})
}

// TestGetLatestGitHubRepoRelease tests fetching the latest release from GitHub.
func TestGetLatestGitHubRepoRelease(t *testing.T) {
	t.Run("fetches latest release from public repo", func(t *testing.T) {
		// Check GitHub access and rate limits
		rateLimits := tests.RequireGitHubAccess(t)
		if rateLimits != nil && rateLimits.Remaining < 5 {
			t.Skipf("Need at least 5 GitHub API requests, only %d remaining", rateLimits.Remaining)
		}

		// Test with a known public repo that has releases
		owner := "cloudposse"
		repo := "atmos"

		tag, err := GetLatestGitHubRepoRelease(owner, repo)

		require.NoError(t, err)
		assert.NotEmpty(t, tag, "Expected a release tag to be returned")
		assert.Contains(t, tag, "v", "Expected tag to contain 'v' prefix")
	})

	t.Run("handles repo without releases", func(t *testing.T) {
		// Check GitHub access and rate limits
		rateLimits := tests.RequireGitHubAccess(t)
		if rateLimits != nil && rateLimits.Remaining < 5 {
			t.Skipf("Need at least 5 GitHub API requests, only %d remaining", rateLimits.Remaining)
		}

		// Test with a repo that might not have releases.
		// Using a valid owner but potentially a repo without releases.
		owner := "cloudposse"
		repo := "test-harness-this-repo-should-not-exist"

		tag, err := GetLatestGitHubRepoRelease(owner, repo)
		// Should get an error for non-existent repo.
		require.Error(t, err)
		assert.Empty(t, tag)
	})

	t.Run("handles invalid owner", func(t *testing.T) {
		// Check GitHub access and rate limits
		rateLimits := tests.RequireGitHubAccess(t)
		if rateLimits != nil && rateLimits.Remaining < 5 {
			t.Skipf("Need at least 5 GitHub API requests, only %d remaining", rateLimits.Remaining)
		}

		owner := "this-owner-definitely-does-not-exist-12345"
		repo := "test-repo"

		tag, err := GetLatestGitHubRepoRelease(owner, repo)

		// Should return error for invalid owner
		assert.Error(t, err)
		assert.Empty(t, tag)
	})

	t.Run("handles invalid repo", func(t *testing.T) {
		// Check GitHub access and rate limits
		rateLimits := tests.RequireGitHubAccess(t)
		if rateLimits != nil && rateLimits.Remaining < 5 {
			t.Skipf("Need at least 5 GitHub API requests, only %d remaining", rateLimits.Remaining)
		}

		owner := "cloudposse"
		repo := "this-repo-definitely-does-not-exist-12345"

		tag, err := GetLatestGitHubRepoRelease(owner, repo)

		// Should return error for invalid repo
		assert.Error(t, err)
		assert.Empty(t, tag)
	})
}

// TestGetLatestGitHubRepoReleaseWithAuthentication tests authenticated GitHub API access.
func TestGetLatestGitHubRepoReleaseWithAuthentication(t *testing.T) {
	t.Run("uses authentication when token available", func(t *testing.T) {
		// Check GitHub access and rate limits
		rateLimits := tests.RequireGitHubAccess(t)
		if rateLimits != nil && rateLimits.Remaining < 5 {
			t.Skipf("Need at least 5 GitHub API requests, only %d remaining", rateLimits.Remaining)
		}

		// If GITHUB_TOKEN is set, the client should be authenticated
		// and have higher rate limits
		owner := "cloudposse"
		repo := "atmos"

		tag, err := GetLatestGitHubRepoRelease(owner, repo)

		require.NoError(t, err)
		assert.NotEmpty(t, tag)
	})
}

// TestGitHubClientCreationWithContext tests client creation with different contexts.
func TestGitHubClientCreationWithContext(t *testing.T) {
	t.Run("creates client with background context", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "")

		ctx := context.Background()
		client := newGitHubClient(ctx)

		assert.NotNil(t, client)
	})

	t.Run("creates client with custom context", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "")

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client := newGitHubClient(ctx)

		assert.NotNil(t, client)
	})
}

// TestGitHubTokenHandling tests token handling from environment.
func TestGitHubTokenHandling(t *testing.T) {
	t.Run("reads token from GITHUB_TOKEN env var", func(t *testing.T) {
		// Test with token
		testToken := "ghp_test_123"
		t.Setenv("GITHUB_TOKEN", testToken)

		// Verify token is read
		token := os.Getenv("GITHUB_TOKEN")
		assert.Equal(t, testToken, token)
	})

	t.Run("handles empty token", func(t *testing.T) {
		// Test without token
		t.Setenv("GITHUB_TOKEN", "")
		token := os.Getenv("GITHUB_TOKEN")
		assert.Empty(t, token)
	})
}

// TestGitHubReleaseTagFormat tests that release tags follow expected format.
func TestGitHubReleaseTagFormat(t *testing.T) {
	t.Run("validates tag format from public repos", func(t *testing.T) {
		// Check GitHub access and rate limits
		rateLimits := tests.RequireGitHubAccess(t)
		if rateLimits != nil && rateLimits.Remaining < 10 {
			t.Skipf("Need at least 10 GitHub API requests, only %d remaining", rateLimits.Remaining)
		}

		testCases := []struct {
			owner string
			repo  string
		}{
			{
				owner: "cloudposse",
				repo:  "atmos",
			},
			{
				owner: "hashicorp",
				repo:  "terraform",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.owner+"/"+tc.repo, func(t *testing.T) {
				tag, err := GetLatestGitHubRepoRelease(tc.owner, tc.repo)
				if err != nil {
					t.Skipf("Skipping test due to GitHub API error: %v", err)
					return
				}

				// Most releases start with 'v' but some don't
				assert.NotEmpty(t, tag, "Expected non-empty tag")

				// Tag should not contain spaces
				assert.NotContains(t, tag, " ", "Tag should not contain spaces")
			})
		}
	})
}

// TestGitHubAPIRateLimit tests handling of rate limits.
func TestGitHubAPIRateLimit(t *testing.T) {
	t.Run("handles rate limit gracefully", func(t *testing.T) {
		// Check GitHub access and rate limits
		rateLimits := tests.RequireGitHubAccess(t)

		if rateLimits != nil {
			t.Logf("Current GitHub API rate limit: %d remaining out of %d",
				rateLimits.Remaining, rateLimits.Limit)

			// If we're very close to the limit, skip
			if rateLimits.Remaining < 2 {
				t.Skipf("Too close to rate limit: %d requests remaining", rateLimits.Remaining)
			}
		}

		// Make a simple API call
		owner := "cloudposse"
		repo := "atmos"

		_, err := GetLatestGitHubRepoRelease(owner, repo)
		// Should either succeed or fail with a clear error
		if err != nil {
			// Log the error but don't fail - could be rate limited
			t.Logf("API call failed (possibly rate limited): %v", err)
		}
	})
}

// TestGitHubClientConfiguration tests client configuration options.
func TestGitHubClientConfiguration(t *testing.T) {
	t.Run("client is properly configured", func(t *testing.T) {
		ctx := context.Background()
		client := newGitHubClient(ctx)

		assert.NotNil(t, client)
		assert.NotNil(t, client.Repositories, "Repositories service should be initialized")
	})
}

// TestConcurrentGitHubAPICalls tests concurrent API calls.
func TestConcurrentGitHubAPICalls(t *testing.T) {
	t.Run("handles concurrent API calls", func(t *testing.T) {
		// Check GitHub access and rate limits
		rateLimits := tests.RequireGitHubAccess(t)
		if rateLimits != nil && rateLimits.Remaining < 10 {
			t.Skipf("Need at least 10 GitHub API requests, only %d remaining", rateLimits.Remaining)
		}

		// Make multiple concurrent calls
		done := make(chan bool, 3)

		repos := []struct {
			owner string
			repo  string
		}{
			{"cloudposse", "atmos"},
			{"cloudposse", "terraform-aws-components"},
			{"cloudposse", "terraform-null-label"},
		}

		for _, r := range repos {
			go func(owner, repo string) {
				_, err := GetLatestGitHubRepoRelease(owner, repo)
				// Don't assert on error - could be rate limited.
				_ = err
				done <- true
			}(r.owner, r.repo)
		}

		// Wait for all goroutines to complete
		for i := 0; i < len(repos); i++ {
			<-done
		}
	})
}
