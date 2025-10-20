package utils

import (
	"github.com/google/go-github/v59/github"

	gh "github.com/cloudposse/atmos/pkg/github"
)

// GetLatestGitHubRepoRelease returns the latest release tag for a GitHub repository.
// Deprecated: Use github.GetLatestRelease instead.
func GetLatestGitHubRepoRelease(owner string, repo string) (string, error) {
	return gh.GetLatestRelease(owner, repo)
}

// GitHubReleasesOptions contains options for fetching GitHub releases.
// Deprecated: Use github.ReleasesOptions instead.
type GitHubReleasesOptions = gh.ReleasesOptions

// GetGitHubRepoReleases fetches GitHub releases with pagination, prerelease filtering, and date filtering.
// Deprecated: Use github.GetReleases instead.
func GetGitHubRepoReleases(opts GitHubReleasesOptions) ([]*github.RepositoryRelease, error) {
	return gh.GetReleases(opts)
}

// GetGitHubReleaseByTag fetches a specific GitHub release by tag name.
// Deprecated: Use github.GetReleaseByTag instead.
func GetGitHubReleaseByTag(owner, repo, tag string) (*github.RepositoryRelease, error) {
	return gh.GetReleaseByTag(owner, repo, tag)
}

// GetGitHubLatestRelease fetches the latest stable release from GitHub.
// Deprecated: Use github.GetLatestReleaseInfo instead.
func GetGitHubLatestRelease(owner, repo string) (*github.RepositoryRelease, error) {
	return gh.GetLatestReleaseInfo(owner, repo)
}
