package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-github/v59/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tests"
)

// TestGetArchivedStatus_MockServer exercises getArchivedStatus (the client-injectable core of
// IsArchived) against a mock HTTP server, mirroring TestFetchAllReleases_MockServer's convention
// in releases_test.go.
func TestGetArchivedStatus_MockServer(t *testing.T) {
	t.Run("reports archived=true", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/owner/archived-repo", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(&github.Repository{Archived: github.Bool(true)})
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		client := newTestClient(t, server.URL)
		archived, err := getArchivedStatus(context.Background(), client, "owner", "archived-repo")

		require.NoError(t, err)
		assert.True(t, archived)
	})

	t.Run("reports archived=false", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/owner/active-repo", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(&github.Repository{Archived: github.Bool(false)})
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		client := newTestClient(t, server.URL)
		archived, err := getArchivedStatus(context.Background(), client, "owner", "active-repo")

		require.NoError(t, err)
		assert.False(t, archived)
	})

	t.Run("returns wrapped error on API failure", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/owner/missing-repo", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "Not Found"})
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		client := newTestClient(t, server.URL)
		archived, err := getArchivedStatus(context.Background(), client, "owner", "missing-repo")

		require.Error(t, err)
		assert.False(t, archived)
	})
}

// TestIsArchived_LiveNetwork proves IsArchived's plumbing (newGitHubClient -> getArchivedStatus)
// end-to-end against the real GitHub API, matching the live-network test convention used
// elsewhere in this package (TestGetLatestRelease etc.), gated on GitHub access.
func TestIsArchived_LiveNetwork(t *testing.T) {
	rateLimits := tests.RequireGitHubAccess(t)
	if rateLimits != nil && rateLimits.Remaining < 5 {
		t.Skipf("Need at least 5 GitHub API requests, only %d remaining", rateLimits.Remaining)
	}

	archived, err := IsArchived(context.Background(), "cloudposse", "atmos")
	if isGitHubTransientError(err) {
		t.Skipf("Skipping due to transient GitHub API error: %v", err)
	}
	require.NoError(t, err)
	assert.False(t, archived, "cloudposse/atmos is expected to be an active, non-archived repository")
}

// TestParseOwnerRepo covers ExtractGitURI-shaped inputs (the only inputs ParseOwnerRepo is
// actually called with in production) as well as non-GitHub hosts and malformed values.
func TestParseOwnerRepo(t *testing.T) {
	tests := []struct {
		name      string
		gitURI    string
		wantOwner string
		wantRepo  string
		wantOK    bool
	}{
		{
			name:      "github https URI with .git suffix",
			gitURI:    "https://github.com/cloudposse/terraform-aws-vpc.git",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-aws-vpc",
			wantOK:    true,
		},
		{
			name:      "github https URI without .git suffix",
			gitURI:    "https://github.com/cloudposse/terraform-aws-vpc",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-aws-vpc",
			wantOK:    true,
		},
		{
			name:   "gitlab host is not a GitHub source",
			gitURI: "https://gitlab.com/cloudposse/terraform-aws-vpc.git",
			wantOK: false,
		},
		{
			name:   "bitbucket host is not a GitHub source",
			gitURI: "https://bitbucket.org/cloudposse/terraform-aws-vpc.git",
			wantOK: false,
		},
		{
			name:   "owner-only path has no repo",
			gitURI: "https://github.com/cloudposse",
			wantOK: false,
		},
		{
			name:   "empty string",
			gitURI: "",
			wantOK: false,
		},
		{
			name:   "malformed URL",
			gitURI: "://not a url",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, ok := ParseOwnerRepo(tt.gitURI)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.wantOwner, owner)
				assert.Equal(t, tt.wantRepo, repo)
			}
		})
	}
}
