package github

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v59/github"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// ghResp builds a *github.Response with the given HTTP status code and remaining rate.
func ghResp(statusCode, remaining int) *github.Response {
	return &github.Response{
		Response: &http.Response{StatusCode: statusCode},
		Rate: github.Rate{
			Remaining: remaining,
			Limit:     5000,
			Reset:     github.Timestamp{Time: time.Now().Add(30 * time.Minute)},
		},
	}
}

// TestHandleGitHubAPIError tests the handleGitHubAPIError function.
func TestHandleGitHubAPIError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		resp *github.Response
		// sentinel is the error we expect to be wrapped (nil means the original err is returned unchanged).
		sentinel error
		// origUnchanged asserts the original err is returned verbatim.
		origUnchanged bool
	}{
		{
			name:     "401 bad credentials maps to authentication failed, not rate limit",
			err:      errors.New("401 Bad credentials"),
			resp:     ghResp(http.StatusUnauthorized, 0),
			sentinel: errUtils.ErrAuthenticationFailed,
		},
		{
			name:     "403 with zero remaining is a rate limit",
			err:      errors.New("API rate limit exceeded"),
			resp:     ghResp(http.StatusForbidden, 0),
			sentinel: errUtils.ErrGitHubRateLimitExceeded,
		},
		{
			name:     "429 with zero remaining is a rate limit",
			err:      errors.New("secondary rate limit"),
			resp:     ghResp(http.StatusTooManyRequests, 0),
			sentinel: errUtils.ErrGitHubRateLimitExceeded,
		},
		{
			name:          "403 with remaining is a plain forbidden error, returned unchanged",
			err:           errors.New("forbidden"),
			resp:          ghResp(http.StatusForbidden, 100),
			origUnchanged: true,
		},
		{
			name:          "non-rate-limit error",
			err:           errors.New("other error"),
			resp:          ghResp(http.StatusInternalServerError, 100),
			origUnchanged: true,
		},
		{
			name:          "nil response",
			err:           errors.New("network error"),
			resp:          nil,
			origUnchanged: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handleGitHubAPIError(tt.err, tt.resp)
			require.Error(t, err)
			if tt.origUnchanged {
				assert.Equal(t, tt.err, err)
				return
			}
			assert.ErrorIs(t, err, tt.sentinel)
			// The original cause is always preserved.
			assert.ErrorIs(t, err, tt.err)
		})
	}
}

// TestHandleGitHubAPIError_RateLimitHintBranches tests that rate-limit errors
// produce different hints depending on whether a GitHub token is present.
func TestHandleGitHubAPIError_RateLimitHintBranches(t *testing.T) {
	rateLimitResp := ghResp(http.StatusForbidden, 0)
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
