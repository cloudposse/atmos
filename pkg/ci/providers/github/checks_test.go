package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/google/go-github/v59/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
)

func TestMapCheckRunStateToStatusState(t *testing.T) {
	tests := []struct {
		name     string
		state    provider.CheckRunState
		expected string
	}{
		{"pending", provider.CheckRunStatePending, "pending"},
		{"in_progress maps to pending", provider.CheckRunStateInProgress, "pending"},
		{"success", provider.CheckRunStateSuccess, "success"},
		{"failure", provider.CheckRunStateFailure, "failure"},
		{"error", provider.CheckRunStateError, "error"},
		{"cancelled maps to error", provider.CheckRunStateCancelled, "error"},
		{"unknown defaults to pending", provider.CheckRunState("unknown"), "pending"},
		{"empty defaults to pending", provider.CheckRunState(""), "pending"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapCheckRunStateToStatusState(tt.state)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTruncateDescription(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "short string unchanged",
			input:    "3 to add, 1 to change",
			expected: "3 to add, 1 to change",
		},
		{
			name:     "exactly 140 chars unchanged",
			input:    strings.Repeat("a", 140),
			expected: strings.Repeat("a", 140),
		},
		{
			name:     "141 chars truncated",
			input:    strings.Repeat("a", 141),
			expected: strings.Repeat("a", 137) + "...",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "unicode characters counted correctly",
			input:    strings.Repeat("日", 141),
			expected: strings.Repeat("日", 137) + "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateDescription(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProvider_CreateCheckRun(t *testing.T) {
	t.Run("creates commit status with pending state", func(t *testing.T) {
		var capturedRequest map[string]any
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/owner/repo/statuses/abc123", func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewDecoder(r.Body).Decode(&capturedRequest); err != nil {
				t.Errorf("failed to decode request: %v", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			response := map[string]any{
				"id":          12345,
				"context":     capturedRequest["context"],
				"state":       capturedRequest["state"],
				"description": capturedRequest["description"],
			}
			json.NewEncoder(w).Encode(response) //nolint:errcheck
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
			Name:   "atmos/plan/dev/vpc",
			Status: provider.CheckRunStatePending,
			Title:  "Plan in progress...",
		})
		require.NoError(t, err)
		require.NotNil(t, checkRun)
		assert.Equal(t, int64(12345), checkRun.ID)
		assert.Equal(t, "atmos/plan/dev/vpc", checkRun.Name)
		assert.Equal(t, provider.CheckRunStatePending, checkRun.Status)

		// Verify request body.
		assert.Equal(t, "pending", capturedRequest["state"])
		assert.Equal(t, "atmos/plan/dev/vpc", capturedRequest["context"])
		assert.Equal(t, "Plan in progress...", capturedRequest["description"])
	})

	t.Run("creates commit status with target_url", func(t *testing.T) {
		var capturedRequest map[string]any
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/owner/repo/statuses/abc123", func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewDecoder(r.Body).Decode(&capturedRequest); err != nil {
				t.Errorf("failed to decode request: %v", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			response := map[string]any{
				"id":          12345,
				"context":     capturedRequest["context"],
				"state":       capturedRequest["state"],
				"description": capturedRequest["description"],
				"target_url":  capturedRequest["target_url"],
			}
			json.NewEncoder(w).Encode(response) //nolint:errcheck
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
		_, err = p.CreateCheckRun(ctx, &provider.CreateCheckRunOptions{
			Owner:      "owner",
			Repo:       "repo",
			SHA:        "abc123",
			Name:       "atmos/plan/dev/vpc",
			Status:     provider.CheckRunStateInProgress,
			Title:      "Plan in progress...",
			DetailsURL: "https://github.com/owner/repo/actions/runs/123",
		})
		require.NoError(t, err)

		// Verify target_url was included.
		assert.Equal(t, "https://github.com/owner/repo/actions/runs/123", capturedRequest["target_url"])
	})
}

func TestProvider_UpdateCheckRun(t *testing.T) {
	t.Run("update is idempotent CreateStatus call", func(t *testing.T) {
		// UpdateCheckRun should call the same CreateStatus endpoint.
		// No prior CreateCheckRun needed — Status API is idempotent by context.
		var capturedRequest map[string]any
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/owner/repo/statuses/abc123", func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewDecoder(r.Body).Decode(&capturedRequest); err != nil {
				t.Errorf("failed to decode request: %v", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			response := map[string]any{
				"id":          12345,
				"context":     capturedRequest["context"],
				"state":       capturedRequest["state"],
				"description": capturedRequest["description"],
			}
			json.NewEncoder(w).Encode(response) //nolint:errcheck
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
		checkRun, err := p.UpdateCheckRun(ctx, &provider.UpdateCheckRunOptions{
			Owner:  "owner",
			Repo:   "repo",
			SHA:    "abc123",
			Name:   "atmos/plan/dev/vpc",
			Status: provider.CheckRunStateSuccess,
			Title:  "3 to add, 1 to change, 0 to destroy",
		})
		require.NoError(t, err)
		require.NotNil(t, checkRun)
		assert.Equal(t, int64(12345), checkRun.ID)
		assert.Equal(t, "atmos/plan/dev/vpc", checkRun.Name)
		assert.Equal(t, provider.CheckRunStateSuccess, checkRun.Status)

		// Verify request body.
		assert.Equal(t, "success", capturedRequest["state"])
		assert.Equal(t, "atmos/plan/dev/vpc", capturedRequest["context"])
		assert.Equal(t, "3 to add, 1 to change, 0 to destroy", capturedRequest["description"])
	})

	t.Run("update without prior create works identically", func(t *testing.T) {
		// No prior CreateCheckRun needed — no fallback logic.
		var requestCount int
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/owner/repo/statuses/abc123", func(w http.ResponseWriter, r *http.Request) {
			requestCount++
			w.Header().Set("Content-Type", "application/json")
			response := map[string]any{
				"id":      99999,
				"context": "atmos/plan/dev/vpc",
				"state":   "success",
			}
			json.NewEncoder(w).Encode(response) //nolint:errcheck
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
		checkRun, err := p.UpdateCheckRun(ctx, &provider.UpdateCheckRunOptions{
			Owner:  "owner",
			Repo:   "repo",
			SHA:    "abc123",
			Name:   "atmos/plan/dev/vpc",
			Status: provider.CheckRunStateSuccess,
			Title:  "No changes",
		})
		require.NoError(t, err)
		require.NotNil(t, checkRun)
		assert.Equal(t, 1, requestCount, "should make exactly one API call")
	})

	t.Run("failure state maps correctly", func(t *testing.T) {
		var capturedRequest map[string]any
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/owner/repo/statuses/abc123", func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewDecoder(r.Body).Decode(&capturedRequest); err != nil {
				t.Errorf("failed to decode request: %v", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			response := map[string]any{
				"id":      12345,
				"context": capturedRequest["context"],
				"state":   capturedRequest["state"],
			}
			json.NewEncoder(w).Encode(response) //nolint:errcheck
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
		_, err = p.UpdateCheckRun(ctx, &provider.UpdateCheckRunOptions{
			Owner:  "owner",
			Repo:   "repo",
			SHA:    "abc123",
			Name:   "atmos/plan/dev/vpc",
			Status: provider.CheckRunStateFailure,
			Title:  "Failed",
		})
		require.NoError(t, err)

		assert.Equal(t, "failure", capturedRequest["state"])
	})
}

// newErrorServer creates a test server that returns the given HTTP status code
// for all status API requests. Returns the server and a Provider connected to it.
func newErrorServer(t *testing.T, statusCode int, message string) *Provider {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/statuses/abc123", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": message})
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	serverURL, err := url.Parse(server.URL + "/")
	require.NoError(t, err)
	ghClient := github.NewClient(nil)
	ghClient.BaseURL = serverURL

	return NewProviderWithClient(&Client{client: ghClient})
}

func TestUpdateCheckRun_ErrorHints(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		message    string
		expectHint bool
	}{
		{"404 includes permission hint", http.StatusNotFound, "Not Found", true},
		{"403 includes permission hint", http.StatusForbidden, "Forbidden", true},
		{"500 has no permission hint", http.StatusInternalServerError, "Internal Server Error", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newErrorServer(t, tt.statusCode, tt.message)

			_, err := p.UpdateCheckRun(context.Background(), &provider.UpdateCheckRunOptions{
				Owner:  "owner",
				Repo:   "repo",
				SHA:    "abc123",
				Name:   "atmos/plan/dev/vpc",
				Status: provider.CheckRunStateSuccess,
				Title:  "Done",
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), strconv.Itoa(tt.statusCode))

			hints := errors.GetAllHints(err)
			allHints := strings.Join(hints, " ")
			if tt.expectHint {
				assert.Contains(t, allHints, "ATMOS_CI_GITHUB_TOKEN")
			} else {
				assert.NotContains(t, allHints, "ATMOS_CI_GITHUB_TOKEN")
			}
		})
	}
}
