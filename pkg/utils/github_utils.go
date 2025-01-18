package utils

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cloudposse/atmos/pkg/schema"
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
func GetLatestGitHubRepoRelease(owner string, repo string, config schema.AtmosConfiguration) (string, error) {
	opt := &github.ListOptions{Page: 1, PerPage: 1}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	client := newGitHubClient(ctx)

	LogDebug(config, fmt.Sprintf("Fetching latest release for %s/%s from GitHub API", owner, repo))
	releases, _, err := client.Repositories.ListReleases(ctx, owner, repo, opt)
	if err != nil {
		LogDebug(config, fmt.Sprintf("Error fetching GitHub releases: %v", err))
		return "", err
	}

	if len(releases) > 0 {
		latestRelease := releases[0]
		latestReleaseTag := *latestRelease.TagName
		LogDebug(config, fmt.Sprintf("Latest release tag: %s", latestReleaseTag))
		return latestReleaseTag, nil
	}

	LogDebug(config, "No releases found")
	return "", nil
}
