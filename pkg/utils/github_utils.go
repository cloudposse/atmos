package utils

import (
	"context"
	"os"
	"time"

	log "github.com/charmbracelet/log"
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
	opt := &github.ListOptions{Page: 1, PerPage: 1}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	client := newGitHubClient(ctx)

	releases, _, err := client.Repositories.ListReleases(ctx, owner, repo, opt)
	if err != nil {
		return "", err
	}
	log.Debug("We got the following", "releases", releases, "err", err, "token", os.Getenv("GITHUB_TOKEN"))
	if len(releases) > 0 {
		latestRelease := releases[0]
		latestReleaseTag := *latestRelease.TagName
		return latestReleaseTag, nil
	}

	return "", nil
}
