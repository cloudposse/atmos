package github

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-github/v59/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tests/testhelpers/httpmock"
)

// TestGetLatestRelease uses controlled release metadata instead of depending on
// the live repository's latest release being available during CI.
func TestGetLatestRelease(t *testing.T) {
	t.Run("fetches latest release from public repo", func(t *testing.T) {
		mock := setupMockGitHubClient(t)
		mock.RegisterRelease("owner", "repo", httpmock.ReleaseSpec{TagName: "v2.0.0"})
		mock.RegisterRelease("owner", "repo", httpmock.ReleaseSpec{TagName: "v1.0.0"})

		tag, err := GetLatestRelease("owner", "repo")

		require.NoError(t, err)
		assert.Equal(t, "v2.0.0", tag)
		assert.Equal(t, []httpmock.RequestLogEntry{{
			Method: http.MethodGet,
			Path:   "/api/v3/repos/owner/repo/releases/latest",
		}}, mock.Requests())
	})

	for _, tc := range []struct {
		name  string
		owner string
		repo  string
	}{
		{name: "handles repo without releases", owner: "owner", repo: "empty"},
		{name: "handles invalid owner", owner: "missing", repo: "repo"},
		{name: "handles invalid repo", owner: "owner", repo: "missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := setupMockGitHubClient(t)
			mock.RegisterRelease("owner", "repo", httpmock.ReleaseSpec{TagName: "v1.0.0"})

			tag, err := GetLatestRelease(tc.owner, tc.repo)

			require.Error(t, err)
			assert.Empty(t, tag)
			var apiError *github.ErrorResponse
			require.ErrorAs(t, err, &apiError)
			assert.Equal(t, http.StatusNotFound, apiError.Response.StatusCode)
			assert.Equal(t, []httpmock.RequestLogEntry{{
				Method: http.MethodGet,
				Path:   "/api/v3/repos/" + tc.owner + "/" + tc.repo + "/releases/latest",
			}}, mock.Requests())
		})
	}
}

// TestGetLatestReleaseWithAuthentication verifies the actual request header,
// without using CI credentials or the developer's GitHub CLI session.
func TestGetLatestReleaseWithAuthentication(t *testing.T) {
	for _, token := range []string{"", "test-release-token"} {
		name := "without token"
		if token != "" {
			name = "uses authentication when token available"
		}
		t.Run(name, func(t *testing.T) {
			clearGitHubEndpointEnv(t)
			t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")
			t.Setenv("ATMOS_GITHUB_TOKEN", "")
			t.Setenv("GITHUB_TOKEN", token)
			t.Setenv("ATMOS_GITHUB_CLI", "")
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/api/v3/repos/owner/repo/releases/latest", r.URL.Path)
				if token == "" {
					assert.Empty(t, r.Header.Get("Authorization"))
				} else {
					assert.Equal(t, "Bearer "+token, r.Header.Get("Authorization"))
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"tag_name":"v2.0.0"}`))
			}))
			t.Cleanup(server.Close)
			// Trust only the local test server's certificate for this sequential test.
			original := http.DefaultTransport
			http.DefaultTransport = server.Client().Transport
			t.Cleanup(func() { http.DefaultTransport = original })
			t.Setenv("GITHUB_SERVER_URL", server.URL)
			t.Setenv("GITHUB_API_URL", server.URL+"/api/v3")

			tag, err := GetLatestRelease("owner", "repo")

			require.NoError(t, err)
			assert.Equal(t, "v2.0.0", tag)
		})
	}
}

// TestGetLatestReleaseInfo exercises stable-release metadata and real 404 errors.
func TestGetLatestReleaseInfo(t *testing.T) {
	t.Run("fetches latest stable release", func(t *testing.T) {
		mock := setupMockGitHubClient(t)
		mock.RegisterRelease("owner", "repo", httpmock.ReleaseSpec{TagName: "v3.0.0-rc.1", Prerelease: true})
		mock.RegisterRelease("owner", "repo", httpmock.ReleaseSpec{TagName: "v3.0.0", Draft: true})
		mock.RegisterRelease("owner", "repo", httpmock.ReleaseSpec{TagName: "v2.0.0"})

		release, err := GetLatestReleaseInfo("owner", "repo")

		require.NoError(t, err)
		require.NotNil(t, release)
		assert.Equal(t, "v2.0.0", release.GetTagName())
		assert.False(t, release.GetPrerelease(), "Latest release should not be a prerelease")
		assert.False(t, release.GetDraft(), "Latest release should not be a draft")
		assert.Equal(t, []httpmock.RequestLogEntry{{
			Method: http.MethodGet,
			Path:   "/api/v3/repos/owner/repo/releases/latest",
		}}, mock.Requests())
	})

	t.Run("returns error when no release exists", func(t *testing.T) {
		setupMockGitHubClient(t)

		release, err := GetLatestReleaseInfo("owner", "repo")

		require.Error(t, err)
		assert.Nil(t, release)
		var apiError *github.ErrorResponse
		require.ErrorAs(t, err, &apiError)
		assert.Equal(t, http.StatusNotFound, apiError.Response.StatusCode)
	})
}
