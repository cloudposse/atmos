package github

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-github/v59/github"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// GithubAPIMaxPerPage is the maximum number of items per page allowed by GitHub API.
	githubAPIMaxPerPage = 100
	// GithubAPIStatus422 is the HTTP status code returned when pagination exceeds limits.
	githubAPIStatus422 = 422
	// GithubAPIMinRateLimitThreshold is the minimum number of remaining requests before warning.
	githubAPIMinRateLimitThreshold = 5
	// Logging field name constants.
	logFieldOwner = "owner"
	logFieldRepo  = "repo"
)

// newGitHubClient creates a new GitHub client. If a token is provided, it returns an authenticated client;
// otherwise, it returns an unauthenticated client.
func newGitHubClient(ctx context.Context) *github.Client {
	defer perf.Track(nil, "github.newGitHubClient")()

	// Check for ATMOS_GITHUB_TOKEN first, then fall back to GITHUB_TOKEN.
	githubToken := viper.GetString("ATMOS_GITHUB_TOKEN")
	if githubToken == "" {
		githubToken = viper.GetString("GITHUB_TOKEN")
	}

	if githubToken == "" {
		return github.NewClient(nil)
	}

	// Token found, create an authenticated client.
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubToken},
	)
	tc := oauth2.NewClient(ctx, ts)

	return github.NewClient(tc)
}

// handleGitHubAPIError converts GitHub API errors to more descriptive error messages,
// especially for rate limiting.
func handleGitHubAPIError(err error, resp *github.Response) error {
	defer perf.Track(nil, "github.handleGitHubAPIError")()

	if resp != nil && resp.Rate.Remaining == 0 {
		resetTime := resp.Rate.Reset.Time
		waitDuration := time.Until(resetTime)

		return fmt.Errorf("%w: rate limit exceeded, resets at %s (in %s). Consider setting ATMOS_GITHUB_TOKEN or GITHUB_TOKEN for higher limits",
			errUtils.ErrGitHubRateLimitExceeded,
			resetTime.Format(time.RFC3339),
			waitDuration.Round(time.Second),
		)
	}

	return err
}
