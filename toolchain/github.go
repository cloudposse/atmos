package toolchain

import (
	"github.com/cloudposse/atmos/pkg/github"
	"github.com/cloudposse/atmos/pkg/perf"
)

// GitHubAPI defines the interface for GitHub API operations.
type GitHubAPI interface {
	FetchReleases(owner, repo string, limit int) ([]string, error)
}

// GitHubAPIClient implements GitHubAPI using pkg/github.
type GitHubAPIClient struct{}

// NewGitHubAPIClient creates a new GitHub API client.
func NewGitHubAPIClient() *GitHubAPIClient {
	defer perf.Track(nil, "toolchain.NewGitHubAPIClient")()

	return &GitHubAPIClient{}
}

// FetchReleases fetches all available versions from GitHub API.
func (g *GitHubAPIClient) FetchReleases(owner, repo string, limit int) ([]string, error) {
	defer perf.Track(nil, "toolchain.GitHubAPIClient.FetchReleases")()

	return github.GetReleaseVersions(owner, repo, limit)
}

// Global GitHub API client instance.
var defaultGitHubAPI GitHubAPI = NewGitHubAPIClient()

// SetGitHubAPI sets the global GitHub API client (for testing).
func SetGitHubAPI(api GitHubAPI) {
	defer perf.Track(nil, "toolchain.SetGitHubAPI")()

	defaultGitHubAPI = api
}

// ResetGitHubAPI resets the global GitHub API client to the default.
func ResetGitHubAPI() {
	defer perf.Track(nil, "toolchain.ResetGitHubAPI")()

	defaultGitHubAPI = NewGitHubAPIClient()
}

// fetchAllGitHubVersions is the public function that uses the global GitHub API client.
func fetchAllGitHubVersions(owner, repo string, limit int) ([]string, error) {
	return defaultGitHubAPI.FetchReleases(owner, repo, limit)
}
