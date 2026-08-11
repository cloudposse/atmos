package pro

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
)

func TestUploadExecMetadata(t *testing.T) {
	mockRoundTripper := new(MockRoundTripper)
	httpClient := &http.Client{Transport: mockRoundTripper}
	apiClient := &AtmosProAPIClient{
		BaseURL:         "http://localhost",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      httpClient,
	}

	dto := dtos.ExecUploadRequest{Command: "atmos version"}

	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"success":true}`)),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	err := apiClient.UploadExecMetadata(&dto)
	assert.NoError(t, err)

	mockRoundTripper.AssertExpectations(t)
}

func TestUploadExecMetadata_Error(t *testing.T) {
	mockRoundTripper := new(MockRoundTripper)
	httpClient := &http.Client{Transport: mockRoundTripper}
	apiClient := &AtmosProAPIClient{
		BaseURL:         "http://localhost",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      httpClient,
	}

	dto := dtos.ExecUploadRequest{Command: "atmos version"}

	mockResponse := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	err := apiClient.UploadExecMetadata(&dto)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrFailedToUploadExecMetadata))

	mockRoundTripper.AssertExpectations(t)
}

// TestUploadExecMetadata_401RefreshRetry verifies a 401 triggers RefreshToken
// and a subsequent successful retry, matching doWithRetry's documented
// behavior for every other Pro upload.
func TestUploadExecMetadata_401RefreshRetry(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"success":false}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	client := &AtmosProAPIClient{
		BaseURL:         server.URL,
		BaseAPIEndpoint: "v1",
		APIToken:        "test-token",
		HTTPClient:      &http.Client{},
		useOIDC:         false, // static token: RefreshToken is a documented no-op.
	}

	err := client.UploadExecMetadata(&dtos.ExecUploadRequest{Command: "atmos version"})
	assert.NoError(t, err)
	assert.Equal(t, int32(2), attempts.Load())
}

// TestUploadExecMetadata_5xxRetry verifies a transient 5xx is retried without
// token refresh.
func TestUploadExecMetadata_5xxRetry(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"success":false}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	client := &AtmosProAPIClient{
		BaseURL:         server.URL,
		BaseAPIEndpoint: "v1",
		APIToken:        "test-token",
		HTTPClient:      &http.Client{},
	}

	err := client.UploadExecMetadata(&dtos.ExecUploadRequest{Command: "atmos version"})
	assert.NoError(t, err)
	assert.Equal(t, int32(2), attempts.Load())
}

// TestUploadExecMetadata_NonRetryable verifies 400/403/404 are returned
// immediately, without retry.
func TestUploadExecMetadata_NonRetryable(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"success":false}`))
			}))
			defer server.Close()

			client := &AtmosProAPIClient{
				BaseURL:         server.URL,
				BaseAPIEndpoint: "v1",
				APIToken:        "test-token",
				HTTPClient:      &http.Client{},
			}

			err := client.UploadExecMetadata(&dtos.ExecUploadRequest{Command: "atmos version"})
			assert.Error(t, err)
			assert.Equal(t, int32(1), attempts.Load())
		})
	}
}
