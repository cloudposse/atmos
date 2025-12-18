package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-github/v59/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci"
)

func TestAllChecksPassed(t *testing.T) {
	tests := []struct {
		name     string
		checks   []*ci.CheckStatus
		expected bool
	}{
		{
			name:     "empty checks",
			checks:   []*ci.CheckStatus{},
			expected: true,
		},
		{
			name: "all success",
			checks: []*ci.CheckStatus{
				{Name: "test1", Status: "completed", Conclusion: "success"},
				{Name: "test2", Status: "completed", Conclusion: "success"},
			},
			expected: true,
		},
		{
			name: "all skipped",
			checks: []*ci.CheckStatus{
				{Name: "test1", Status: "completed", Conclusion: "skipped"},
			},
			expected: true,
		},
		{
			name: "mixed success and skipped",
			checks: []*ci.CheckStatus{
				{Name: "test1", Status: "completed", Conclusion: "success"},
				{Name: "test2", Status: "completed", Conclusion: "skipped"},
			},
			expected: true,
		},
		{
			name: "one failure",
			checks: []*ci.CheckStatus{
				{Name: "test1", Status: "completed", Conclusion: "success"},
				{Name: "test2", Status: "completed", Conclusion: "failure"},
			},
			expected: false,
		},
		{
			name: "one pending",
			checks: []*ci.CheckStatus{
				{Name: "test1", Status: "completed", Conclusion: "success"},
				{Name: "test2", Status: "in_progress", Conclusion: ""},
			},
			expected: false,
		},
		{
			name: "cancelled",
			checks: []*ci.CheckStatus{
				{Name: "test1", Status: "completed", Conclusion: "cancelled"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := allChecksPassed(tt.checks)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProvider_GetStatus(t *testing.T) {
	t.Run("basic status fetch", func(t *testing.T) {
		mux := http.NewServeMux()

		// Mock check runs endpoint.
		mux.HandleFunc("/repos/owner/repo/commits/abc123/check-runs", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			response := map[string]any{
				"total_count": 1,
				"check_runs": []map[string]any{
					{
						"id":          1,
						"name":        "build",
						"status":      "completed",
						"conclusion":  "success",
						"details_url": "https://github.com/owner/repo/actions/runs/1",
					},
				},
			}
			json.NewEncoder(w).Encode(response)
		})

		// Mock combined status endpoint.
		mux.HandleFunc("/repos/owner/repo/commits/abc123/status", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			response := map[string]any{
				"state":    "success",
				"statuses": []map[string]any{},
			}
			json.NewEncoder(w).Encode(response)
		})

		// Mock PRs list endpoint.
		mux.HandleFunc("/repos/owner/repo/pulls", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		serverURL, _ := url.Parse(server.URL + "/")
		ghClient := github.NewClient(nil)
		ghClient.BaseURL = serverURL

		client := &Client{client: ghClient}
		provider := NewProviderWithClient(client)

		ctx := context.Background()
		status, err := provider.GetStatus(ctx, ci.StatusOptions{
			Owner:  "owner",
			Repo:   "repo",
			Branch: "main",
			SHA:    "abc123",
		})
		require.NoError(t, err)
		require.NotNil(t, status)
		assert.Equal(t, "owner/repo", status.Repository)
		require.NotNil(t, status.CurrentBranch)
		assert.Equal(t, "main", status.CurrentBranch.Branch)
		assert.Len(t, status.CurrentBranch.Checks, 1)
	})
}

func TestProvider_GetBranchStatus(t *testing.T) {
	t.Run("branch with PR", func(t *testing.T) {
		mux := http.NewServeMux()

		// Mock PRs list endpoint - returns a PR.
		mux.HandleFunc("/repos/owner/repo/pulls", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			response := []map[string]any{
				{
					"number":   42,
					"title":    "Test PR",
					"html_url": "https://github.com/owner/repo/pull/42",
					"head": map[string]any{
						"ref": "feature",
						"sha": "def456",
					},
					"base": map[string]any{
						"ref": "main",
					},
				},
			}
			json.NewEncoder(w).Encode(response)
		})

		// Mock check runs for branch SHA.
		mux.HandleFunc("/repos/owner/repo/commits/abc123/check-runs", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			response := map[string]any{
				"total_count": 0,
				"check_runs":  []map[string]any{},
			}
			json.NewEncoder(w).Encode(response)
		})

		// Mock check runs for PR head SHA.
		mux.HandleFunc("/repos/owner/repo/commits/def456/check-runs", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			response := map[string]any{
				"total_count": 1,
				"check_runs": []map[string]any{
					{
						"id":         1,
						"name":       "CI",
						"status":     "completed",
						"conclusion": "success",
					},
				},
			}
			json.NewEncoder(w).Encode(response)
		})

		// Mock combined status.
		mux.HandleFunc("/repos/owner/repo/commits/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			response := map[string]any{
				"state":    "success",
				"statuses": []map[string]any{},
			}
			json.NewEncoder(w).Encode(response)
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		serverURL, _ := url.Parse(server.URL + "/")
		ghClient := github.NewClient(nil)
		ghClient.BaseURL = serverURL

		client := &Client{client: ghClient}
		provider := NewProviderWithClient(client)

		ctx := context.Background()
		status, err := provider.getBranchStatus(ctx, "owner", "repo", "feature", "abc123")
		require.NoError(t, err)
		require.NotNil(t, status)
		assert.Equal(t, "feature", status.Branch)
		require.NotNil(t, status.PullRequest)
		assert.Equal(t, 42, status.PullRequest.Number)
		assert.Equal(t, "Test PR", status.PullRequest.Title)
	})

	t.Run("branch without PR", func(t *testing.T) {
		mux := http.NewServeMux()

		// Mock PRs list endpoint - returns empty.
		mux.HandleFunc("/repos/owner/repo/pulls", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
		})

		// Mock check runs.
		mux.HandleFunc("/repos/owner/repo/commits/abc123/check-runs", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			response := map[string]any{
				"total_count": 0,
				"check_runs":  []map[string]any{},
			}
			json.NewEncoder(w).Encode(response)
		})

		// Mock combined status.
		mux.HandleFunc("/repos/owner/repo/commits/abc123/status", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			response := map[string]any{
				"state":    "success",
				"statuses": []map[string]any{},
			}
			json.NewEncoder(w).Encode(response)
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		serverURL, _ := url.Parse(server.URL + "/")
		ghClient := github.NewClient(nil)
		ghClient.BaseURL = serverURL

		client := &Client{client: ghClient}
		provider := NewProviderWithClient(client)

		ctx := context.Background()
		status, err := provider.getBranchStatus(ctx, "owner", "repo", "main", "abc123")
		require.NoError(t, err)
		require.NotNil(t, status)
		assert.Nil(t, status.PullRequest)
	})
}

