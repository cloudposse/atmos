package registry

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newHeader builds an http.Header via Set (which canonicalizes keys), so
// header names like "X-RateLimit-Remaining" round-trip through Get the same
// way they would from a real HTTP response — a raw map literal keyed by the
// non-canonical string silently fails to match.
func newHeader(pairs ...string) http.Header {
	h := http.Header{}
	for i := 0; i+1 < len(pairs); i += 2 {
		h.Set(pairs[i], pairs[i+1])
	}
	return h
}

func TestIsRetryableGitHubStatus(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		header     http.Header
		want       bool
	}{
		{"429 is always retryable, even with no headers", http.StatusTooManyRequests, http.Header{}, true},
		{"403 with Retry-After is a genuine rate limit", http.StatusForbidden, newHeader("Retry-After", "1"), true},
		{"403 with X-RateLimit-Remaining: 0 is a genuine rate limit", http.StatusForbidden, newHeader("X-RateLimit-Remaining", "0"), true},
		{"403 with no rate-limit signal is a terminal authorization failure", http.StatusForbidden, http.Header{}, false},
		{"403 with X-RateLimit-Remaining non-zero is terminal", http.StatusForbidden, newHeader("X-RateLimit-Remaining", "42"), false},
		{"500 is retryable", http.StatusInternalServerError, http.Header{}, true},
		{"502 is retryable", http.StatusBadGateway, http.Header{}, true},
		{"503 is retryable", http.StatusServiceUnavailable, http.Header{}, true},
		{"404 is terminal", http.StatusNotFound, http.Header{}, false},
		{"400 is terminal", http.StatusBadRequest, http.Header{}, false},
		{"200 is not applicable but not retryable", http.StatusOK, http.Header{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsRetryableGitHubStatus(tt.statusCode, tt.header))
		})
	}
}

func TestGitHubSignalsRateLimit(t *testing.T) {
	tests := []struct {
		name   string
		header http.Header
		want   bool
	}{
		{"Retry-After present", newHeader("Retry-After", "1"), true},
		{"X-RateLimit-Remaining: 0", newHeader("X-RateLimit-Remaining", "0"), true},
		{"X-RateLimit-Remaining non-zero", newHeader("X-RateLimit-Remaining", "10"), false},
		{"no headers", http.Header{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, GitHubSignalsRateLimit(tt.header))
		})
	}
}

func TestGitHubRetryAfter(t *testing.T) {
	t.Run("Retry-After takes precedence over X-RateLimit-Reset", func(t *testing.T) {
		header := newHeader(
			"Retry-After", "5",
			"X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(1*time.Hour).Unix(), 10),
		)
		wait, ok := GitHubRetryAfter(header)
		require.True(t, ok)
		assert.Equal(t, 5*time.Second, wait)
	})

	t.Run("falls back to X-RateLimit-Reset when Retry-After absent", func(t *testing.T) {
		reset := strconv.FormatInt(time.Now().Add(3*time.Second).Unix(), 10)
		wait, ok := GitHubRetryAfter(newHeader("X-RateLimit-Reset", reset))
		require.True(t, ok)
		assert.Positive(t, wait)
		assert.LessOrEqual(t, wait, 3*time.Second)
	})

	t.Run("X-RateLimit-Reset already in the past resolves to zero wait", func(t *testing.T) {
		reset := strconv.FormatInt(time.Now().Add(-1*time.Hour).Unix(), 10)
		wait, ok := GitHubRetryAfter(newHeader("X-RateLimit-Reset", reset))
		require.True(t, ok)
		assert.Equal(t, time.Duration(0), wait)
	})

	t.Run("no headers", func(t *testing.T) {
		_, ok := GitHubRetryAfter(http.Header{})
		assert.False(t, ok)
	})

	t.Run("unparseable Retry-After falls through to no-header result", func(t *testing.T) {
		_, ok := GitHubRetryAfter(newHeader("Retry-After", "not-a-number"))
		assert.False(t, ok)
	})

	t.Run("negative Retry-After is rejected", func(t *testing.T) {
		_, ok := GitHubRetryAfter(newHeader("Retry-After", "-1"))
		assert.False(t, ok)
	})
}

func TestHTTPStatusError(t *testing.T) {
	err := &HTTPStatusError{StatusCode: http.StatusTooManyRequests, Header: http.Header{}, URL: "https://api.github.com/repos/x/y"}

	assert.Contains(t, err.Error(), "429")
	assert.Contains(t, err.Error(), "https://api.github.com/repos/x/y")
	assert.ErrorIs(t, err, ErrHTTPRequest)
}
