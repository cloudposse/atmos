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

	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
)

func TestMapGitHubStatusToCheckRunState(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected provider.CheckRunState
	}{
		{
			name:     "queued",
			status:   "queued",
			expected: provider.CheckRunStatePending,
		},
		{
			name:     "in_progress",
			status:   "in_progress",
			expected: provider.CheckRunStateInProgress,
		},
		{
			name:     "completed",
			status:   "completed",
			expected: provider.CheckRunStateSuccess, // Fallback for completed.
		},
		{
			name:     "unknown status",
			status:   "unknown",
			expected: provider.CheckRunStatePending,
		},
		{
			name:     "empty status",
			status:   "",
			expected: provider.CheckRunStatePending,
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
		state              provider.CheckRunState
		providedConclusion string
		expected           string
	}{
		{
			name:               "success state",
			state:              provider.CheckRunStateSuccess,
			providedConclusion: "",
			expected:           "success",
		},
		{
			name:               "failure state",
			state:              provider.CheckRunStateFailure,
			providedConclusion: "",
			expected:           "failure",
		},
		{
			name:               "error state",
			state:              provider.CheckRunStateError,
			providedConclusion: "",
			expected:           "failure",
		},
		{
			name:               "cancelled state",
			state:              provider.CheckRunStateCancelled,
			providedConclusion: "",
			expected:           "cancelled",
		},
		{
			name:               "pending state",
			state:              provider.CheckRunStatePending,
			providedConclusion: "",
			expected:           "neutral",
		},
		{
			name:               "provided conclusion overrides",
			state:              provider.CheckRunStateFailure,
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
			if err := json.NewDecoder(r.Body).Decode(&capturedRequest); err != nil {
				t.Errorf("failed to decode request: %v", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
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

		serverURL, err := url.Parse(server.URL + "/")
		require.NoError(t, err)
		ghClient := github.NewClient(nil)
		ghClient.BaseURL = serverURL

		client := &Client{client: ghClient}
		p := NewProviderWithClient(client)

		ctx := context.Background()
		checkRun, err := p.CreateCheckRun(ctx, &provider.CreateCheckRunOptions{
			Owner:  "owner",
			Repo:   "repo",
			SHA:    "abc123",
			Name:   "terraform-plan",
			Status: provider.CheckRunStatePending,
		})
		require.NoError(t, err)
		require.NotNil(t, checkRun)
		assert.Equal(t, int64(12345), checkRun.ID)
		assert.Equal(t, "terraform-plan", checkRun.Name)
		assert.Equal(t, provider.CheckRunStatePending, checkRun.Status)
	})

	t.Run("create check run with output", func(t *testing.T) {
		var capturedRequest map[string]any
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/owner/repo/check-runs", func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewDecoder(r.Body).Decode(&capturedRequest); err != nil {
				t.Errorf("failed to decode request: %v", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
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

		serverURL, err := url.Parse(server.URL + "/")
		require.NoError(t, err)
		ghClient := github.NewClient(nil)
		ghClient.BaseURL = serverURL

		client := &Client{client: ghClient}
		p := NewProviderWithClient(client)

		ctx := context.Background()
		checkRun, err := p.CreateCheckRun(ctx, &provider.CreateCheckRunOptions{
			Owner:   "owner",
			Repo:    "repo",
			SHA:     "abc123",
			Name:    "terraform-plan",
			Status:  provider.CheckRunStatePending,
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
	t.Run("create then update lifecycle", func(t *testing.T) {
		// Test the full lifecycle: CreateCheckRun stores ID internally,
		// then UpdateCheckRun resolves it by name.
		mux := http.NewServeMux()

		// CreateCheckRun endpoint.
		mux.HandleFunc("/repos/owner/repo/check-runs", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			response := map[string]any{
				"id":         12345,
				"name":       "terraform-plan",
				"status":     "in_progress",
				"started_at": time.Now().Format(time.RFC3339),
			}
			json.NewEncoder(w).Encode(response)
		})

		// UpdateCheckRun endpoint — expects ID 12345 from internal map.
		mux.HandleFunc("/repos/owner/repo/check-runs/12345", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPatch {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
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

		serverURL, err := url.Parse(server.URL + "/")
		require.NoError(t, err)
		ghClient := github.NewClient(nil)
		ghClient.BaseURL = serverURL

		client := &Client{client: ghClient}
		p := NewProviderWithClient(client)

		ctx := context.Background()

		// Step 1: Create check run — ID is stored internally.
		created, err := p.CreateCheckRun(ctx, &provider.CreateCheckRunOptions{
			Owner:  "owner",
			Repo:   "repo",
			SHA:    "abc123",
			Name:   "terraform-plan",
			Status: provider.CheckRunStateInProgress,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(12345), created.ID)

		// Step 2: Update check run — ID is resolved by name.
		now := time.Now()
		updated, err := p.UpdateCheckRun(ctx, &provider.UpdateCheckRunOptions{
			Owner:       "owner",
			Repo:        "repo",
			Name:        "terraform-plan",
			Status:      provider.CheckRunStateSuccess,
			CompletedAt: &now,
		})
		require.NoError(t, err)
		require.NotNil(t, updated)
		assert.Equal(t, int64(12345), updated.ID)
		assert.Equal(t, "success", updated.Conclusion)
	})

	t.Run("update without prior create falls back to create", func(t *testing.T) {
		// When UpdateCheckRun is called without a prior CreateCheckRun,
		// it should fall back to creating a new completed check run.
		mux := http.NewServeMux()

		var createCalled bool
		mux.HandleFunc("/repos/owner/repo/check-runs", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			createCalled = true
			w.Header().Set("Content-Type", "application/json")
			response := map[string]any{
				"id":         99999,
				"name":       "terraform-plan",
				"status":     "completed",
				"conclusion": "success",
			}
			json.NewEncoder(w).Encode(response)
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		serverURL, err := url.Parse(server.URL + "/")
		require.NoError(t, err)
		ghClient := github.NewClient(nil)
		ghClient.BaseURL = serverURL

		client := &Client{client: ghClient}
		p := NewProviderWithClient(client)

		ctx := context.Background()

		// Update without prior create — should fall back to create.
		checkRun, err := p.UpdateCheckRun(ctx, &provider.UpdateCheckRunOptions{
			Owner:   "owner",
			Repo:    "repo",
			SHA:     "abc123",
			Name:    "terraform-plan",
			Status:  provider.CheckRunStateSuccess,
			Title:   "Plan succeeded",
			Summary: "No changes",
		})
		require.NoError(t, err)
		require.NotNil(t, checkRun)
		assert.True(t, createCalled, "should have called CreateCheckRun as fallback")
		assert.Equal(t, int64(99999), checkRun.ID)
	})
}