func TestProvider_GetCheckRuns(t *testing.T) {
	t.Run("with check runs", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/owner/repo/commits/abc123/check-runs", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			response := map[string]any{
				"total_count": 2,
				"check_runs": []map[string]any{
					{
						"id":          1,
						"name":        "build",
						"status":      "completed",
						"conclusion":  "success",
						"details_url": "https://example.com/1",
					},
					{
						"id":          2,
						"name":        "test",
						"status":      "in_progress",
						"conclusion":  "",
						"details_url": "https://example.com/2",
					},
				},
			}
			json.NewEncoder(w).Encode(response)
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		serverURL, _ := url.Parse(server.URL + "/")
		ghClient := github.NewClient(nil)
		ghClient.BaseURL = serverURL

		client := &Client{client: ghClient}
		provider := NewProviderWithClient(client)

		ctx := context.Background()
		checks, err := provider.getCheckRuns(ctx, "owner", "repo", "abc123")
		require.NoError(t, err)
		require.Len(t, checks, 2)

		assert.Equal(t, "build", checks[0].Name)
		assert.Equal(t, "completed", checks[0].Status)
		assert.Equal(t, "success", checks[0].Conclusion)
		assert.Equal(t, "https://example.com/1", checks[0].DetailsURL)

		assert.Equal(t, "test", checks[1].Name)
		assert.Equal(t, "in_progress", checks[1].Status)
	})

	t.Run("empty check runs", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/owner/repo/commits/abc123/check-runs", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			response := map[string]any{
				"total_count": 0,
				"check_runs":  []map[string]any{},
			}
			json.NewEncoder(w).Encode(response)
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		serverURL, _ := url.Parse(server.URL + "/")
		ghClient := github.NewClient(nil)
		ghClient.BaseURL = serverURL

		client := &Client{client: ghClient}
		provider := NewProviderWithClient(client)

		ctx := context.Background()
		checks, err := provider.getCheckRuns(ctx, "owner", "repo", "abc123")
		require.NoError(t, err)
		assert.Empty(t, checks)
	})
}

