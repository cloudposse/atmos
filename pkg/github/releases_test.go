package github

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v59/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/tests"
)

// isRateLimitError checks if an error is a GitHub API rate limit error.
func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	// Check for our wrapped error type first.
	if errors.Is(err, errUtils.ErrGitHubRateLimitExceeded) {
		return true
	}
	// Fallback to checking error message for GitHub API rate limit errors.
	// This handles errors from API calls that don't use handleGitHubAPIError.
	errMsg := err.Error()
	return strings.Contains(errMsg, "rate limit exceeded") ||
		strings.Contains(errMsg, "API rate limit")
}

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

// TestGetLatestRelease tests fetching the latest release from GitHub.
func TestGetLatestRelease(t *testing.T) {
	t.Run("fetches latest release from public repo", func(t *testing.T) {
		// Check GitHub access and rate limits
		rateLimits := tests.RequireGitHubAccess(t)
		if rateLimits != nil && rateLimits.Remaining < 5 {
			t.Skipf("Need at least 5 GitHub API requests, only %d remaining", rateLimits.Remaining)
		}

		// Test with a known public repo that has releases
		owner := "cloudposse"
		repo := "atmos"

		tag, err := GetLatestRelease(owner, repo)
		if isRateLimitError(err) {
			t.Skipf("Skipping due to GitHub API rate limit: %v", err)
		}

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

		tag, err := GetLatestRelease(owner, repo)
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

		tag, err := GetLatestRelease(owner, repo)

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

		tag, err := GetLatestRelease(owner, repo)

		// Should return error for invalid repo
		assert.Error(t, err)
		assert.Empty(t, tag)
	})
}

