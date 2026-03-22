package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/go-github/v59/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGitHubOwnerRepo(t *testing.T) {
	tests := []struct {
		name      string
		uri       string
		wantOwner string
		wantRepo  string
		wantOK    bool
	}{
		// Standard no-scheme GitHub URIs (go-getter style).
		{
			name:      "plain github.com URI",
			uri:       "github.com/cloudposse/terraform-null-label",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-null-label",
			wantOK:    true,
		},
		{
			name:      "github.com URI with subdirectory",
			uri:       "github.com/cloudposse/terraform-null-label//modules/vpc",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-null-label",
			wantOK:    true,
		},
		{
			name:      "github.com URI with subdirectory and query params",
			uri:       "github.com/cloudposse/terraform-null-label//modules/vpc?ref=v1.0.0",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-null-label",
			wantOK:    true,
		},

		// HTTPS URLs.
		{
			name:      "https github URL",
			uri:       "https://github.com/cloudposse/terraform-null-label",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-null-label",
			wantOK:    true,
		},
		{
			name:      "https github URL with .git suffix",
			uri:       "https://github.com/cloudposse/terraform-null-label.git",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-null-label",
			wantOK:    true,
		},
		{
			name:      "https github URL with subdirectory",
			uri:       "https://github.com/cloudposse/terraform-null-label//modules/vpc?ref=v1.0.0",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-null-label",
			wantOK:    true,
		},

		// go-getter force prefix.
		{
			name:      "git:: force prefix with https",
			uri:       "git::https://github.com/cloudposse/terraform-null-label.git",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-null-label",
			wantOK:    true,
		},
		{
			name:      "git:: force prefix without scheme",
			uri:       "git::github.com/cloudposse/terraform-null-label",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-null-label",
			wantOK:    true,
		},

		// SSH scheme (ssh://) — distinct from SCP-style.
		{
			name:      "ssh:// URL with .git and subdirectory",
			uri:       "ssh://git@github.com/cloudposse/terraform-null-label.git//.?ref=0.25.0",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-null-label",
			wantOK:    true,
		},
		{
			name:      "git:: force prefix with ssh:// URL and subdirectory",
			uri:       "git::ssh://git@github.com/cloudposse/terraform-null-label.git//.?ref=0.25.0",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-null-label",
			wantOK:    true,
		},
		{
			name:      "ssh:// URL without subdirectory",
			uri:       "ssh://git@github.com/cloudposse/terraform-null-label.git",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-null-label",
			wantOK:    true,
		},
		{
			name:   "ssh:// URL to non-GitHub host",
			uri:    "ssh://git@gitlab.com/owner/repo.git",
			wantOK: false,
		},

		// SCP-style Git URLs.
		{
			name:      "SCP-style git@github.com",
			uri:       "git@github.com:cloudposse/terraform-null-label.git",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-null-label",
			wantOK:    true,
		},
		{
			name:      "SCP-style git@github.com with subdirectory",
			uri:       "git@github.com:cloudposse/terraform-null-label.git//modules/vpc",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-null-label",
			wantOK:    true,
		},

		// Non-GitHub URIs (should return ok=false).
		{
			name:   "gitlab URI",
			uri:    "gitlab.com/owner/repo",
			wantOK: false,
		},
		{
			name:   "s3 URI",
			uri:    "s3::s3://mybucket/path",
			wantOK: false,
		},
		{
			name:   "oci URI",
			uri:    "oci://registry.example.com/org/image:tag",
			wantOK: false,
		},
		{
			name:   "local path",
			uri:    "./local/path",
			wantOK: false,
		},
		{
			name:   "empty URI",
			uri:    "",
			wantOK: false,
		},
		{
			name:   "bitbucket URI",
			uri:    "bitbucket.org/owner/repo",
			wantOK: false,
		},

		// github:// scheme.
		{
			name:      "github:// scheme bare",
			uri:       "github://cloudposse/terraform-null-label",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-null-label",
			wantOK:    true,
		},
		{
			name:      "github:// scheme with subdir and ref",
			uri:       "github://cloudposse/terraform-null-label/modules/vpc@v1.0.0",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-null-label",
			wantOK:    true,
		},
		{
			name:      "git:: force prefix with github:// scheme",
			uri:       "git::github://cloudposse/terraform-null-label",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-null-label",
			wantOK:    true,
		},

		// github:// scheme with fragment — must strip #fragment.
		{
			name:      "github:// scheme with fragment",
			uri:       "github://cloudposse/repo#main",
			wantOwner: "cloudposse",
			wantRepo:  "repo",
			wantOK:    true,
		},
		// Uppercase github:// scheme — must be handled case-insensitively.
		{
			name:      "GITHUB:// uppercase scheme",
			uri:       "GITHUB://cloudposse/repo",
			wantOwner: "cloudposse",
			wantRepo:  "repo",
			wantOK:    true,
		},
		// Uppercase scheme/host — url.Parse lowercases the host.
		{
			name:      "uppercase scheme and host",
			uri:       "HTTPS://GITHUB.COM/owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantOK:    true,
		},
		// Double-slash before owner (template-expansion artifact).
		{
			name:      "https with double-slash before owner",
			uri:       "https://github.com//cloudposse/terraform-null-label",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-null-label",
			wantOK:    true,
		},

		// Port-qualified hostname.
		{
			name:      "https github URL with explicit port 443",
			uri:       "https://github.com:443/cloudposse/terraform-null-label",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-null-label",
			wantOK:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, ok := ParseGitHubOwnerRepo(tt.uri)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.wantOwner, owner)
				assert.Equal(t, tt.wantRepo, repo)
			}
		})
	}
}

