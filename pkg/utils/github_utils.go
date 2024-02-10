package utils

import (
	"context"
	"time"

	"github.com/google/go-github/v59/github"
)

// GetLatestGitHubRepoRelease returns the latest release tag for a GitHub repository
func GetLatestGitHubRepoRelease(owner string, repo string) (string, error) {
	opt := &github.ListOptions{Page: 1, PerPage: 1}
	client := github.NewClient(nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	releases, _, err := client.Repositories.ListReleases(ctx, owner, repo, opt)
	if err != nil {
		return "", err
	}

	if len(releases) > 0 {
		latestRelease := releases[0]
		latestReleaseTag := *latestRelease.TagName
		return latestReleaseTag, nil
	}

	return "", nil
}