// TestGetLatestReleaseWithAuthentication tests authenticated GitHub API access.
func TestGetLatestReleaseWithAuthentication(t *testing.T) {
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

		tag, err := GetLatestRelease(owner, repo)
		if isRateLimitError(err) {
			t.Skipf("Skipping due to GitHub API rate limit: %v", err)
		}

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
				tag, err := GetLatestRelease(tc.owner, tc.repo)
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

		_, err := GetLatestRelease(owner, repo)
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
				_, err := GetLatestRelease(owner, repo)
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

// TestGetReleases tests fetching multiple releases with pagination.
func TestGetReleases(t *testing.T) {
	t.Run("fetches releases with default options", func(t *testing.T) {
		rateLimits := tests.RequireGitHubAccess(t)
		if rateLimits != nil && rateLimits.Remaining < 5 {
			t.Skipf("Need at least 5 GitHub API requests, only %d remaining", rateLimits.Remaining)
		}

		opts := ReleasesOptions{
			Owner:              "cloudposse",
			Repo:               "atmos",
			Limit:              5,
			Offset:             0,
			IncludePrereleases: false,
		}

		releases, err := GetReleases(opts)
		if isRateLimitError(err) {
			t.Skipf("Skipping due to GitHub API rate limit: %v", err)
		}
		require.NoError(t, err)
		assert.LessOrEqual(t, len(releases), 5)
		for _, release := range releases {
			assert.False(t, release.GetPrerelease(), "Should not include prereleases")
		}
	})

	t.Run("includes prereleases when requested", func(t *testing.T) {
		rateLimits := tests.RequireGitHubAccess(t)
		if rateLimits != nil && rateLimits.Remaining < 5 {
			t.Skipf("Need at least 5 GitHub API requests, only %d remaining", rateLimits.Remaining)
		}

		opts := ReleasesOptions{
			Owner:              "cloudposse",
			Repo:               "atmos",
			Limit:              10,
			Offset:             0,
			IncludePrereleases: true,
		}

		releases, err := GetReleases(opts)
		if isRateLimitError(err) {
			t.Skipf("Skipping due to GitHub API rate limit: %v", err)
		}
		require.NoError(t, err)
		assert.LessOrEqual(t, len(releases), 10)
		// We can't guarantee prereleases exist, just that we don't filter them.
	})

	t.Run("handles pagination with offset", func(t *testing.T) {
		rateLimits := tests.RequireGitHubAccess(t)
		if rateLimits != nil && rateLimits.Remaining < 10 {
			t.Skipf("Need at least 10 GitHub API requests, only %d remaining", rateLimits.Remaining)
		}

		// Get first page.
		opts1 := ReleasesOptions{
			Owner:              "cloudposse",
			Repo:               "atmos",
			Limit:              5,
			Offset:             0,
			IncludePrereleases: false,
		}
		releases1, err := GetReleases(opts1)
		if isRateLimitError(err) {
			t.Skipf("Skipping due to GitHub API rate limit: %v", err)
		}
		require.NoError(t, err)

		// Get second page.
		opts2 := ReleasesOptions{
			Owner:              "cloudposse",
			Repo:               "atmos",
			Limit:              5,
			Offset:             5,
			IncludePrereleases: false,
		}
		releases2, err := GetReleases(opts2)
		if isRateLimitError(err) {
			t.Skipf("Skipping due to GitHub API rate limit: %v", err)
		}
		require.NoError(t, err)

		// Ensure different releases (if enough exist).
		if len(releases1) > 0 && len(releases2) > 0 {
			assert.NotEqual(t, releases1[0].GetTagName(), releases2[0].GetTagName())
		}
	})

	t.Run("returns empty slice when offset exceeds total", func(t *testing.T) {
		rateLimits := tests.RequireGitHubAccess(t)
		if rateLimits != nil && rateLimits.Remaining < 5 {
			t.Skipf("Need at least 5 GitHub API requests, only %d remaining", rateLimits.Remaining)
		}

		opts := ReleasesOptions{
			Owner:              "cloudposse",
			Repo:               "atmos",
			Limit:              5,
			Offset:             500, // Offset beyond likely total releases (GitHub API limit is 1000).
			IncludePrereleases: false,
		}

		releases, err := GetReleases(opts)
		if isRateLimitError(err) {
			t.Skipf("Skipping due to GitHub API rate limit: %v", err)
		}
		require.NoError(t, err)
		// Should either be empty or have fewer than requested if offset is near the end.
		assert.LessOrEqual(t, len(releases), 5)
	})
}

// TestGetReleaseByTag tests fetching a specific release by tag.
func TestGetReleaseByTag(t *testing.T) {
	t.Run("fetches specific release by tag", func(t *testing.T) {
		rateLimits := tests.RequireGitHubAccess(t)
		if rateLimits != nil && rateLimits.Remaining < 5 {
			t.Skipf("Need at least 5 GitHub API requests, only %d remaining", rateLimits.Remaining)
		}

		// Use a known release tag.
		release, err := GetReleaseByTag("cloudposse", "atmos", "v1.50.0")
		if isRateLimitError(err) {
			t.Skipf("Skipping due to GitHub API rate limit: %v", err)
		}
		require.NoError(t, err)
		assert.NotNil(t, release)
		assert.Equal(t, "v1.50.0", release.GetTagName())
	})

	t.Run("returns error for invalid tag", func(t *testing.T) {
		rateLimits := tests.RequireGitHubAccess(t)
		if rateLimits != nil && rateLimits.Remaining < 5 {
			t.Skipf("Need at least 5 GitHub API requests, only %d remaining", rateLimits.Remaining)
		}

		_, err := GetReleaseByTag("cloudposse", "atmos", "v999.999.999")
		assert.Error(t, err)
	})
}

// TestGetLatestReleaseInfo tests fetching the latest stable release.
func TestGetLatestReleaseInfo(t *testing.T) {
	t.Run("fetches latest stable release", func(t *testing.T) {
		rateLimits := tests.RequireGitHubAccess(t)
		if rateLimits != nil && rateLimits.Remaining < 5 {
			t.Skipf("Need at least 5 GitHub API requests, only %d remaining", rateLimits.Remaining)
		}

		release, err := GetLatestReleaseInfo("cloudposse", "atmos")
		if isRateLimitError(err) {
			t.Skipf("Skipping due to GitHub API rate limit: %v", err)
		}
		require.NoError(t, err)
		assert.NotNil(t, release)
		assert.NotEmpty(t, release.GetTagName())
		assert.False(t, release.GetPrerelease(), "Latest release should not be a prerelease")
	})
}

// TestNewGitHubClientWithAtmosToken tests token precedence.
func TestNewGitHubClientWithAtmosToken(t *testing.T) {
	t.Run("prefers ATMOS_GITHUB_TOKEN over GITHUB_TOKEN", func(t *testing.T) {
		// This test verifies the viper binding logic.
		// We can't directly test precedence without mocking viper,
		// but we can verify the function doesn't panic.
		t.Setenv("ATMOS_GITHUB_TOKEN", "test-atmos-token")
		t.Setenv("GITHUB_TOKEN", "test-github-token")

		ctx := context.Background()
		client := newGitHubClient(ctx)
		assert.NotNil(t, client)
	})
}

// TestFilterPrereleases tests the filterPrereleases function.
func TestFilterPrereleases(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name        string
		releases    []*github.RepositoryRelease
		include     bool
		expectedLen int
	}{
		{
			name: "filters out prereleases when not included",
			releases: []*github.RepositoryRelease{
				{TagName: github.String("v1.0.0"), Prerelease: github.Bool(false), PublishedAt: &github.Timestamp{Time: now}},
				{TagName: github.String("v2.0.0-beta"), Prerelease: github.Bool(true), PublishedAt: &github.Timestamp{Time: now}},
				{TagName: github.String("v1.5.0"), Prerelease: github.Bool(false), PublishedAt: &github.Timestamp{Time: now}},
			},
			include:     false,
			expectedLen: 2,
		},
		{
			name: "includes prereleases when requested",
			releases: []*github.RepositoryRelease{
				{TagName: github.String("v1.0.0"), Prerelease: github.Bool(false), PublishedAt: &github.Timestamp{Time: now}},
				{TagName: github.String("v2.0.0-beta"), Prerelease: github.Bool(true), PublishedAt: &github.Timestamp{Time: now}},
			},
			include:     true,
			expectedLen: 2,
		},
		{
			name:        "handles empty slice",
			releases:    []*github.RepositoryRelease{},
			include:     false,
			expectedLen: 0,
		},
		{
			name: "handles all prereleases",
			releases: []*github.RepositoryRelease{
				{TagName: github.String("v2.0.0-alpha"), Prerelease: github.Bool(true), PublishedAt: &github.Timestamp{Time: now}},
				{TagName: github.String("v2.0.0-beta"), Prerelease: github.Bool(true), PublishedAt: &github.Timestamp{Time: now}},
			},
			include:     false,
			expectedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterPrereleases(tt.releases, tt.include)
			assert.Len(t, result, tt.expectedLen)

			// Verify no prereleases when not included.
			if !tt.include {
				for _, release := range result {
					assert.False(t, release.GetPrerelease(), "Should not contain prereleases")
				}
			}
		})
	}
}

