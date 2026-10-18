package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-github/v59/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// treeFixture serves the contents and commit endpoints TreeSHA reads.
func treeFixture(t *testing.T) *github.Client {
	t.Helper()
	mux := http.NewServeMux()
	listing := func(w http.ResponseWriter, r *http.Request, entries ...*github.RepositoryContent) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("ref") != "1.0.0" {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "No commit found for the ref"})
			return
		}
		_ = json.NewEncoder(w).Encode(entries)
	}
	dir := func(name, sha string) *github.RepositoryContent {
		return &github.RepositoryContent{Name: github.String(name), Type: github.String("dir"), SHA: github.String(sha)}
	}
	mux.HandleFunc("/repos/o/mono/contents/modules/eks", func(w http.ResponseWriter, r *http.Request) {
		listing(w, r, dir("cluster", "aaa111"), &github.RepositoryContent{Name: github.String("README"), Type: github.String("file"), SHA: github.String("fff")})
	})
	mux.HandleFunc("/repos/o/new/contents/", func(w http.ResponseWriter, r *http.Request) {
		listing(w, r, dir("src", "bbb222"))
	})
	mux.HandleFunc("/repos/o/limited/contents/modules", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	// Commit lookup distinguishes a missing ref from a missing directory.
	mux.HandleFunc("/repos/o/mono/commits/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/o/mono/commits/1.0.0" {
			w.Header().Set("Content-Type", "application/vnd.github.sha")
			_, _ = w.Write([]byte("deadbeef"))
			return
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return newTestClient(t, server.URL)
}

func TestTreeSHA(t *testing.T) {
	client := treeFixture(t)
	for _, tt := range []struct {
		name    string
		repo    string
		ref     string
		path    string
		want    string
		wantErr error
	}{
		{"nested module directory", "mono", "1.0.0", "modules/eks/cluster", "aaa111", nil},
		{"directory in the repository root", "new", "1.0.0", "src", "bbb222", nil},
		{"directory missing from an existing listing", "mono", "1.0.0", "modules/eks/missing", "", ErrPathNotFound},
		{"entry that is a file", "mono", "1.0.0", "modules/eks/README", "", ErrPathNotFound},
		{"missing ref on the parent listing", "mono", "9.9.9", "modules/eks/cluster", "", ErrRefNotFound},
		{"missing parent directory at an existing ref", "mono", "1.0.0", "modules/gone/cluster", "", ErrPathNotFound},
		{"no directory path", "mono", "1.0.0", "", "", ErrPathNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := treeSHA(context.Background(), client, &treeQuery{owner: "o", repo: tt.repo, ref: tt.ref, dirPath: tt.path})
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	t.Run("other failures are returned unclassified", func(t *testing.T) {
		_, err := treeSHA(context.Background(), client, &treeQuery{owner: "o", repo: "limited", ref: "1.0.0", dirPath: "modules/x"})
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrPathNotFound)
		assert.NotErrorIs(t, err, ErrRefNotFound)
	})
}

func TestTreeSHADoesNotInferAbsenceFromLimitedListing(t *testing.T) {
	for _, count := range []int{contentsDirectoryLimit - 1, contentsDirectoryLimit} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			entries := make([]*github.RepositoryContent, count)
			for i := range entries {
				entries[i] = &github.RepositoryContent{Name: github.String(fmt.Sprintf("dir-%d", i)), Type: github.String("dir"), SHA: github.String("present-sha")}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				require.NoError(t, json.NewEncoder(w).Encode(entries))
			}))
			t.Cleanup(server.Close)
			client := newTestClient(t, server.URL)
			_, err := treeSHA(context.Background(), client, &treeQuery{owner: "o", repo: "r", ref: "v1", dirPath: "missing"})
			if count == contentsDirectoryLimit {
				require.ErrorIs(t, err, ErrTreeListingIncomplete)
				require.NotErrorIs(t, err, ErrPathNotFound)
			} else {
				require.ErrorIs(t, err, ErrPathNotFound)
			}
			sha, err := treeSHA(context.Background(), client, &treeQuery{owner: "o", repo: "r", ref: "v1", dirPath: "dir-0"})
			require.NoError(t, err)
			require.Equal(t, "present-sha", sha)
		})
	}
}

// publicTreeServer serves a directory listing and records whether a request carried credentials.
func publicTreeServer(t *testing.T) (url string, hits *int, authorized *bool) {
	t.Helper()
	hits, authorized = new(int), new(bool)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hits++
		if r.Header.Get("Authorization") != "" {
			*authorized = true
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]*github.RepositoryContent{{Name: github.String("src"), Type: github.String("dir"), SHA: github.String("bbb222")}})
	}))
	t.Cleanup(server.Close)
	return server.URL, hits, authorized
}

// TestTreeSHAUsesTheConfiguredGitHubAPI covers the public entry point against public github.com.
func TestTreeSHAUsesTheConfiguredGitHubAPI(t *testing.T) {
	url, hits, _ := publicTreeServer(t)
	t.Setenv("GITHUB_SERVER_URL", "https://github.com")
	t.Setenv("GITHUB_API_URL", url)

	sha, err := TreeSHA(context.Background(), "o", "new", "1.0.0", "src")

	require.NoError(t, err)
	assert.Equal(t, "bbb222", sha)
	assert.Equal(t, 1, *hits)
}

// TestTreeSHAQueriesPublicGitHubWithoutTheEnterpriseToken covers a GitHub Enterprise Server
// configuration. The compared repositories are public, so the lookup goes to public github.com
// and never carries the token scoped to the enterprise server.
func TestTreeSHAQueriesPublicGitHubWithoutTheEnterpriseToken(t *testing.T) {
	publicURL, publicHits, authorized := publicTreeServer(t)
	enterpriseURL, enterpriseHits, _ := publicTreeServer(t)
	original := publicGitHubEndpoints
	publicGitHubEndpoints = newEndpoints(defaultGitHubServerURL, publicURL)
	t.Cleanup(func() { publicGitHubEndpoints = original })
	t.Setenv("GITHUB_SERVER_URL", "https://ghes.example.com")
	t.Setenv("GITHUB_API_URL", enterpriseURL)
	t.Setenv("GITHUB_TOKEN", "enterprise-token")

	sha, err := TreeSHA(context.Background(), "o", "new", "1.0.0", "src")

	require.NoError(t, err)
	assert.Equal(t, "bbb222", sha)
	assert.Equal(t, 1, *publicHits, "the lookup targets public github.com")
	assert.Zero(t, *enterpriseHits, "the enterprise server is never queried")
	assert.False(t, *authorized, "the enterprise token is never sent to public github.com")
}
