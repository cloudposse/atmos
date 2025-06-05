package pro

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/cloudposse/atmos/pkg/logger"
)

// MockRoundTripper is an implementation of http.RoundTripper for testing purposes.
type MockRoundTripper struct {
	mock.Mock
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	args := m.Called(req)
	return args.Get(0).(*http.Response), args.Error(1)
}

func TestLockStack(t *testing.T) {
	mockLogger, err := logger.NewLogger("test", "/dev/stdout")
	assert.Nil(t, err)

	mockRoundTripper := new(MockRoundTripper)
	httpClient := &http.Client{Transport: mockRoundTripper}
	apiClient := &AtmosProAPIClient{
		Logger:          mockLogger,
		BaseURL:         "http://localhost",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      httpClient,
	}

	dto := LockStackRequest{ /* populate fields */ }

	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"success": true}`)),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	response, err := apiClient.LockStack(dto)
	assert.NoError(t, err)
	assert.True(t, response.Success)

	mockRoundTripper.AssertExpectations(t)
}

func TestLockStack_Error(t *testing.T) {
	mockLogger, err := logger.NewLogger("test", "/dev/stdout")
	assert.Nil(t, err)

	mockRoundTripper := new(MockRoundTripper)
	httpClient := &http.Client{Transport: mockRoundTripper}
	apiClient := &AtmosProAPIClient{
		Logger:          mockLogger,
		BaseURL:         "http://localhost",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      httpClient,
	}

	dto := LockStackRequest{ /* populate fields */ }

	mockResponse := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(bytes.NewBufferString(`{"success": false, "errorMessage": "Internal Server Error"}`)),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	response, err := apiClient.LockStack(dto)
	assert.Error(t, err)
	assert.False(t, response.Success)
	assert.Contains(t, err.Error(), "Internal Server Error")

	mockRoundTripper.AssertExpectations(t)
}

func TestUnlockStack(t *testing.T) {
	mockLogger, err := logger.NewLogger("test", "/dev/stdout")
	assert.Nil(t, err)

	mockRoundTripper := new(MockRoundTripper)
	httpClient := &http.Client{Transport: mockRoundTripper}
	apiClient := &AtmosProAPIClient{
		Logger:          mockLogger,
		BaseURL:         "http://localhost",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      httpClient,
	}

	dto := UnlockStackRequest{}

	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"success": true}`)),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	response, err := apiClient.UnlockStack(dto)
	assert.NoError(t, err)
	assert.True(t, response.Success)

	mockRoundTripper.AssertExpectations(t)
}

func TestUnlockStack_Error(t *testing.T) {
	mockLogger, err := logger.NewLogger("test", "/dev/stdout")
	assert.Nil(t, err)

	mockRoundTripper := new(MockRoundTripper)
	httpClient := &http.Client{Transport: mockRoundTripper}
	apiClient := &AtmosProAPIClient{
		Logger:          mockLogger,
		BaseURL:         "http://localhost",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      httpClient,
	}

	dto := UnlockStackRequest{}

	mockResponse := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(bytes.NewBufferString(`{"request":"1", "success": false, "errorMessage": "Internal Server Error"}`)),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	response, err := apiClient.UnlockStack(dto)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Internal Server Error")
	assert.False(t, response.Success)

	mockRoundTripper.AssertExpectations(t)
}

func TestNewAtmosProAPIClientFromEnv_OIDC(t *testing.T) {
	// Save original env vars and restore them after the test
	originalEnvVars := map[string]string{
		"ACTIONS_ID_TOKEN_REQUEST_URL":   os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL"),
		"ACTIONS_ID_TOKEN_REQUEST_TOKEN": os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN"),
		"ATMOS_PRO_WORKSPACE_ID":         os.Getenv("ATMOS_PRO_WORKSPACE_ID"),
		"ATMOS_PRO_TOKEN":                os.Getenv("ATMOS_PRO_TOKEN"),
		"ATMOS_PRO_BASE_URL":             os.Getenv("ATMOS_PRO_BASE_URL"),
		"ATMOS_PRO_ENDPOINT":             os.Getenv("ATMOS_PRO_ENDPOINT"),
	}
	defer func() {
		for k, v := range originalEnvVars {
			if v != "" {
				os.Setenv(k, v)
			} else {
				os.Unsetenv(k)
			}
		}
	}()

	mockLogger, err := logger.NewLogger("test", "/dev/stdout")
	assert.Nil(t, err)

	// Test successful OIDC authentication
	t.Run("successful OIDC authentication", func(t *testing.T) {
		// Set up test servers
		githubOIDCServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"value": "github-oidc-token"}`))
		}))
		defer githubOIDCServer.Close()

		atmosAPIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/auth/github-oidc" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"token": "atmos-token"}`))
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer atmosAPIServer.Close()

		// Set up mock environment
		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", githubOIDCServer.URL+"?audience=app.cloudposse.com")
		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "test-token")
		os.Setenv("ATMOS_PRO_WORKSPACE_ID", "test-workspace")
		os.Setenv("ATMOS_PRO_BASE_URL", atmosAPIServer.URL)
		os.Setenv("ATMOS_PRO_ENDPOINT", "api")
		os.Unsetenv("ATMOS_PRO_TOKEN")

		client, err := NewAtmosProAPIClientFromEnv(mockLogger)
		assert.NoError(t, err)
		assert.NotNil(t, client)
		assert.Equal(t, "atmos-token", client.APIToken)
	})

	// Test fallback to API token when OIDC fails
	t.Run("fallback to API token", func(t *testing.T) {
		// Set up test server for OIDC (returns unauthorized)
		githubOIDCServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error": "invalid token"}`))
		}))
		defer githubOIDCServer.Close()

		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", githubOIDCServer.URL+"?audience=app.cloudposse.com")
		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "invalid-token")
		os.Setenv("ATMOS_PRO_TOKEN", "fallback-token")
		os.Setenv("ATMOS_PRO_BASE_URL", "http://localhost")
		os.Setenv("ATMOS_PRO_ENDPOINT", "api")
		os.Unsetenv("ATMOS_PRO_WORKSPACE_ID")

		client, err := NewAtmosProAPIClientFromEnv(mockLogger)
		assert.NoError(t, err)
		assert.NotNil(t, client)
		assert.Equal(t, "fallback-token", client.APIToken)
	})

	// Test failure when both OIDC and API token are missing
	t.Run("both auth methods fail", func(t *testing.T) {
		// Set up test server for OIDC (returns unauthorized)
		githubOIDCServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error": "invalid token"}`))
		}))
		defer githubOIDCServer.Close()

		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", githubOIDCServer.URL+"?audience=app.cloudposse.com")
		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "invalid-token")
		os.Setenv("ATMOS_PRO_BASE_URL", "http://localhost")
		os.Setenv("ATMOS_PRO_ENDPOINT", "api")
		os.Unsetenv("ATMOS_PRO_TOKEN")
		os.Unsetenv("ATMOS_PRO_WORKSPACE_ID")

		client, err := NewAtmosProAPIClientFromEnv(mockLogger)
		assert.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "OIDC authentication failed and ATMOS_PRO_TOKEN is not set")
	})
}
