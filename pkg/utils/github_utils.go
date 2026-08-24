package utils

import (
	gh "github.com/cloudposse/atmos/pkg/github"
)

// GetLatestGitHubRepoRelease returns the latest release tag for a GitHub repository.
// Deprecated: Use github.GetLatestRelease instead.
func GetLatestGitHubRepoRelease(owner string, repo string) (string, error) {
	return gh.GetLatestRelease(owner, repo)
}
