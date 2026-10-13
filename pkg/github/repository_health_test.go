package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRepositoryHealthPreservesUncertainty(t *testing.T) {
	for _, tt := range []struct {
		name         string
		code         int
		body, status string
	}{
		{"active", 200, `{"archived":false}`, "healthy"},
		{"archived", 200, `{"archived":true}`, "archived"},
		{"private or missing", 404, `{}`, "unavailable"},
		{"gone", 410, `{}`, "missing"},
		{"authentication", 401, `{}`, "inconclusive"},
		{"rate limit", 403, `{}`, "inconclusive"},
		{"server failure", 503, `{}`, "inconclusive"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/repos/owner/repo", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.code)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			result := inspectRepository(context.Background(), newTestClient(t, server.URL), "owner", "repo")
			require.Equal(t, tt.status, result.Status)
		})
	}
}

// TestInspectRepositoryUsesConfiguredEnterpriseEndpoints exercises the exported entry point
// against a local server standing in for a GitHub Enterprise Server host.
func TestInspectRepositoryUsesConfiguredEnterpriseEndpoints(t *testing.T) {
	for _, tt := range []struct {
		name   string
		code   int
		body   string
		status string
	}{
		{"active", http.StatusOK, `{"archived":false}`, "healthy"},
		{"archived", http.StatusOK, `{"archived":true}`, "archived"},
		{"private or missing", http.StatusNotFound, `{}`, "unavailable"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			clearGitHubEndpointEnv(t)
			var requested string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requested = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.code)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			t.Setenv("GITHUB_SERVER_URL", server.URL)
			t.Setenv("GITHUB_API_URL", server.URL+"/api/v3")
			// A fake token keeps the lookup from shelling out to the gh CLI.
			t.Setenv("GITHUB_TOKEN", "test-token")

			result := InspectRepository(context.Background(), server.URL+"/owner/repo.git")

			require.Equal(t, tt.status, result.Status)
			require.Equal(t, "/api/v3/repos/owner/repo", requested)
		})
	}
}

func TestInspectRepositoryRejectsUnsupportedHosts(t *testing.T) {
	clearGitHubEndpointEnv(t)
	for _, uri := range []string{
		"https://gitlab.com/owner/repo.git",
		"https://github.com/owner",
	} {
		t.Run(uri, func(t *testing.T) {
			result := InspectRepository(context.Background(), uri)
			require.Equal(t, "unsupported", result.Status)
		})
	}
}
