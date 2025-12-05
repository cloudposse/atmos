package pro

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
)

func TestUnlockStack_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path
		assert.Equal(t, "DELETE", r.Method)
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
			"data": {}
		}`))
	}))
	defer server.Close()

	client := &AtmosProAPIClient{
		BaseURL:         server.URL,
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient: &http.Client{
			Timeout: time.Second * 5,
		},
	}

	dto := dtos.UnlockStackRequest{
		Key: "test-owner/test-repo/test-stack/test-component",
	}

	response, err := client.UnlockStack(&dto)
	assert.NoError(t, err)
	assert.True(t, response.Success)
}

func TestUnlockStack_HTTPErrors(t *testing.T) {
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
			expectedError: errUtils.ErrFailedToUnlockStack,
		},
		{
			name:          "server returns 401 unauthorized",
			statusCode:    http.StatusUnauthorized,
			responseBody:  `{"success": false, "errorMessage": "Unauthorized"}`,
			expectedError: errUtils.ErrFailedToUnlockStack,
		},
		{
			name:          "server returns 404 not found",
			statusCode:    http.StatusNotFound,
			responseBody:  `{"success": false, "errorMessage": "Lock not found"}`,
			expectedError: errUtils.ErrFailedToUnlockStack,
		},
		{
			name:          "server returns 500 internal server error",
			statusCode:    http.StatusInternalServerError,
			responseBody:  `{"success": false, "errorMessage": "Internal Server Error"}`,
			expectedError: errUtils.ErrFailedToUnlockStack,
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
				HTTPClient: &http.Client{
					Timeout: time.Second * 5,
				},
			}

			dto := dtos.UnlockStackRequest{Key: "test-key"}

			response, err := client.UnlockStack(&dto)
			assert.Error(t, err)
			assert.ErrorIs(t, err, tc.expectedError)
			assert.False(t, response.Success)
		})
	}
}

func TestUnlockStack_NetworkError(t *testing.T) {
	client := &AtmosProAPIClient{
		BaseURL:         "http://invalid-host-that-does-not-exist:12345",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient: &http.Client{
			Timeout: time.Second * 5,
		},
	}

	dto := dtos.UnlockStackRequest{Key: "test-key"}

	response, err := client.UnlockStack(&dto)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToMakeRequest)
	assert.False(t, response.Success)
}

func TestUnlockStack_InvalidJSONResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`invalid json response`))
	}))
	defer server.Close()

	client := &AtmosProAPIClient{
		BaseURL:         server.URL,
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient: &http.Client{
			Timeout: time.Second * 5,
		},
	}

	dto := dtos.UnlockStackRequest{Key: "test-key"}

	response, err := client.UnlockStack(&dto)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToUnmarshalAPIResponse)
	assert.False(t, response.Success)
}

func TestUnlockStack_ReadBodyError(t *testing.T) {
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
		Body:       &FailingReaderUnlock{},
		Header:     make(http.Header),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	dto := dtos.UnlockStackRequest{Key: "test-key"}

	response, err := client.UnlockStack(&dto)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToReadResponseBody)
	assert.False(t, response.Success)

	mockRoundTripper.AssertExpectations(t)
}

func TestUnlockStack_RequestCreationError(t *testing.T) {
	// Use an invalid URL that would cause http.NewRequest to fail
	client := &AtmosProAPIClient{
		BaseURL:         "://invalid-url", // Malformed URL
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient: &http.Client{
			Timeout: time.Second * 5,
		},
	}

	dto := dtos.UnlockStackRequest{Key: "test-key"}

	response, err := client.UnlockStack(&dto)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToCreateAuthRequest)
	assert.False(t, response.Success)
}

func TestUnlockStack_SuccessFalseWithContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"success": false,
			"errorMessage": "Lock not found or already expired",
			"context": {
				"key": "test-key",
				"status": "not_found"
			}
		}`))
	}))
	defer server.Close()

	client := &AtmosProAPIClient{
		BaseURL:         server.URL,
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient: &http.Client{
			Timeout: time.Second * 5,
		},
	}

	dto := dtos.UnlockStackRequest{Key: "test-key"}

	response, err := client.UnlockStack(&dto)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToUnlockStack)
	assert.Contains(t, err.Error(), "Lock not found or already expired")
	assert.False(t, response.Success)
}

// FailingReaderUnlock is a mock io.Reader that always returns an error for unlock tests.
type FailingReaderUnlock struct{}

func (f *FailingReaderUnlock) Read(p []byte) (n int, err error) {
	return 0, errors.New("simulated read error")
}

func (f *FailingReaderUnlock) Close() error {
	return nil
}
