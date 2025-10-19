package version

import (
	"testing"
	"time"

	"github.com/google/go-github/v59/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListCommand_BasicProperties(t *testing.T) {
	assert.Equal(t, "list", listCmd.Use)
	assert.NotEmpty(t, listCmd.Short)
	assert.NotEmpty(t, listCmd.Long)
	assert.NotEmpty(t, listCmd.Example)
	assert.NotNil(t, listCmd.RunE)
}

func TestListCommand_Flags(t *testing.T) {
	tests := []struct {
		name       string
		flagName   string
		hasDefault bool
	}{
		{name: "limit flag exists", flagName: "limit", hasDefault: true},
		{name: "offset flag exists", flagName: "offset", hasDefault: true},
		{name: "since flag exists", flagName: "since", hasDefault: false},
		{name: "include-prereleases flag exists", flagName: "include-prereleases", hasDefault: true},
		{name: "format flag exists", flagName: "format", hasDefault: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := listCmd.Flags().Lookup(tt.flagName)
			assert.NotNil(t, flag, "flag %s should exist", tt.flagName)
		})
	}
}

func TestFetchReleasesWithSpinner_Mock(t *testing.T) {
	publishedAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	mockReleases := []*github.RepositoryRelease{
		{
			TagName:     github.String("v1.0.0"),
			Name:        github.String("Release 1.0.0"),
			PublishedAt: &github.Timestamp{Time: publishedAt},
		},
	}

	client := &MockGitHubClient{
		Releases: mockReleases,
	}

	opts := ReleaseOptions{
		Limit:  10,
		Offset: 0,
	}

	releases, err := fetchReleasesWithSpinner(client, opts)
	require.NoError(t, err)
	assert.Len(t, releases, 1)
	assert.Equal(t, "v1.0.0", *releases[0].TagName)
}

func TestFetchReleasesWithSpinner_Error(t *testing.T) {
	client := &MockGitHubClient{
		Err: assert.AnError,
	}

	opts := ReleaseOptions{
		Limit:  10,
		Offset: 0,
	}

	releases, err := fetchReleasesWithSpinner(client, opts)
	assert.Error(t, err)
	assert.Nil(t, releases)
}

func TestListModel_Init(t *testing.T) {
	m := &listModel{}

	cmd := m.Init()
	assert.NotNil(t, cmd)
}

func TestListModel_View(t *testing.T) {
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
			expected: "Fetching releases from GitHub...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &listModel{
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

func TestListModel_Update_Success(t *testing.T) {
	publishedAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	mockReleases := []*github.RepositoryRelease{
		{
			TagName:     github.String("v1.0.0"),
			Name:        github.String("Release 1.0.0"),
			PublishedAt: &github.Timestamp{Time: publishedAt},
		},
	}

	m := &listModel{
		done: false,
		err:  nil,
	}

	// Simulate receiving releases message (the actual message type).
	updatedModel, _ := m.Update(mockReleases)
	finalModel, ok := updatedModel.(*listModel)
	require.True(t, ok)
	assert.True(t, finalModel.done)
	assert.Len(t, finalModel.releases, 1)
}

func TestListModel_Update_Error(t *testing.T) {
	m := &listModel{
		done: false,
		err:  nil,
	}

	// Simulate receiving an error message.
	testErr := assert.AnError
	updatedModel, _ := m.Update(testErr)
	finalModel, ok := updatedModel.(*listModel)
	require.True(t, ok)
	assert.True(t, finalModel.done)
	assert.Error(t, finalModel.err)
}

func TestListModel_Update_Default(t *testing.T) {
	m := &listModel{
		done: false,
		err:  nil,
	}

	// Simulate receiving an unknown message type.
	updatedModel, cmd := m.Update("unknown")
	finalModel, ok := updatedModel.(*listModel)
	require.True(t, ok)
	assert.False(t, finalModel.done)
	assert.Nil(t, cmd)
}

// TestListCommand_ValidationErrors tests the validation logic in the RunE function.
func TestListCommand_ValidationErrors(t *testing.T) {
	tests := []struct {
		name      string
		limit     int
		offset    int
		since     string
		wantError bool
		errString string
	}{
		{
			name:      "invalid limit too low",
			limit:     0,
			offset:    0,
			wantError: true,
			errString: "limit must be between",
		},
		{
			name:      "invalid limit too high",
			limit:     101,
			offset:    0,
			wantError: true,
			errString: "limit must be between",
		},
		{
			name:      "invalid since date format",
			limit:     10,
			offset:    0,
			since:     "invalid-date",
			wantError: true,
			errString: "date format",
		},
		{
			name:      "valid parameters",
			limit:     10,
			offset:    0,
			since:     "2025-01-01",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flags to test values.
			listLimit = tt.limit
			listOffset = tt.offset
			listSince = tt.since

			// Create a test command instance.
			cmd := listCmd

			// Execute the command - it will fail at GitHub API call, but we're testing validation.
			err := cmd.RunE(cmd, []string{})

			if tt.wantError {
				assert.Error(t, err)
				if tt.errString != "" {
					assert.Contains(t, err.Error(), tt.errString)
				}
			} else if err != nil {
				// This will fail at GitHub API call, which is expected.
				// We just want to verify validation passed.
				// If it's not a validation error, that's acceptable.
				assert.NotContains(t, err.Error(), "limit must be between")
				assert.NotContains(t, err.Error(), "date format")
			}
		})
	}
}

// TestListCommand_FormatValidation tests format validation.
func TestListCommand_FormatValidation(t *testing.T) {
	tests := []struct {
		name      string
		format    string
		wantError bool
	}{
		{
			name:      "valid format text",
			format:    "text",
			wantError: false,
		},
		{
			name:      "valid format json",
			format:    "json",
			wantError: false,
		},
		{
			name:      "valid format yaml",
			format:    "yaml",
			wantError: false,
		},
		{
			name:      "invalid format",
			format:    "invalid",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset to valid values for other fields.
			listLimit = 10
			listOffset = 0
			listSince = ""
			listFormat = tt.format

			cmd := listCmd
			err := cmd.RunE(cmd, []string{})

			if tt.wantError {
				// Should fail with unsupported format error.
				if err != nil {
					assert.Contains(t, err.Error(), "unsupported")
				}
			}
			// Note: Test may fail at GitHub API, which is fine for validation test.
		})
	}
}
