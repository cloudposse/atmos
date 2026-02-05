package toolchain

import (
	"sync"

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

// Global GitHub API client instance with mutex protection for concurrent access.
var (
	defaultGitHubAPI GitHubAPI = NewGitHubAPIClient()
	githubAPIMu      sync.RWMutex
)

// SetGitHubAPI sets the global GitHub API client (for testing).
func SetGitHubAPI(api GitHubAPI) {
	defer perf.Track(nil, "toolchain.SetGitHubAPI")()

	githubAPIMu.Lock()
	defer githubAPIMu.Unlock()
	defaultGitHubAPI = api
}

// ResetGitHubAPI resets the global GitHub API client to the default.
func ResetGitHubAPI() {
	defer perf.Track(nil, "toolchain.ResetGitHubAPI")()

	githubAPIMu.Lock()
	defer githubAPIMu.Unlock()
	defaultGitHubAPI = NewGitHubAPIClient()
}

// fetchAllGitHubVersions is the public function that uses the global GitHub API client.
func fetchAllGitHubVersions(owner, repo string, limit int) ([]string, error) {
	githubAPIMu.RLock()
	api := defaultGitHubAPI
	githubAPIMu.RUnlock()
	return api.FetchReleases(owner, repo, limit)
}
