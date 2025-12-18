package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/go-github/v59/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci"
)

func TestMapGitHubStatusToCheckRunState(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected ci.CheckRunState
	}{
		{
			name:     "queued",
			status:   "queued",
			expected: ci.CheckRunStatePending,
		},
		{
			name:     "in_progress",
			status:   "in_progress",
			expected: ci.CheckRunStateInProgress,
		},
		{
			name:     "completed",
			status:   "completed",
			expected: ci.CheckRunStateSuccess, // Fallback for completed.
		},
		{
			name:     "unknown status",
			status:   "unknown",
			expected: ci.CheckRunStatePending,
		},
		{
			name:     "empty status",
			status:   "",
			expected: ci.CheckRunStatePending,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapGitHubStatusToCheckRunState(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMapCheckRunStateToConclusion(t *testing.T) {
	tests := []struct {
		name               string
		state              ci.CheckRunState
		providedConclusion string
		expected           string
	}{
		{
			name:               "success state",
			state:              ci.CheckRunStateSuccess,
			providedConclusion: "",
			expected:           "success",
		},
		{
			name:               "failure state",
			state:              ci.CheckRunStateFailure,
			providedConclusion: "",
			expected:           "failure",
		},
		{
			name:               "error state",
			state:              ci.CheckRunStateError,
			providedConclusion: "",
			expected:           "failure",
		},
		{
			name:               "cancelled state",
			state:              ci.CheckRunStateCancelled,
			providedConclusion: "",
			expected:           "cancelled",
		},
		{
			name:               "pending state",
			state:              ci.CheckRunStatePending,
			providedConclusion: "",
			expected:           "neutral",
		},
		{
			name:               "provided conclusion overrides",
			state:              ci.CheckRunStateFailure,
			providedConclusion: "timed_out",
			expected:           "timed_out",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapCheckRunStateToConclusion(tt.state, tt.providedConclusion)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetCheckRunOutputTitle(t *testing.T) {
	t.Run("with output", func(t *testing.T) {
		cr := &github.CheckRun{
			Output: &github.CheckRunOutput{
				Title: github.String("Test Title"),
			},
		}
		assert.Equal(t, "Test Title", getCheckRunOutputTitle(cr))
	})

	t.Run("nil output", func(t *testing.T) {
		cr := &github.CheckRun{}
		assert.Equal(t, "", getCheckRunOutputTitle(cr))
	})

	t.Run("nil title", func(t *testing.T) {
		cr := &github.CheckRun{
			Output: &github.CheckRunOutput{},
		}
		assert.Equal(t, "", getCheckRunOutputTitle(cr))
	})
}

func TestGetCheckRunOutputSummary(t *testing.T) {
	t.Run("with output", func(t *testing.T) {
		cr := &github.CheckRun{
			Output: &github.CheckRunOutput{
				Summary: github.String("Test Summary"),
			},
		}
		assert.Equal(t, "Test Summary", getCheckRunOutputSummary(cr))
	})

	t.Run("nil output", func(t *testing.T) {
		cr := &github.CheckRun{}
		assert.Equal(t, "", getCheckRunOutputSummary(cr))
	})

	t.Run("nil summary", func(t *testing.T) {
		cr := &github.CheckRun{
			Output: &github.CheckRunOutput{},
		}
		assert.Equal(t, "", getCheckRunOutputSummary(cr))
	})
}

func TestProvider_CreateCheckRun(t *testing.T) {
	t.Run("create pending check run", func(t *testing.T) {
		var capturedRequest map[string]any
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/owner/repo/check-runs", func(w http.ResponseWriter, r *http.Request) {
			json.NewDecoder(r.Body).Decode(&capturedRequest)
			w.Header().Set("Content-Type", "application/json")
			response := map[string]any{
				"id":         12345,
				"name":       "terraform-plan",
				"status":     "queued",
				"started_at": time.Now().Format(time.RFC3339),
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
		checkRun, err := provider.CreateCheckRun(ctx, &ci.CreateCheckRunOptions{
			Owner:  "owner",
			Repo:   "repo",
			SHA:    "abc123",
			Name:   "terraform-plan",
			Status: ci.CheckRunStatePending,
		})
		require.NoError(t, err)
		require.NotNil(t, checkRun)
		assert.Equal(t, int64(12345), checkRun.ID)
		assert.Equal(t, "terraform-plan", checkRun.Name)
		assert.Equal(t, ci.CheckRunStatePending, checkRun.Status)
	})

	t.Run("create check run with output", func(t *testing.T) {
		var capturedRequest map[string]any
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/owner/repo/check-runs", func(w http.ResponseWriter, r *http.Request) {
			json.NewDecoder(r.Body).Decode(&capturedRequest)
			w.Header().Set("Content-Type", "application/json")
			response := map[string]any{
				"id":     12345,
				"name":   "terraform-plan",
				"status": "queued",
				"output": map[string]any{
					"title":   "Terraform Plan",
					"summary": "Plan summary here",
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
		checkRun, err := provider.CreateCheckRun(ctx, &ci.CreateCheckRunOptions{
			Owner:   "owner",
			Repo:    "repo",
			SHA:     "abc123",
			Name:    "terraform-plan",
			Status:  ci.CheckRunStatePending,
			Title:   "Terraform Plan",
			Summary: "Plan summary here",
		})
		require.NoError(t, err)
		require.NotNil(t, checkRun)

		// Verify output was included in request.
		output, ok := capturedRequest["output"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Terraform Plan", output["title"])
		assert.Equal(t, "Plan summary here", output["summary"])
	})
}

func TestProvider_UpdateCheckRun(t *testing.T) {
	t.Run("update to completed success", func(t *testing.T) {
		var capturedRequest map[string]any
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/owner/repo/check-runs/12345", func(w http.ResponseWriter, r *http.Request) {
			json.NewDecoder(r.Body).Decode(&capturedRequest)
			w.Header().Set("Content-Type", "application/json")
			now := time.Now()
			response := map[string]any{
				"id":           12345,
				"name":         "terraform-plan",
				"status":       "completed",
				"conclusion":   "success",
				"completed_at": now.Format(time.RFC3339),
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
		now := time.Now()
		checkRun, err := provider.UpdateCheckRun(ctx, &ci.UpdateCheckRunOptions{
			Owner:       "owner",
			Repo:        "repo",
			CheckRunID:  12345,
			Title:       "terraform-plan",
			Status:      ci.CheckRunStateSuccess,
			CompletedAt: &now,
		})
		require.NoError(t, err)
		require.NotNil(t, checkRun)
		assert.Equal(t, "success", checkRun.Conclusion)
	})

	t.Run("update to completed failure", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/owner/repo/check-runs/12345", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			response := map[string]any{
				"id":         12345,
				"name":       "terraform-plan",
				"status":     "completed",
				"conclusion": "failure",
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
		checkRun, err := provider.UpdateCheckRun(ctx, &ci.UpdateCheckRunOptions{
			Owner:      "owner",
			Repo:       "repo",
			CheckRunID: 12345,
			Title:      "terraform-plan",
			Status:     ci.CheckRunStateFailure,
			Summary:    "Plan failed with errors",
		})
		require.NoError(t, err)
		require.NotNil(t, checkRun)
		assert.Equal(t, "failure", checkRun.Conclusion)
	})

	t.Run("update to in progress", func(t *testing.T) {
		var capturedRequest map[string]any
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/owner/repo/check-runs/12345", func(w http.ResponseWriter, r *http.Request) {
			json.NewDecoder(r.Body).Decode(&capturedRequest)
			w.Header().Set("Content-Type", "application/json")
			response := map[string]any{
				"id":     12345,
				"name":   "terraform-plan",
				"status": "in_progress",
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
		checkRun, err := provider.UpdateCheckRun(ctx, &ci.UpdateCheckRunOptions{
			Owner:      "owner",
			Repo:       "repo",
			CheckRunID: 12345,
			Title:      "terraform-plan",
			Status:     ci.CheckRunStateInProgress,
		})
		require.NoError(t, err)
		require.NotNil(t, checkRun)
		assert.Equal(t, "in_progress", capturedRequest["status"])
	})
}
