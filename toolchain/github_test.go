package toolchain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockGitHubAPI implements GitHubAPI for testing.
type MockGitHubAPI struct {
	releases map[string][]string
	errors   map[string]error
}

// NewMockGitHubAPI creates a new mock GitHub API.
func NewMockGitHubAPI() *MockGitHubAPI {
	return &MockGitHubAPI{
		releases: make(map[string][]string),
		errors:   make(map[string]error),
	}
}

// SetReleases sets the mock releases for a specific owner/repo.
func (m *MockGitHubAPI) SetReleases(owner, repo string, releases []string) {
	key := owner + "/" + repo
	m.releases[key] = releases
}

// SetError sets an error for a specific owner/repo.
func (m *MockGitHubAPI) SetError(owner, repo string, err error) {
	key := owner + "/" + repo
	m.errors[key] = err
}

// FetchReleases implements GitHubAPI interface.
func (m *MockGitHubAPI) FetchReleases(owner, repo string, limit int) ([]string, error) {
	key := owner + "/" + repo

	// Check if we should return an error
	if err, exists := m.errors[key]; exists {
		return nil, err
	}

	// Return mock releases
	if releases, exists := m.releases[key]; exists {
		return releases, nil
	}

	// Default: return empty list
	return []string{}, nil
}

func TestMockGitHubAPI(t *testing.T) {
	mock := NewMockGitHubAPI()

	// Test successful case
	mock.SetReleases("hashicorp", "terraform", []string{"1.12.0", "1.11.4", "1.9.8"})
	releases, err := mock.FetchReleases("hashicorp", "terraform", 10)
	require.NoError(t, err)
	assert.Equal(t, []string{"1.12.0", "1.11.4", "1.9.8"}, releases)

	// Test error case
	mock.SetError("nonexistent", "repo", assert.AnError)
	_, err = mock.FetchReleases("nonexistent", "repo", 10)
	assert.Error(t, err)

	// Test default case (no releases set)
	releases, err = mock.FetchReleases("unknown", "repo", 10)
	require.NoError(t, err)
	assert.Equal(t, []string{}, releases)
}

func TestSetAndResetGitHubAPI(t *testing.T) {
	// Test setting custom API
	mock := NewMockGitHubAPI()
	mock.SetReleases("test", "repo", []string{"1.0.0"})

	SetGitHubAPI(mock)

	// Test that the global API now uses our mock
	releases, err := fetchAllGitHubVersions("test", "repo", 10)
	require.NoError(t, err)
	assert.Equal(t, []string{"1.0.0"}, releases)

	// Reset to default
	ResetGitHubAPI()

	// Verify the API was reset by checking we can set a new mock.
	// We intentionally do NOT make real HTTP calls here as they can hang
	// indefinitely in CI environments without network access.
	newMock := NewMockGitHubAPI()
	newMock.SetReleases("verify", "reset", []string{"2.0.0"})
	SetGitHubAPI(newMock)

	releases, err = fetchAllGitHubVersions("verify", "reset", 10)
	require.NoError(t, err)
	assert.Equal(t, []string{"2.0.0"}, releases)

	// Clean up
	ResetGitHubAPI()
}
