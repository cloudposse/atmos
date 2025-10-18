package version

import (
	"time"

	"github.com/google/go-github/v59/github"

	"github.com/cloudposse/atmos/pkg/utils"
)

// GitHubClient interface for fetching releases (enables mocking).
type GitHubClient interface {
	GetReleases(owner, repo string, opts ReleaseOptions) ([]*github.RepositoryRelease, error)
	GetRelease(owner, repo, tag string) (*github.RepositoryRelease, error)
	GetLatestRelease(owner, repo string) (*github.RepositoryRelease, error)
}

// ReleaseOptions contains options for fetching releases.
type ReleaseOptions struct {
	Limit              int
	Offset             int
	IncludePrereleases bool
	Since              *time.Time
}

// RealGitHubClient implements GitHubClient using the real GitHub API.
type RealGitHubClient struct{}

// GetReleases fetches releases from GitHub.
func (c *RealGitHubClient) GetReleases(owner, repo string, opts ReleaseOptions) ([]*github.RepositoryRelease, error) {
	return utils.GetGitHubRepoReleases(utils.GitHubReleasesOptions{
		Owner:              owner,
		Repo:               repo,
		Limit:              opts.Limit,
		Offset:             opts.Offset,
		IncludePrereleases: opts.IncludePrereleases,
		Since:              opts.Since,
	})
}

// GetRelease fetches a specific release by tag.
func (c *RealGitHubClient) GetRelease(owner, repo, tag string) (*github.RepositoryRelease, error) {
	return utils.GetGitHubReleaseByTag(owner, repo, tag)
}

// GetLatestRelease fetches the latest stable release.
func (c *RealGitHubClient) GetLatestRelease(owner, repo string) (*github.RepositoryRelease, error) {
	return utils.GetGitHubLatestRelease(owner, repo)
}

// MockGitHubClient for testing (no API calls).
type MockGitHubClient struct {
	Releases []*github.RepositoryRelease
	Release  *github.RepositoryRelease
	Err      error
}

// GetReleases returns mock releases.
func (m *MockGitHubClient) GetReleases(owner, repo string, opts ReleaseOptions) ([]*github.RepositoryRelease, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Releases, nil
}

// GetRelease returns a mock release.
func (m *MockGitHubClient) GetRelease(owner, repo, tag string) (*github.RepositoryRelease, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Release, nil
}

// GetLatestRelease returns a mock latest release.
func (m *MockGitHubClient) GetLatestRelease(owner, repo string) (*github.RepositoryRelease, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Release, nil
}