func TestProvider_GetCombinedStatus(t *testing.T) {
	t.Run("with statuses", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/owner/repo/commits/abc123/status", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			response := map[string]any{
				"state": "success",
				"statuses": []map[string]any{
					{
						"context":    "ci/travis",
						"state":      "success",
						"target_url": "https://travis-ci.org/build/1",
					},
					{
						"context":    "coverage/codecov",
						"state":      "pending",
						"target_url": "https://codecov.io/build/1",
					},
				},
			}
			json.NewEncoder(w).Encode(response)
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		serverURL, _ := url.Parse(server.URL + "/")
		ghClient := github.NewClient(nil)
		ghClient.BaseURL = serverURL

		client := &Client{client: ghClient}
		provider := NewProviderWithClient(client)

		ctx := context.Background()
		checks, err := provider.getCombinedStatus(ctx, "owner", "repo", "abc123")
		require.NoError(t, err)
		require.Len(t, checks, 2)

		assert.Equal(t, "ci/travis", checks[0].Name)
		assert.Equal(t, "completed", checks[0].Status) // Legacy status is always completed.
		assert.Equal(t, "success", checks[0].Conclusion)

		assert.Equal(t, "coverage/codecov", checks[1].Name)
		assert.Equal(t, "pending", checks[1].Conclusion)
	})
}

func TestProvider_GetPRForBranch(t *testing.T) {
	t.Run("PR exists", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/owner/repo/pulls", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			response := []map[string]any{
				{
					"number":   123,
					"title":    "Feature PR",
					"html_url": "https://github.com/owner/repo/pull/123",
					"head": map[string]any{
						"ref": "feature-branch",
						"sha": "headsha123",
					},
					"base": map[string]any{
						"ref": "main",
					},
				},
			}
			json.NewEncoder(w).Encode(response)
		})

		// Mock check runs for PR head.
		mux.HandleFunc("/repos/owner/repo/commits/headsha123/check-runs", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			response := map[string]any{
				"total_count": 1,
				"check_runs": []map[string]any{
					{
						"id":         1,
						"name":       "CI",
						"status":     "completed",
						"conclusion": "success",
					},
				},
			}
			json.NewEncoder(w).Encode(response)
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		serverURL, _ := url.Parse(server.URL + "/")
		ghClient := github.NewClient(nil)
		ghClient.BaseURL = serverURL

		client := &Client{client: ghClient}
		provider := NewProviderWithClient(client)

		ctx := context.Background()
		pr, err := provider.getPRForBranch(ctx, "owner", "repo", "feature-branch")
		require.NoError(t, err)
		require.NotNil(t, pr)
		assert.Equal(t, 123, pr.Number)
		assert.Equal(t, "Feature PR", pr.Title)
		assert.Equal(t, "feature-branch", pr.Branch)
		assert.Equal(t, "main", pr.BaseBranch)
		assert.True(t, pr.AllPassed)
	})

	t.Run("no PR exists", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/owner/repo/pulls", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		serverURL, _ := url.Parse(server.URL + "/")
		ghClient := github.NewClient(nil)
		ghClient.BaseURL = serverURL

		client := &Client{client: ghClient}
		provider := NewProviderWithClient(client)

		ctx := context.Background()
		pr, err := provider.getPRForBranch(ctx, "owner", "repo", "orphan-branch")
		require.NoError(t, err)
		assert.Nil(t, pr)
	})
}
