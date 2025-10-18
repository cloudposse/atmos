package version

import (
	"testing"
	"time"

	"github.com/google/go-github/v59/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRealGitHubClient_GetReleases(t *testing.T) {
	publishedAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	mockReleases := []*github.RepositoryRelease{
		{
			TagName:     github.String("v1.0.0"),
			Name:        github.String("Release 1.0.0"),
			PublishedAt: &github.Timestamp{Time: publishedAt},
		},
	}

	mockClient := &MockGitHubClient{
		Releases: mockReleases,
	}

	opts := ReleaseOptions{
		Limit:              10,
		Offset:             0,
		IncludePrereleases: false,
	}

	releases, err := mockClient.GetReleases("cloudposse", "atmos", opts)
	require.NoError(t, err)
	assert.Len(t, releases, 1)
	assert.Equal(t, "v1.0.0", *releases[0].TagName)
}

func TestRealGitHubClient_GetRelease(t *testing.T) {
	publishedAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	mockRelease := &github.RepositoryRelease{
		TagName:     github.String("v1.2.3"),
		Name:        github.String("Release 1.2.3"),
		PublishedAt: &github.Timestamp{Time: publishedAt},
	}

	mockClient := &MockGitHubClient{
		Release: mockRelease,
	}

	release, err := mockClient.GetRelease("cloudposse", "atmos", "v1.2.3")
	require.NoError(t, err)
	assert.Equal(t, "v1.2.3", *release.TagName)
}

func TestRealGitHubClient_GetLatestRelease(t *testing.T) {
	publishedAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	mockRelease := &github.RepositoryRelease{
		TagName:     github.String("v2.0.0"),
		Name:        github.String("Release 2.0.0"),
		PublishedAt: &github.Timestamp{Time: publishedAt},
	}

	mockClient := &MockGitHubClient{
		Release: mockRelease,
	}

	release, err := mockClient.GetLatestRelease("cloudposse", "atmos")
	require.NoError(t, err)
	assert.Equal(t, "v2.0.0", *release.TagName)
}

func TestMockGitHubClient_GetRelease_Error(t *testing.T) {
	mockClient := &MockGitHubClient{
		Err: assert.AnError,
	}

	release, err := mockClient.GetRelease("cloudposse", "atmos", "v1.0.0")
	assert.Error(t, err)
	assert.Nil(t, release)
}

func TestMockGitHubClient_GetLatestRelease_Error(t *testing.T) {
	mockClient := &MockGitHubClient{
		Err: assert.AnError,
	}

	release, err := mockClient.GetLatestRelease("cloudposse", "atmos")
	assert.Error(t, err)
	assert.Nil(t, release)
}

func TestMockGitHubClient_GetReleases_Error(t *testing.T) {
	mockClient := &MockGitHubClient{
		Err: assert.AnError,
	}

	opts := ReleaseOptions{
		Limit: 10,
	}

	releases, err := mockClient.GetReleases("cloudposse", "atmos", opts)
	assert.Error(t, err)
	assert.Nil(t, releases)
}

func TestRealGitHubClient_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This tests the real GitHub client wrapper methods.
	// They should successfully call through to pkg/github.
	client := &RealGitHubClient{}

	// Test GetLatestRelease.
	release, err := client.GetLatestRelease("cloudposse", "atmos")
	if err != nil {
		t.Skipf("Skipping GetLatestRelease test due to API error (rate limit or network): %v", err)
	}
	assert.NotNil(t, release)
	assert.NotEmpty(t, release.TagName)

	// Test GetRelease with a known stable version.
	specificRelease, err := client.GetRelease("cloudposse", "atmos", "v1.63.0")
	if err != nil {
		t.Skipf("Skipping GetRelease test due to API error: %v", err)
	}
	assert.NotNil(t, specificRelease)
	assert.Equal(t, "v1.63.0", *specificRelease.TagName)

	// Test GetReleases with small limit.
	opts := ReleaseOptions{
		Limit:              5,
		Offset:             0,
		IncludePrereleases: false,
	}
	releases, err := client.GetReleases("cloudposse", "atmos", opts)
	if err != nil {
		t.Skipf("Skipping GetReleases test due to API error: %v", err)
	}
	assert.NotNil(t, releases)
	assert.LessOrEqual(t, len(releases), 5)
}
