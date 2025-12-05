package pro

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
)

func TestLockStack_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/locks", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		// Verify request body
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.Contains(t, string(body), "test-owner/test-repo/test-stack/test-component")

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"success": true,
			"data": {
				"id": "lock-123",
				"key": "test-owner/test-repo/test-stack/test-component"
			}
		}`))
	}))
	defer server.Close()

	client := &AtmosProAPIClient{
		BaseURL:         server.URL,
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      http.DefaultClient,
	}

	dto := dtos.LockStackRequest{
		Key:         "test-owner/test-repo/test-stack/test-component",
		TTL:         300,
		LockMessage: "Test lock",
	}

	response, err := client.LockStack(&dto)
	assert.NoError(t, err)
	assert.True(t, response.Success)
}

func TestLockStack_HTTPErrors(t *testing.T) {
	testCases := []struct {
		name          string
		statusCode    int
		responseBody  string
		expectedError error
	}{
		{
			name:          "server returns 400 bad request",
			statusCode:    http.StatusBadRequest,
			responseBody:  `{"success": false, "errorMessage": "Bad request"}`,
			expectedError: errUtils.ErrFailedToLockStack,
		},
		{
			name:          "server returns 401 unauthorized",
			statusCode:    http.StatusUnauthorized,
			responseBody:  `{"success": false, "errorMessage": "Unauthorized"}`,
			expectedError: errUtils.ErrFailedToLockStack,
		},
		{
			name:          "server returns 500 internal server error",
			statusCode:    http.StatusInternalServerError,
			responseBody:  `{"success": false, "errorMessage": "Internal Server Error"}`,
			expectedError: errUtils.ErrFailedToLockStack,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
				w.Write([]byte(tc.responseBody))
			}))
			defer server.Close()

			client := &AtmosProAPIClient{
				BaseURL:         server.URL,
				BaseAPIEndpoint: "api",
				APIToken:        "test-token",
				HTTPClient:      http.DefaultClient,
			}

			dto := dtos.LockStackRequest{Key: "test-key"}

			response, err := client.LockStack(&dto)
			assert.Error(t, err)
			assert.ErrorIs(t, err, tc.expectedError)
			assert.False(t, response.Success)
		})
	}
}

func TestLockStack_NetworkError(t *testing.T) {
	client := &AtmosProAPIClient{
		BaseURL:         "http://invalid-host-that-does-not-exist:12345",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      http.DefaultClient,
	}

	dto := dtos.LockStackRequest{Key: "test-key"}

	response, err := client.LockStack(&dto)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToMakeRequest)
	assert.False(t, response.Success)
}

func TestLockStack_InvalidJSONResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`invalid json response`))
	}))
	defer server.Close()

	client := &AtmosProAPIClient{
		BaseURL:         server.URL,
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      http.DefaultClient,
	}

	dto := dtos.LockStackRequest{Key: "test-key"}

	response, err := client.LockStack(&dto)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToUnmarshalAPIResponse)
	assert.False(t, response.Success)
}

func TestLockStack_ReadBodyError(t *testing.T) {
	mockRoundTripper := new(MockRoundTripper)
	httpClient := &http.Client{Transport: mockRoundTripper}

	client := &AtmosProAPIClient{
		BaseURL:         "http://localhost",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      httpClient,
	}

	// Mock response with body that will fail to read
	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       &FailingReader{},
		Header:     make(http.Header),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	dto := dtos.LockStackRequest{Key: "test-key"}

	response, err := client.LockStack(&dto)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToReadResponseBody)
	assert.False(t, response.Success)

	mockRoundTripper.AssertExpectations(t)
}

func TestLockStack_RequestCreationError(t *testing.T) {
	// Use an invalid URL that would cause http.NewRequest to fail
	client := &AtmosProAPIClient{
		BaseURL:         "://invalid-url", // Malformed URL
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      http.DefaultClient,
	}

	dto := dtos.LockStackRequest{Key: "test-key"}

	response, err := client.LockStack(&dto)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToCreateAuthRequest)
	assert.False(t, response.Success)
}

func TestLockStack_SuccessFalseWithContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"success": false,
			"errorMessage": "Stack is already locked",
			"context": {
				"locked_by": "user123",
				"locked_at": "2023-01-01T12:00:00Z"
			}
		}`))
	}))
	defer server.Close()

	client := &AtmosProAPIClient{
		BaseURL:         server.URL,
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      http.DefaultClient,
	}

	dto := dtos.LockStackRequest{Key: "test-key"}

	response, err := client.LockStack(&dto)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToLockStack)
	assert.Contains(t, err.Error(), "Stack is already locked")
	assert.False(t, response.Success)
}

// FailingReader is a mock io.Reader that always returns an error.
type FailingReader struct{}

func (f *FailingReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("simulated read error")
}

func (f *FailingReader) Close() error {
	return nil
}
