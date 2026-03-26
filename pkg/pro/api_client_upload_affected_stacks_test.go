package pro

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestUploadAffectedStacks_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/affected-stacks", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}))
	defer server.Close()

	client := &AtmosProAPIClient{
		BaseURL:         server.URL,
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      &http.Client{Timeout: 2 * time.Second},
	}

	dto := dtos.UploadAffectedStacksRequest{
		HeadSHA:   "test-head-sha",
		BaseSHA:   "test-base-sha",
		RepoURL:   "https://github.com/test/repo",
		RepoName:  "repo",
		RepoOwner: "test",
		RepoHost:  "github.com",
		Stacks: []schema.Affected{
			{
				Component:     "test-component",
				ComponentType: "terraform",
				Stack:         "test-stack",
				Affected:      "stack.vars",
			},
		},
	}

	err := client.UploadAffectedStacks(&dto)
	assert.NoError(t, err)
}

func TestUploadAffectedStacks_HTTPErrors(t *testing.T) {
	testCases := []struct {
		name          string
		statusCode    int
		expectedError error
	}{
		{
			name:          "server returns 400 bad request",
			statusCode:    http.StatusBadRequest,
			expectedError: errUtils.ErrFailedToUploadStacks,
		},
		{
			name:          "server returns 401 unauthorized",
			statusCode:    http.StatusUnauthorized,
			expectedError: errUtils.ErrFailedToUploadStacks,
		},
		{
			name:          "server returns 403 forbidden",
			statusCode:    http.StatusForbidden,
			expectedError: errUtils.ErrFailedToUploadStacks,
		},
		{
			name:          "server returns 500 internal server error",
			statusCode:    http.StatusInternalServerError,
			expectedError: errUtils.ErrFailedToUploadStacks,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
				w.Write([]byte(`{"error": "server error"}`))
			}))
			defer server.Close()

			client := &AtmosProAPIClient{
				BaseURL:         server.URL,
				BaseAPIEndpoint: "api",
				APIToken:        "test-token",
				HTTPClient:      &http.Client{Timeout: 2 * time.Second},
			}

			dto := dtos.UploadAffectedStacksRequest{
				HeadSHA: "test-head-sha",
				BaseSHA: "test-base-sha",
				Stacks:  []schema.Affected{},
			}

			err := client.UploadAffectedStacks(&dto)
			assert.Error(t, err)
			assert.ErrorIs(t, err, tc.expectedError)
		})
	}
}

func TestUploadAffectedStacks_NetworkError(t *testing.T) {
	client := &AtmosProAPIClient{
		BaseURL:         "http://invalid-host-that-does-not-exist:12345",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      &http.Client{Timeout: 2 * time.Second},
	}

	dto := dtos.UploadAffectedStacksRequest{
		HeadSHA: "test-head-sha",
		BaseSHA: "test-base-sha",
		Stacks:  []schema.Affected{},
	}

	err := client.UploadAffectedStacks(&dto)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToMakeRequest)
}

func TestUploadAffectedStacks_RequestCreationError(t *testing.T) {
	// Use an invalid URL that would cause http.NewRequest to fail
	client := &AtmosProAPIClient{
		BaseURL:         "://invalid-url", // Malformed URL
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      &http.Client{Timeout: 2 * time.Second},
	}

	dto := dtos.UploadAffectedStacksRequest{
		HeadSHA: "test-head-sha",
		BaseSHA: "test-base-sha",
		Stacks:  []schema.Affected{},
	}

	err := client.UploadAffectedStacks(&dto)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToCreateAuthRequest)
}

func TestUploadAffectedStacks_Chunked(t *testing.T) {
	var requestCount atomic.Int32
	var mu sync.Mutex
	var receivedBodies []dtos.UploadAffectedStacksRequest
	var handlerErr error // Captures first error from handler goroutine, guarded by mu.

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			mu.Lock()
			if handlerErr == nil {
				handlerErr = err
			}
			mu.Unlock()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var req dtos.UploadAffectedStacksRequest
		if err := json.Unmarshal(body, &req); err != nil {
			mu.Lock()
			if handlerErr == nil {
				handlerErr = err
			}
			mu.Unlock()
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		mu.Lock()
		receivedBodies = append(receivedBodies, req)
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}))
	defer server.Close()

	client := &AtmosProAPIClient{
		BaseURL:         server.URL,
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      &http.Client{Timeout: 10 * time.Second},
	}

	// Create a large payload that exceeds DefaultMaxPayloadBytes.
	largeSettings := make(schema.AtmosSectionMapType)
	bigValue := make([]byte, 1000)
	for i := range bigValue {
		bigValue[i] = 'a'
	}
	largeSettings["big"] = string(bigValue)

	numStacks := (DefaultMaxPayloadBytes / 1000) + 50
	stacks := make([]schema.Affected, numStacks)
	for i := range stacks {
		stacks[i] = schema.Affected{
			Component: "component",
			Stack:     "stack",
			Settings:  largeSettings,
		}
	}

	dto := dtos.UploadAffectedStacksRequest{
		HeadSHA:   "head-sha",
		BaseSHA:   "base-sha",
		RepoURL:   "https://github.com/test/repo",
		RepoName:  "repo",
		RepoOwner: "test",
		RepoHost:  "github.com",
		Stacks:    stacks,
	}

	err := client.UploadAffectedStacks(&dto)
	require.NoError(t, err)

	// Check for handler-side errors (read/unmarshal failures).
	mu.Lock()
	hErr := handlerErr
	mu.Unlock()
	if hErr != nil {
		t.Fatalf("httptest handler error: %v", hErr)
	}

	// Should have sent multiple requests.
	totalRequests := int(requestCount.Load())
	require.Greater(t, totalRequests, 1, "large payload should be chunked into multiple requests")

	// All requests should have the same batch_id.
	mu.Lock()
	bodies := make([]dtos.UploadAffectedStacksRequest, len(receivedBodies))
	copy(bodies, receivedBodies)
	mu.Unlock()

	require.NotEmpty(t, bodies, "receivedBodies should not be empty")
	batchID := bodies[0].BatchID
	assert.NotEmpty(t, batchID)

	totalStacks := 0
	for i, body := range bodies {
		assert.Equal(t, batchID, body.BatchID)
		require.NotNil(t, body.BatchIndex)
		assert.Equal(t, i, *body.BatchIndex)
		require.NotNil(t, body.BatchTotal)
		assert.Equal(t, totalRequests, *body.BatchTotal)
		// Metadata should be preserved in each chunk.
		assert.Equal(t, "head-sha", body.HeadSHA)
		assert.Equal(t, "base-sha", body.BaseSHA)
		assert.Equal(t, "test", body.RepoOwner)
		totalStacks += len(body.Stacks)
	}
	assert.Equal(t, numStacks, totalStacks, "all stacks should be accounted for across chunks")
}
