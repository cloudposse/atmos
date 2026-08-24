package pro

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
)

func TestUploadInstances(t *testing.T) {
	mockRoundTripper := new(MockRoundTripper)
	httpClient := &http.Client{Transport: mockRoundTripper}
	apiClient := &AtmosProAPIClient{
		BaseURL:         "http://localhost",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      httpClient,
	}

	dto := dtos.InstancesUploadRequest{
		RepoURL:   "https://github.com/org/repo",
		RepoName:  "repo",
		RepoOwner: "org",
		RepoHost:  "github.com",
		Instances: []dtos.UploadInstance{
			{
				Component:     "vpc",
				Stack:         "tenant1-ue2-dev",
				ComponentType: "terraform",
				Settings: map[string]any{
					"pro": map[string]any{
						"drift_detection": map[string]any{
							"enabled": true,
						},
					},
				},
			},
			{
				Component:     "eks",
				Stack:         "tenant1-ue2-dev",
				ComponentType: "terraform",
				Settings: map[string]any{
					"pro": map[string]any{
						"drift_detection": map[string]any{
							"enabled": true,
						},
					},
				},
			},
		},
	}

	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"success": true}`)),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	err := apiClient.UploadInstances(&dto)
	assert.NoError(t, err)

	mockRoundTripper.AssertExpectations(t)
}

func TestUploadInstances_Error(t *testing.T) {
	mockRoundTripper := new(MockRoundTripper)
	httpClient := &http.Client{Transport: mockRoundTripper}
	apiClient := &AtmosProAPIClient{
		BaseURL:         "http://localhost",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      httpClient,
	}

	dto := dtos.InstancesUploadRequest{
		RepoURL:   "https://github.com/org/repo",
		RepoName:  "repo",
		RepoOwner: "org",
		RepoHost:  "github.com",
		Instances: []dtos.UploadInstance{
			{
				Component:     "vpc",
				Stack:         "tenant1-ue2-dev",
				ComponentType: "terraform",
				Settings: map[string]any{
					"pro": map[string]any{
						"drift_detection": map[string]any{
							"enabled": true,
						},
					},
				},
			},
			{
				Component:     "eks",
				Stack:         "tenant1-ue2-dev",
				ComponentType: "terraform",
				Settings: map[string]any{
					"pro": map[string]any{
						"drift_detection": map[string]any{
							"enabled": true,
						},
					},
				},
			},
		},
	}

	mockResponse := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(bytes.NewBufferString(`{"success": false, "errorMessage": "Internal Server Error"}`)),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	err := apiClient.UploadInstances(&dto)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrFailedToUploadInstances))

	mockRoundTripper.AssertExpectations(t)
}

// TestUploadInstances_400_NotRetried verifies that a 400 response from the
// server is treated as non-retryable: exactly one HTTP request is made, the
// returned error contains the server's user-facing message rendered as a
// bullet list, the trace_id is preserved, and the drift-detection hint is
// attached.
func TestUploadInstances_400_NotRetried(t *testing.T) {
	var requestCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{
			"success": false,
			"status": 400,
			"errorTag": "DriftDetectionValidationError",
			"errorMessage": "Drift detection validation failed: A; B",
			"data": {"validationErrors": ["A", "B"]},
			"traceId": "abc-trace"
		}`))
	}))
	defer server.Close()

	apiClient := &AtmosProAPIClient{
		BaseURL:         server.URL,
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      &http.Client{},
	}

	dto := dtos.InstancesUploadRequest{
		RepoURL:   "https://github.com/org/repo",
		RepoName:  "repo",
		RepoOwner: "org",
		RepoHost:  "github.com",
		Instances: []dtos.UploadInstance{
			{Component: "vpc", Stack: "tenant1-ue2-dev", ComponentType: "terraform"},
		},
	}

	err := apiClient.UploadInstances(&dto)
	require.Error(t, err)
	require.True(t, errors.Is(err, errUtils.ErrFailedToUploadInstances))

	// 4xx must not trigger retries.
	assert.Equal(t, int32(1), atomic.LoadInt32(&requestCount), "400 must not be retried")

	// User-facing rendering: bullets, trace_id, no "HTTP 400:" noise.
	msg := err.Error()
	assert.Contains(t, msg, "UploadInstances:")
	assert.NotContains(t, msg, "HTTP 400:")
	assert.Contains(t, msg, "- A")
	assert.Contains(t, msg, "- B")
	assert.Contains(t, msg, "trace_id: abc-trace")

	// Status-specific hints surface at the top-level error: wrapErr re-attaches
	// cockroach hints from the cause onto the outer wrap, so the CLI renderer
	// (which calls cockroachErrors.GetAllHints on the final error) sees them.
	hints := cockroachErrors.GetAllHints(err)
	allHints := strings.Join(hints, "\n")
	assert.Contains(t, allHints, "settings.pro")
	assert.Contains(t, allHints, "atmos-pro.com/docs/configure/drift-detection")
}

// TestUploadInstances_RedirectErrorClosesBody triggers the rare
// "non-nil response with non-nil error" return path of http.Client.Do via
// CheckRedirect, exercising the defensive resp.Body.Close() guard inside
// the doWithRetry closure.
func TestUploadInstances_RedirectErrorClosesBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.Path+"/loop", http.StatusFound)
	}))
	defer server.Close()

	httpClient := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errRedirectBlocked
		},
	}

	apiClient := &AtmosProAPIClient{
		BaseURL:         server.URL,
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      httpClient,
	}

	dto := dtos.InstancesUploadRequest{
		RepoURL:   "https://github.com/org/repo",
		RepoName:  "repo",
		RepoOwner: "org",
		RepoHost:  "github.com",
		Instances: []dtos.UploadInstance{
			{Component: "vpc", Stack: "tenant1-ue2-dev", ComponentType: "terraform"},
		},
	}

	err := apiClient.UploadInstances(&dto)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrFailedToUploadInstances))
	assert.True(t, errors.Is(err, errUtils.ErrFailedToMakeRequest))
}
