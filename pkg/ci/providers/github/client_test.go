package github

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

const defaultAPIBaseURL = "https://api.github.com/"

// clearAPIURLEnv blanks every variable NewClient consults so tests are hermetic.
func clearAPIURLEnv(t *testing.T) {
	t.Helper()
	t.Setenv("ATMOS_CI_GITHUB_API_URL", "")
	t.Setenv("GITHUB_API_URL", "")
	t.Setenv("ATMOS_CI_GITHUB_TOKEN", "")
	t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "test-token")
}

func TestNewClient_APIURL(t *testing.T) {
	tests := []struct {
		name        string
		ciURL       string
		githubURL   string
		wantBaseURL string
		wantErr     bool
	}{
		{name: "default when unset", wantBaseURL: defaultAPIBaseURL},
		{name: "GITHUB_API_URL gets trailing slash", githubURL: "http://127.0.0.1:8080", wantBaseURL: "http://127.0.0.1:8080/"},
		{name: "GITHUB_API_URL keeps existing trailing slash", githubURL: "http://127.0.0.1:8080/", wantBaseURL: "http://127.0.0.1:8080/"},
		{name: "GHES path preserved", githubURL: "https://ghes.example.com/api/v3", wantBaseURL: "https://ghes.example.com/api/v3/"},
		{name: "ATMOS_CI_GITHUB_API_URL wins", ciURL: "http://override.example.com/api", githubURL: "https://ghes.example.com/api/v3", wantBaseURL: "http://override.example.com/api/"},
		{name: "unparsable URL errors", githubURL: "http://[::1", wantErr: true},
		{name: "relative URL errors", githubURL: "not-a-url", wantErr: true},
		{name: "unsupported scheme errors", githubURL: "ftp://example.com", wantErr: true},
		{name: "invalid override errors even when GITHUB_API_URL is valid", ciURL: "://bad", githubURL: "https://ghes.example.com/api/v3", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearAPIURLEnv(t)
			t.Setenv("ATMOS_CI_GITHUB_API_URL", tt.ciURL)
			t.Setenv("GITHUB_API_URL", tt.githubURL)

			client, err := NewClient()
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrInvalidURL)
				assert.Nil(t, client)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, client)
			assert.Equal(t, tt.wantBaseURL, client.GitHub().BaseURL.String())
			if tt.wantBaseURL != defaultAPIBaseURL {
				assert.Equal(t, tt.wantBaseURL, client.GitHub().UploadURL.String())
			}
		})
	}
}

func TestNewClient_TokenPrecedenceUnchanged(t *testing.T) {
	clearAPIURLEnv(t)
	t.Setenv("GITHUB_TOKEN", "")

	_, err := NewClient()
	require.ErrorIs(t, err, errUtils.ErrGitHubTokenNotFound)

	t.Setenv("GH_TOKEN", "gh-token-for-tests")
	client, err := NewClient()
	require.NoError(t, err)
	assert.NotNil(t, client)
}