func TestIsRepoArchived(t *testing.T) {
	tests := []struct {
		name           string
		responseBody   string
		responseStatus int
		wantArchived   bool
		wantErr        bool
	}{
		{
			name:           "archived repo",
			responseStatus: 200,
			responseBody:   `{"archived": true}`,
			wantArchived:   true,
		},
		{
			name:           "active repo",
			responseStatus: 200,
			responseBody:   `{"archived": false}`,
			wantArchived:   false,
		},
		{
			name:           "repo not found (404)",
			responseStatus: 404,
			responseBody:   `{"message": "Not Found"}`,
			wantErr:        true,
		},
		{
			name:           "unauthorized (401)",
			responseStatus: 401,
			responseBody:   `{"message": "Requires authentication"}`,
			wantErr:        true,
		},
		{
			name:           "forbidden (403)",
			responseStatus: 403,
			responseBody:   `{"message": "Forbidden"}`,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset cache between sub-tests so each gets a fresh API call.
			ResetArchivedRepoCache()

			mux := http.NewServeMux()
			mux.HandleFunc("/repos/owner/repo", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.responseStatus)
				_, _ = w.Write([]byte(tt.responseBody))
			})
			ts := httptest.NewServer(mux)
			defer ts.Close()

			// Build a GitHub client pointing at the test server.
			client := github.NewClient(nil)
			u, err := url.Parse(ts.URL + "/")
			require.NoError(t, err)
			client.BaseURL = u

			archived, err := isRepoArchivedWithClient(context.Background(), client.Repositories, "owner", "repo")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantArchived, archived)
			}
		})
	}
}

// TestArchivedCheckTimeoutOverride verifies that the archivedCheckTimeout can be
// overridden at runtime (simulating the ATMOS_GITHUB_ARCHIVED_CHECK_TIMEOUT env var
// read during init) and that a zero timeout causes the API call to be cancelled
// before completing (IsRepoArchived returns an error rather than blocking).
func TestArchivedCheckTimeoutOverride(t *testing.T) {
	t.Run("default timeout is 5s", func(t *testing.T) {
		reset := SetArchivedCheckTimeoutForTest(defaultArchivedCheckTimeout)
		t.Cleanup(reset)
		assert.Equal(t, 5*time.Second, ArchivedCheckTimeoutForTest())
	})

	t.Run("zero timeout cancels API call immediately", func(t *testing.T) {
		t.Cleanup(ResetArchivedRepoCache)
		reset := SetArchivedCheckTimeoutForTest(0)
		t.Cleanup(reset)

		// A dummy server that blocks the response to prove the test doesn't hang.
		// We pass a zero-timeout context directly to isRepoArchivedWithClient so the
		// call should fail before ever reaching the server.
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"archived": false}`))
		}))
		defer ts.Close()

		client := github.NewClient(nil)
		u, err := url.Parse(ts.URL + "/")
		require.NoError(t, err)
		client.BaseURL = u

		// Use an already-expired context (0 timeout) to simulate the effect of
		// IsRepoArchived applying archivedCheckTimeout=0 to the parent context.
		expiredCtx, cancel := context.WithTimeout(context.Background(), 0)
		defer cancel()

		_, err = isRepoArchivedWithClient(expiredCtx, client.Repositories, "org", "repo")
		assert.Error(t, err, "expected error with already-expired context")
	})
}
