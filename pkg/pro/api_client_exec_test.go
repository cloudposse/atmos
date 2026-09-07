package pro

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

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

// TestUploadExecMetadata_InlineUnderThreshold verifies a record whose
// marshaled size is under MaxPayloadBytes is sent as a single request with
// Data inline (research.md Decision 16) — no /exec/data call is made.
func TestUploadExecMetadata_InlineUnderThreshold(t *testing.T) {
	var execRequests, dataRequests atomic.Int32
	var receivedBody dtos.ExecUploadRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/atmos/exec":
			execRequests.Add(1)
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(body, &receivedBody))
		case "/api/atmos/exec/data":
			dataRequests.Add(1)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	client := &AtmosProAPIClient{
		BaseURL:         server.URL,
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      &http.Client{Timeout: 10 * time.Second},
	}

	dto := dtos.ExecUploadRequest{
		ExecutionID: "11111111-1111-4111-8111-111111111111",
		Command:     "atmos terraform plan",
		Data:        json.RawMessage(`{"resource_counts":{"create":1}}`),
	}

	err := client.UploadExecMetadata(&dto)
	require.NoError(t, err)

	assert.Equal(t, int32(1), execRequests.Load(), "exactly one /exec request for a small record")
	assert.Equal(t, int32(0), dataRequests.Load(), "no /exec/data call for a small record")
	assert.JSONEq(t, `{"resource_counts":{"create":1}}`, string(receivedBody.Data), "Data must be inline, unchanged")
}

// TestUploadExecMetadata_BlobURLOverThreshold verifies a record whose
// marshaled size is at/over MaxPayloadBytes is delivered via a single
// /exec/data upload (never chunked) followed by a single /exec request whose
// Data is the returned URL, keyed by ExecutionID (FR-011, research.md
// Decision 16) — replacing the retired multi-chunk model.
func TestUploadExecMetadata_BlobURLOverThreshold(t *testing.T) {
	var execRequests, dataRequests atomic.Int32
	var mu sync.Mutex
	var execBody dtos.ExecUploadRequest
	var dataBody dtos.ExecDataUploadRequest

	const blobURL = "https://blob.vercel-storage.com/atmos-exec/example/data.json"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		switch r.URL.Path {
		case "/api/atmos/exec/data":
			dataRequests.Add(1)
			mu.Lock()
			require.NoError(t, json.Unmarshal(body, &dataBody))
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success":true,"url":"` + blobURL + `"}`))
			return
		case "/api/atmos/exec":
			execRequests.Add(1)
			mu.Lock()
			require.NoError(t, json.Unmarshal(body, &execBody))
			mu.Unlock()
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	client := &AtmosProAPIClient{
		BaseURL:         server.URL,
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      &http.Client{Timeout: 10 * time.Second},
		MaxPayloadBytes: 128, // artificially small so the fixed test payload trips the threshold.
	}

	bigData, err := json.Marshal(map[string]string{"padding": string(make([]byte, 1000))})
	require.NoError(t, err)

	dto := dtos.ExecUploadRequest{
		ExecutionID: "22222222-2222-4222-8222-222222222222",
		Command:     "atmos terraform plan",
		Data:        json.RawMessage(bigData),
	}

	err = client.UploadExecMetadata(&dto)
	require.NoError(t, err)

	assert.Equal(t, int32(1), dataRequests.Load(), "exactly one /exec/data upload, never chunked")
	assert.Equal(t, int32(1), execRequests.Load(), "exactly one /exec request following the blob upload")

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, dto.ExecutionID, dataBody.ExecutionID, "ExecutionID must key the /exec/data request")
	assert.JSONEq(t, string(bigData), string(dataBody.Data), "the original structured data must be sent to /exec/data unchanged")

	var execDataAsString string
	require.NoError(t, json.Unmarshal(execBody.Data, &execDataAsString), "the main /exec request's Data must be a JSON string (the blob URL), not inline")
	assert.Equal(t, blobURL, execDataAsString)
	assert.Equal(t, dto.ExecutionID, execBody.ExecutionID)
}

// TestUploadExecData_Success verifies UploadExecData's request/response
// shape directly.
func TestUploadExecData_Success(t *testing.T) {
	var received dtos.ExecDataUploadRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/atmos/exec/data", r.URL.Path)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &received))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"url":"https://blob.example/data.json"}`))
	}))
	defer server.Close()

	client := &AtmosProAPIClient{
		BaseURL:         server.URL,
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      &http.Client{Timeout: 10 * time.Second},
	}

	resp, err := client.UploadExecData(&dtos.ExecDataUploadRequest{
		ExecutionID: "33333333-3333-4333-8333-333333333333",
		Data:        json.RawMessage(`{"foo":"bar"}`),
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "https://blob.example/data.json", resp.URL)
	assert.Equal(t, "33333333-3333-4333-8333-333333333333", received.ExecutionID)
	assert.JSONEq(t, `{"foo":"bar"}`, string(received.Data))
}

// TestUploadExecData_Error verifies a non-2xx response surfaces as an error
// wrapping ErrFailedToUploadExecData.
func TestUploadExecData_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"success":false}`))
	}))
	defer server.Close()

	client := &AtmosProAPIClient{
		BaseURL:         server.URL,
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      &http.Client{Timeout: 10 * time.Second},
	}

	resp, err := client.UploadExecData(&dtos.ExecDataUploadRequest{ExecutionID: "x", Data: json.RawMessage(`{}`)})
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.True(t, errors.Is(err, errUtils.ErrFailedToUploadExecData))
}

// TestUploadExecData_MalformedJSONOn2xx verifies a 2xx response with a body
// that fails to decode as JSON is treated as an error, never as success.
func TestUploadExecData_MalformedJSONOn2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()

	client := &AtmosProAPIClient{
		BaseURL:         server.URL,
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      &http.Client{Timeout: 10 * time.Second},
	}

	resp, err := client.UploadExecData(&dtos.ExecDataUploadRequest{ExecutionID: "x", Data: json.RawMessage(`{}`)})
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.True(t, errors.Is(err, errUtils.ErrFailedToUploadExecData))
}

// TestUploadExecData_MissingURLOn2xx verifies a 2xx response that omits the
// blob URL is treated as an error rather than a usable success.
func TestUploadExecData_MissingURLOn2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	client := &AtmosProAPIClient{
		BaseURL:         server.URL,
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      &http.Client{Timeout: 10 * time.Second},
	}

	resp, err := client.UploadExecData(&dtos.ExecDataUploadRequest{ExecutionID: "x", Data: json.RawMessage(`{}`)})
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.True(t, errors.Is(err, errUtils.ErrFailedToUploadExecData))
	assert.True(t, errors.Is(err, errUtils.ErrEmptyURL))
}