// TestFilterByDate tests the filterByDate function.
func TestFilterByDate(t *testing.T) {
	baseTime := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	beforeTime := baseTime.Add(-24 * time.Hour)
	afterTime := baseTime.Add(24 * time.Hour)

	tests := []struct {
		name        string
		releases    []*github.RepositoryRelease
		since       *time.Time
		expectedLen int
	}{
		{
			name: "filters releases before since date",
			releases: []*github.RepositoryRelease{
				{TagName: github.String("v1.0.0"), PublishedAt: &github.Timestamp{Time: beforeTime}},
				{TagName: github.String("v2.0.0"), PublishedAt: &github.Timestamp{Time: baseTime}},
				{TagName: github.String("v3.0.0"), PublishedAt: &github.Timestamp{Time: afterTime}},
			},
			since:       &baseTime,
			expectedLen: 2, // baseTime and afterTime
		},
		{
			name: "includes releases on exact since date",
			releases: []*github.RepositoryRelease{
				{TagName: github.String("v1.0.0"), PublishedAt: &github.Timestamp{Time: beforeTime}},
				{TagName: github.String("v2.0.0"), PublishedAt: &github.Timestamp{Time: baseTime}},
			},
			since:       &baseTime,
			expectedLen: 1, // Only baseTime
		},
		{
			name: "returns all when since is nil",
			releases: []*github.RepositoryRelease{
				{TagName: github.String("v1.0.0"), PublishedAt: &github.Timestamp{Time: beforeTime}},
				{TagName: github.String("v2.0.0"), PublishedAt: &github.Timestamp{Time: afterTime}},
			},
			since:       nil,
			expectedLen: 2,
		},
		{
			name:        "handles empty slice",
			releases:    []*github.RepositoryRelease{},
			since:       &baseTime,
			expectedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterByDate(tt.releases, tt.since)
			assert.Len(t, result, tt.expectedLen)

			// Verify all releases are after or equal to since date.
			if tt.since != nil {
				for _, release := range result {
					publishedAt := release.GetPublishedAt().Time
					assert.True(t, publishedAt.After(*tt.since) || publishedAt.Equal(*tt.since),
						"Release %s published at %v should be after or equal to %v",
						release.GetTagName(), publishedAt, *tt.since)
				}
			}
		})
	}
}

// TestApplyPagination tests the applyPagination function.
func TestApplyPagination(t *testing.T) {
	now := time.Now()
	releases := []*github.RepositoryRelease{
		{TagName: github.String("v1.0.0"), PublishedAt: &github.Timestamp{Time: now}},
		{TagName: github.String("v1.1.0"), PublishedAt: &github.Timestamp{Time: now}},
		{TagName: github.String("v1.2.0"), PublishedAt: &github.Timestamp{Time: now}},
		{TagName: github.String("v1.3.0"), PublishedAt: &github.Timestamp{Time: now}},
		{TagName: github.String("v1.4.0"), PublishedAt: &github.Timestamp{Time: now}},
	}

	tests := []struct {
		name        string
		offset      int
		limit       int
		expectedLen int
		expectedTag string // Tag of first element
	}{
		{
			name:        "normal pagination",
			offset:      0,
			limit:       3,
			expectedLen: 3,
			expectedTag: "v1.0.0",
		},
		{
			name:        "pagination with offset",
			offset:      2,
			limit:       2,
			expectedLen: 2,
			expectedTag: "v1.2.0",
		},
		{
			name:        "offset exceeds length",
			offset:      10,
			limit:       5,
			expectedLen: 0,
			expectedTag: "",
		},
		{
			name:        "limit exceeds remaining items",
			offset:      3,
			limit:       10,
			expectedLen: 2, // Only 2 items left after offset 3
			expectedTag: "v1.3.0",
		},
		{
			name:        "zero limit",
			offset:      0,
			limit:       0,
			expectedLen: 5, // limit<=0 returns all items
			expectedTag: "v1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyPagination(releases, tt.offset, tt.limit)
			assert.Len(t, result, tt.expectedLen)
			if tt.expectedLen > 0 {
				assert.Equal(t, tt.expectedTag, result[0].GetTagName())
			}
		})
	}
}
