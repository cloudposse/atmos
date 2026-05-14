package github

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetGitHubToken(t *testing.T) {
	tests := []struct {
		name           string
		atmosToken     string
		githubToken    string
		expectedPrefix string // Token should start with this.
		expectEmpty    bool
	}{
		{
			name:        "no tokens set",
			atmosToken:  "",
			githubToken: "",
			expectEmpty: true, // May get token from gh CLI if installed.
		},
		{
			name:           "ATMOS_GITHUB_TOKEN set",
			atmosToken:     "atmos_token_123",
			githubToken:    "",
			expectedPrefix: "atmos_token_123",
		},
		{
			name:           "GITHUB_TOKEN set",
			atmosToken:     "",
			githubToken:    "github_token_456",
			expectedPrefix: "github_token_456",
		},
		{
			name:           "both tokens set - ATMOS takes precedence",
			atmosToken:     "atmos_token_789",
			githubToken:    "github_token_abc",
			expectedPrefix: "atmos_token_789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up env vars using t.Setenv for automatic cleanup.
			t.Setenv("ATMOS_GITHUB_TOKEN", tt.atmosToken)
			t.Setenv("GITHUB_TOKEN", tt.githubToken)

			token := GetGitHubToken()

			if tt.expectEmpty && tt.atmosToken == "" && tt.githubToken == "" {
				// With no env vars set, token comes from gh CLI (if installed) or is empty.
				// Either outcome is valid — we just verify it's not a test fixture value.
				if token != "" {
					assert.NotEqual(t, "atmos_token_123", token, "should not be a test fixture value")
					assert.NotEqual(t, "github_token_456", token, "should not be a test fixture value")
				}
			} else if tt.expectedPrefix != "" {
				assert.Equal(t, tt.expectedPrefix, token)
			}
		})
	}
}

func TestGetGitHubTokenOrError(t *testing.T) {
	t.Run("with token", func(t *testing.T) {
		t.Setenv("ATMOS_GITHUB_TOKEN", "test_token")
		t.Setenv("GITHUB_TOKEN", "")

		token, err := GetGitHubTokenOrError()
		assert.NoError(t, err)
		assert.Equal(t, "test_token", token)
	})

	t.Run("without token and no gh CLI", func(t *testing.T) {
		t.Setenv("ATMOS_GITHUB_TOKEN", "")
		t.Setenv("GITHUB_TOKEN", "")

		token, err := GetGitHubTokenOrError()
		// If gh CLI is installed, this will succeed.
		// If not, it will return ErrGitHubTokenRequired.
		if err != nil {
			assert.ErrorIs(t, err, ErrGitHubTokenRequired)
			assert.Empty(t, token)
		} else {
			// gh CLI returned a token.
			assert.NotEmpty(t, token)
		}
	})
}
