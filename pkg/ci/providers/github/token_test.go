package github

import (
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/stretchr/testify/assert"
)

func TestGetCIGitHubToken(t *testing.T) {
	tests := []struct {
		name        string
		ciToken     string
		atmosToken  string
		githubToken string
		expected    string
	}{
		{
			name:        "ATMOS_CI_GITHUB_TOKEN takes highest precedence",
			ciToken:     "ci-token-value",
			atmosToken:  "atmos-token-value",
			githubToken: "github-token-value",
			expected:    "ci-token-value",
		},
		{
			name:        "ATMOS_CI_GITHUB_TOKEN alone",
			ciToken:     "ci-token-value",
			atmosToken:  "",
			githubToken: "",
			expected:    "ci-token-value",
		},
		{
			name:        "falls back to ATMOS_GITHUB_TOKEN",
			ciToken:     "",
			atmosToken:  "atmos-token-value",
			githubToken: "github-token-value",
			expected:    "atmos-token-value",
		},
		{
			name:        "falls back to GITHUB_TOKEN",
			ciToken:     "",
			atmosToken:  "",
			githubToken: "github-token-value",
			expected:    "github-token-value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ATMOS_CI_GITHUB_TOKEN", tt.ciToken)
			t.Setenv("ATMOS_GITHUB_TOKEN", tt.atmosToken)
			t.Setenv("GITHUB_TOKEN", tt.githubToken)

			result := GetCIGitHubToken()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetCIGitHubTokenOrError(t *testing.T) {
	t.Run("with CI token", func(t *testing.T) {
		t.Setenv("ATMOS_CI_GITHUB_TOKEN", "ci-test-token")
		t.Setenv("ATMOS_GITHUB_TOKEN", "")
		t.Setenv("GITHUB_TOKEN", "")

		token, err := GetCIGitHubTokenOrError()
		assert.NoError(t, err)
		assert.Equal(t, "ci-test-token", token)
	})

	t.Run("without token and no gh CLI", func(t *testing.T) {
		t.Setenv("ATMOS_CI_GITHUB_TOKEN", "")
		t.Setenv("ATMOS_GITHUB_TOKEN", "")
		t.Setenv("GITHUB_TOKEN", "")
		// Override PATH to prevent gh CLI fallback from finding a token.
		t.Setenv("PATH", t.TempDir())

		token, err := GetCIGitHubTokenOrError()
		assert.ErrorIs(t, err, errUtils.ErrGitHubTokenNotFound)
		assert.Empty(t, token)
	})
}
