package utils

import (
	"context"
	"fmt"
	"os"

	"github.com/google/go-github/v59/github"
	"golang.org/x/oauth2"
)

// newGitHubClient creates a new GitHub client. If a token is provided, it returns an authenticated client;
// otherwise, it returns an unauthenticated client.
func newGitHubClient(ctx context.Context) *github.Client {
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		return github.NewClient(nil)
	}

	// Token found, create an authenticated client
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubToken},
	)
	tc := oauth2.NewClient(ctx, ts)

	return github.NewClient(tc)
}

// GetLatestGitHubRepoRelease returns the latest release tag for a GitHub repository
func GetLatestGitHubRepoRelease(owner string, repo string) (string, error) {
	LogDebug(fmt.Sprintf("Fetching latest release for %s/%s from Github API", owner, repo))

	// Create a new GitHub client with authentication if available
	ctx := context.Background()
	client := newGitHubClient(ctx)

	// Get the latest release
	release, _, err := client.Repositories.GetLatestRelease(ctx, owner, repo)
	if err != nil {
		return "", err
	}

	if release == nil || release.TagName == nil {
		return "", nil
	}

	return *release.TagName, nil
}
