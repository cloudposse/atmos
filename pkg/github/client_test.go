package github

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v59/github"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestHandleGitHubAPIError tests the handleGitHubAPIError function.
func TestHandleGitHubAPIError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		resp      *github.Response
		wantErr   bool
		errString string
	}{
		{
			name: "rate limit exceeded with response",
			err:  errors.New("API rate limit exceeded"),
			resp: &github.Response{
				Rate: github.Rate{
					Remaining: 0,
					Limit:     5000,
					Reset:     github.Timestamp{Time: time.Now().Add(30 * time.Minute)},
				},
			},
			wantErr:   true,
			errString: "rate limit exceeded",
		},
		{
			name:      "non-rate-limit error",
			err:       errors.New("other error"),
			resp:      &github.Response{Rate: github.Rate{Remaining: 100}},
			wantErr:   true,
			errString: "other error",
		},
		{
			name:      "nil response",
			err:       errors.New("network error"),
			resp:      nil,
			wantErr:   true,
			errString: "network error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handleGitHubAPIError(tt.err, tt.resp)
			if tt.wantErr {
				assert.Error(t, err)
				// For rate limit errors, check the wrapped error type.
				if tt.name == "rate limit exceeded with response" {
					assert.ErrorIs(t, err, errUtils.ErrGitHubRateLimitExceeded)
				}
				// For other errors, verify the error message is preserved.
				if tt.name != "rate limit exceeded with response" {
					assert.Contains(t, err.Error(), tt.errString)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestHandleGitHubAPIError_RateLimitHintBranches tests that rate-limit errors
// produce different hints depending on whether a GitHub token is present.
func TestHandleGitHubAPIError_RateLimitHintBranches(t *testing.T) {
	rateLimitResp := &github.Response{
		Rate: github.Rate{
			Remaining: 0,
			Limit:     5000,
			Reset:     github.Timestamp{Time: time.Now().Add(30 * time.Minute)},
		},
	}
	originalErr := errors.New("rate limit")

	t.Run("rate limit with token includes token-specific hints", func(t *testing.T) {
		// Ensure a token is available so the "with token" branch is taken.
		t.Setenv("GITHUB_TOKEN", "ghp_test_token_for_coverage")

		err := handleGitHubAPIError(originalErr, rateLimitResp)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrGitHubRateLimitExceeded)
		// Verify the original error is preserved as cause.
		assert.ErrorIs(t, err, originalErr)
		// Verify formatted output contains rate limit explanation.
		formatted := errUtils.Format(err, errUtils.DefaultFormatterConfig())
		assert.Contains(t, formatted, "Rate limit exceeded")
		assert.Contains(t, formatted, "Hints")
	})

	t.Run("rate limit without token includes auth hints", func(t *testing.T) {
		// Clear all token sources: env vars and viper.
		t.Setenv("GITHUB_TOKEN", "")
		t.Setenv("ATMOS_GITHUB_TOKEN", "")
		viper.Set("github-token", "")
		defer viper.Set("github-token", "")
		// Override PATH to prevent gh CLI fallback from finding a token.
		t.Setenv("PATH", "")

		err := handleGitHubAPIError(originalErr, rateLimitResp)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrGitHubRateLimitExceeded)
		assert.ErrorIs(t, err, originalErr)
		formatted := errUtils.Format(err, errUtils.DefaultFormatterConfig())
		assert.Contains(t, formatted, "Rate limit exceeded")
		// Without token, hints should suggest authentication.
		assert.True(t, strings.Contains(formatted, "ATMOS_GITHUB_TOKEN") ||
			strings.Contains(formatted, "gh auth login"),
			"Expected auth hint in: %s", formatted)
	})
}

// TestNewGitHubClientWithToken tests the internal client creation with explicit tokens.
func TestNewGitHubClientWithToken(t *testing.T) {
	t.Run("creates unauthenticated client with empty token", func(t *testing.T) {
		client := newGitHubClientWithToken(t.Context(), "")
		assert.NotNil(t, client)
		assert.NotNil(t, client.Repositories)
	})

	t.Run("creates authenticated client with token", func(t *testing.T) {
		client := newGitHubClientWithToken(t.Context(), "ghp_test_token")
		assert.NotNil(t, client)
		assert.NotNil(t, client.Repositories)
	})
}
