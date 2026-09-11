package github

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/tests/testhelpers/httpmock"
)

// setupMockGitHubClient points RepoEndpoints (GITHUB_SERVER_URL/GITHUB_API_URL) at a fresh
// httpmock GitHub facade and disables every source of ambient GitHub authentication
// (ATMOS_PRO_GITHUB_TOKEN/ATMOS_GITHUB_TOKEN/GITHUB_TOKEN and the `gh auth token` CLI
// fallback), so the returned mock's responses are the only thing that can affect the test --
// regardless of the developer machine's or CI runner's own GitHub auth state.
func setupMockGitHubClient(t *testing.T) *httpmock.GitHubMockServer {
	t.Helper()

	mock := httpmock.NewGitHubMockServer(t)
	mock.Setenv(t)
	t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")
	t.Setenv("ATMOS_GITHUB_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("ATMOS_GITHUB_CLI", "") // Disable the `gh auth token` CLI fallback.
	return mock
}

// TestGetLatestRelease_ViaMock exercises GetLatestRelease (pkg/github/releases.go) against the
// httpmock facade's /api/v3/repos/{owner}/{repo}/releases/latest route for the 401/404/429
// cases handleGitHubAPIError (pkg/github/client.go) classifies.
func TestGetLatestRelease_ViaMock(t *testing.T) {
	t.Run("401 returns ErrAuthenticationFailed", func(t *testing.T) {
		mock := setupMockGitHubClient(t)
		mock.FailWith("/api/v3/repos/owner/repo/releases", http.StatusUnauthorized)

		_, err := GetLatestRelease("owner", "repo")

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	})

	// A 404 (repo/release not found) is not classified by handleGitHubAPIError into any atmos
	// sentinel -- it is returned as-is (a *github.ErrorResponse from go-github). Assert the
	// current behavior: a non-nil error that is deliberately NOT one of the two sentinels a 401
	// or a rate-limited 403/429 would produce.
	t.Run("404 returns the raw go-github error, not an atmos sentinel", func(t *testing.T) {
		mock := setupMockGitHubClient(t)
		mock.FailWith("/api/v3/repos/owner/repo/releases", http.StatusNotFound)

		_, err := GetLatestRelease("owner", "repo")

		require.Error(t, err)
		assert.NotErrorIs(t, err, errUtils.ErrAuthenticationFailed)
		assert.NotErrorIs(t, err, errUtils.ErrGitHubRateLimitExceeded)
	})

	t.Run("429 returns ErrGitHubRateLimitExceeded", func(t *testing.T) {
		mock := setupMockGitHubClient(t)
		// handleGitHubAPIError (pkg/github/client.go) requires BOTH a 403/429 status AND
		// X-RateLimit-Remaining: 0 -- GitHub always sends the header alongside a genuine 429,
		// so the mock must be told the budget is exhausted, not just return the status code.
		mock.SetRateLimit(0, time.Now().Add(time.Hour))
		mock.FailWith("/api/v3/repos/owner/repo/releases", http.StatusTooManyRequests)

		_, err := GetLatestRelease("owner", "repo")

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrGitHubRateLimitExceeded)
	})
}

// TestGetReleases_ViaMock exercises GetReleases (pkg/github/releases.go) the same way as
// TestGetLatestRelease_ViaMock, against the list endpoint instead of /releases/latest.
func TestGetReleases_ViaMock(t *testing.T) {
	t.Run("401 returns ErrAuthenticationFailed", func(t *testing.T) {
		mock := setupMockGitHubClient(t)
		mock.FailWith("/api/v3/repos/owner/repo/releases", http.StatusUnauthorized)

		_, err := GetReleases(ReleasesOptions{Owner: "owner", Repo: "repo"})

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	})

	t.Run("404 returns the raw go-github error, not an atmos sentinel", func(t *testing.T) {
		mock := setupMockGitHubClient(t)
		mock.FailWith("/api/v3/repos/owner/repo/releases", http.StatusNotFound)

		_, err := GetReleases(ReleasesOptions{Owner: "owner", Repo: "repo"})

		require.Error(t, err)
		assert.NotErrorIs(t, err, errUtils.ErrAuthenticationFailed)
		assert.NotErrorIs(t, err, errUtils.ErrGitHubRateLimitExceeded)
	})

	// GetReleases calls checkRateLimitBeforeFetch (pkg/github/releases.go) before listing, which
	// itself surfaces ErrGitHubRateLimitExceeded once remaining drops below the 5-request
	// threshold -- the mock's SetRateLimit(0, ...) triggers that pre-check, so the list request
	// (and its own FailWith 429) is never reached. Both paths produce the same sentinel.
	t.Run("429 returns ErrGitHubRateLimitExceeded", func(t *testing.T) {
		mock := setupMockGitHubClient(t)
		mock.SetRateLimit(0, time.Now().Add(time.Hour))
		mock.FailWith("/api/v3/repos/owner/repo/releases", http.StatusTooManyRequests)

		_, err := GetReleases(ReleasesOptions{Owner: "owner", Repo: "repo"})

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrGitHubRateLimitExceeded)
	})

	t.Run("succeeds against the mock's releases list", func(t *testing.T) {
		mock := setupMockGitHubClient(t)
		mock.RegisterRelease("owner", "repo", httpmock.ReleaseSpec{TagName: "v2.0.0"})
		mock.RegisterRelease("owner", "repo", httpmock.ReleaseSpec{TagName: "v1.0.0"})

		releases, err := GetReleases(ReleasesOptions{Owner: "owner", Repo: "repo"})

		require.NoError(t, err)
		require.Len(t, releases, 2)
		assert.Equal(t, "v2.0.0", releases[0].GetTagName())
		assert.Equal(t, "v1.0.0", releases[1].GetTagName())
	})
}
