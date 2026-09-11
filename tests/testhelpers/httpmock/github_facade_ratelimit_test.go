package httpmock

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitHubMockServer_RateLimit_Default(t *testing.T) {
	mock := NewGitHubMockServer(t)

	resp, err := http.Get(mock.URL() + "/api/v3/rate_limit")
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "5000", resp.Header.Get("X-RateLimit-Limit"))
	assert.Equal(t, "5000", resp.Header.Get("X-RateLimit-Remaining"))
	assert.NotEmpty(t, resp.Header.Get("X-RateLimit-Reset"))

	var body struct {
		Resources struct {
			Core struct {
				Limit     int   `json:"limit"`
				Remaining int   `json:"remaining"`
				Reset     int64 `json:"reset"`
			} `json:"core"`
		} `json:"resources"`
	}
	require.NoError(t, decodeJSON(resp.Body, &body))
	assert.Equal(t, 5000, body.Resources.Core.Limit)
	assert.Equal(t, 5000, body.Resources.Core.Remaining)
}

func TestGitHubMockServer_RateLimit_Configured(t *testing.T) {
	mock := NewGitHubMockServer(t)
	reset := time.Now().Add(10 * time.Minute).Truncate(time.Second)
	mock.SetRateLimit(0, reset)
	mock.RegisterRelease("owner", "repo", ReleaseSpec{TagName: "v1"})

	resp, err := http.Get(mock.URL() + "/api/v3/repos/owner/repo/releases/latest")
	require.NoError(t, err)
	defer resp.Body.Close()

	// A configured rate limit is stamped on every API response, not just
	// /rate_limit itself -- matching the real GitHub API.
	assert.Equal(t, "0", resp.Header.Get("X-RateLimit-Remaining"))
	assert.Equal(t, strconv.FormatInt(reset.Unix(), 10), resp.Header.Get("X-RateLimit-Reset"))
}

func TestGitHubMockServer_FailWithHeaders_SecondaryRateLimit(t *testing.T) {
	mock := NewGitHubMockServer(t)
	mock.FailWithHeaders("/api/v3/repos/owner/repo", http.StatusForbidden, map[string]string{"Retry-After": "30"})

	resp, err := http.Get(mock.URL() + "/api/v3/repos/owner/repo/releases/latest")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.Equal(t, "30", resp.Header.Get("Retry-After"))
}
