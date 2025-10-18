package version

import (
	"testing"
	"time"

	"github.com/google/go-github/v59/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShowCommand_Flags(t *testing.T) {
	// Test that show command has required flags.
	formatFlag := showCmd.Flags().Lookup("format")
	assert.NotNil(t, formatFlag)
	assert.Equal(t, "text", formatFlag.DefValue)
}

func TestShowCommand_BasicProperties(t *testing.T) {
	assert.Equal(t, "show [version]", showCmd.Use)
	assert.NotEmpty(t, showCmd.Short)
	assert.NotEmpty(t, showCmd.Long)
	assert.NotEmpty(t, showCmd.Example)
	assert.NotNil(t, showCmd.RunE)
	assert.NotNil(t, showCmd.Args)
}

func TestFetchRelease_Latest(t *testing.T) {
	publishedAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	mockRelease := &github.RepositoryRelease{
		TagName:     github.String("v1.0.0"),
		Name:        github.String("Release 1.0.0"),
		PublishedAt: &github.Timestamp{Time: publishedAt},
	}

	client := &MockGitHubClient{
		Release: mockRelease,
	}

	release, err := fetchRelease(client, "latest")
	require.NoError(t, err)
	assert.Equal(t, "v1.0.0", *release.TagName)
}

func TestFetchRelease_SpecificVersion(t *testing.T) {
	publishedAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	mockRelease := &github.RepositoryRelease{
		TagName:     github.String("v1.2.3"),
		Name:        github.String("Release 1.2.3"),
		PublishedAt: &github.Timestamp{Time: publishedAt},
	}

	client := &MockGitHubClient{
		Release: mockRelease,
	}

	release, err := fetchRelease(client, "v1.2.3")
	require.NoError(t, err)
	assert.Equal(t, "v1.2.3", *release.TagName)
}

func TestFetchReleaseWithSpinner_Mock(t *testing.T) {
	publishedAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	mockRelease := &github.RepositoryRelease{
		TagName:     github.String("v1.0.0"),
		Name:        github.String("Release 1.0.0"),
		PublishedAt: &github.Timestamp{Time: publishedAt},
	}

	client := &MockGitHubClient{
		Release: mockRelease,
	}

	release, err := fetchReleaseWithSpinner(client, "v1.0.0")
	require.NoError(t, err)
	assert.Equal(t, "v1.0.0", *release.TagName)
}

func TestFetchReleaseWithSpinner_Error(t *testing.T) {
	client := &MockGitHubClient{
		Err: assert.AnError,
	}

	release, err := fetchReleaseWithSpinner(client, "v1.0.0")
	assert.Error(t, err)
	assert.Nil(t, release)
}

func TestShowModel_Init(t *testing.T) {
	m := &showModel{}

	cmd := m.Init()
	assert.NotNil(t, cmd)
}

func TestShowModel_View(t *testing.T) {
	tests := []struct {
		name     string
		done     bool
		expected string
	}{
		{
			name:     "done returns empty string",
			done:     true,
			expected: "",
		},
		{
			name:     "not done shows spinner message",
			done:     false,
			expected: "Fetching release from GitHub...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &showModel{
				done: tt.done,
			}

			view := m.View()

			if tt.done {
				assert.Empty(t, view)
			} else {
				assert.Contains(t, view, tt.expected)
			}
		})
	}
}

func TestShowModel_Update_Success(t *testing.T) {
	publishedAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	mockRelease := &github.RepositoryRelease{
		TagName:     github.String("v1.0.0"),
		Name:        github.String("Release 1.0.0"),
		PublishedAt: &github.Timestamp{Time: publishedAt},
	}

	m := &showModel{
		done: false,
		err:  nil,
	}

	// Simulate receiving release message (the actual message type).
	updatedModel, _ := m.Update(mockRelease)
	finalModel, ok := updatedModel.(*showModel)
	require.True(t, ok)
	assert.True(t, finalModel.done)
	assert.Equal(t, "v1.0.0", *finalModel.release.TagName)
}

func TestShowModel_Update_Error(t *testing.T) {
	m := &showModel{
		done: false,
		err:  nil,
	}

	// Simulate receiving an error message.
	testErr := assert.AnError
	updatedModel, _ := m.Update(testErr)
	finalModel, ok := updatedModel.(*showModel)
	require.True(t, ok)
	assert.True(t, finalModel.done)
	assert.Error(t, finalModel.err)
}

func TestShowModel_Update_Default(t *testing.T) {
	m := &showModel{
		done: false,
		err:  nil,
	}

	// Simulate receiving an unknown message type.
	updatedModel, cmd := m.Update("unknown")
	finalModel, ok := updatedModel.(*showModel)
	require.True(t, ok)
	assert.False(t, finalModel.done)
	assert.Nil(t, cmd)
}
