package pro

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
)

// MockRoundTripper is an implementation of http.RoundTripper for testing purposes.
type MockRoundTripper struct {
	mock.Mock
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	args := m.Called(req)
	return args.Get(0).(*http.Response), args.Error(1)
}

func TestNewAtmosProAPIClientFromEnv(t *testing.T) {
	// Save original env vars
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
				t.Setenv(k, v)
			} else {
				os.Unsetenv(k)
			}
		}
		viper.Reset()
	}()

	t.Run("api token set - skip OIDC", func(t *testing.T) {
		t.Setenv("ATMOS_PRO_BASE_URL", "https://api.atmos.example.com")
		t.Setenv("ATMOS_PRO_ENDPOINT", "v1")
		t.Setenv("ATMOS_PRO_TOKEN", "direct-api-token")
		os.Unsetenv("ACTIONS_ID_TOKEN_REQUEST_URL")
		os.Unsetenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
		os.Unsetenv("ATMOS_PRO_WORKSPACE_ID")

		viper.Reset()
		// Bind environment variables like the main application does
		viper.BindEnv("ATMOS_PRO_BASE_URL", "ATMOS_PRO_BASE_URL")
		viper.BindEnv("ATMOS_PRO_ENDPOINT", "ATMOS_PRO_ENDPOINT")
		viper.BindEnv("ATMOS_PRO_TOKEN", "ATMOS_PRO_TOKEN")
		viper.BindEnv("ATMOS_PRO_WORKSPACE_ID", "ATMOS_PRO_WORKSPACE_ID")

		// Create AtmosConfiguration with Pro settings populated from environment
		atmosConfig := schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				Pro: schema.ProSettings{
					BaseURL:     os.Getenv("ATMOS_PRO_BASE_URL"),
					Endpoint:    os.Getenv("ATMOS_PRO_ENDPOINT"),
					Token:       os.Getenv("ATMOS_PRO_TOKEN"),
					WorkspaceID: os.Getenv("ATMOS_PRO_WORKSPACE_ID"),
					GithubOIDC: schema.GithubOIDCSettings{
						RequestURL:   os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL"),
						RequestToken: os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN"),
					},
				},
			},
		}

		client, err := NewAtmosProAPIClientFromEnv(&atmosConfig)
		assert.NoError(t, err)
		assert.NotNil(t, client)
		assert.Equal(t, "direct-api-token", client.APIToken)
		assert.Equal(t, "https://api.atmos.example.com", client.BaseURL)
		assert.Equal(t, "v1", client.BaseAPIEndpoint)
	})

	t.Run("api token set with defaults", func(t *testing.T) {
		t.Setenv("ATMOS_PRO_TOKEN", "direct-api-token")
		// Unset custom URLs to test defaults
		os.Unsetenv("ATMOS_PRO_BASE_URL")
		os.Unsetenv("ATMOS_PRO_ENDPOINT")
		os.Unsetenv("ACTIONS_ID_TOKEN_REQUEST_URL")
		os.Unsetenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
		os.Unsetenv("ATMOS_PRO_WORKSPACE_ID")

		viper.Reset()
		// Bind environment variables like the main application does
		viper.BindEnv("ATMOS_PRO_BASE_URL", "ATMOS_PRO_BASE_URL")
		viper.BindEnv("ATMOS_PRO_ENDPOINT", "ATMOS_PRO_ENDPOINT")
		viper.BindEnv("ATMOS_PRO_TOKEN", "ATMOS_PRO_TOKEN")
		viper.BindEnv("ATMOS_PRO_WORKSPACE_ID", "ATMOS_PRO_WORKSPACE_ID")

		// Create AtmosConfiguration with Pro settings populated from environment
		atmosConfig := schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				Pro: schema.ProSettings{
					BaseURL:     os.Getenv("ATMOS_PRO_BASE_URL"),
					Endpoint:    os.Getenv("ATMOS_PRO_ENDPOINT"),
					Token:       os.Getenv("ATMOS_PRO_TOKEN"),
					WorkspaceID: os.Getenv("ATMOS_PRO_WORKSPACE_ID"),
					GithubOIDC: schema.GithubOIDCSettings{
						RequestURL:   os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL"),
						RequestToken: os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN"),
					},
				},
			},
		}

		client, err := NewAtmosProAPIClientFromEnv(&atmosConfig)
		assert.NoError(t, err)
		assert.NotNil(t, client)
		assert.Equal(t, "direct-api-token", client.APIToken)
		assert.Equal(t, "https://atmos-pro.com", client.BaseURL) // Default
		assert.Equal(t, "api/v1", client.BaseAPIEndpoint)        // Default
	})

	t.Run("successful OIDC flow", func(t *testing.T) {
		// Set up mock server for OIDC token request
		oidcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "Bearer test-request-token", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"value": "github-oidc-token-123"}`))
		}))
		defer oidcServer.Close()

		// Set up mock server for token exchange
		exchangeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success": true, "data": {"token": "atmos-pro-token-456"}}`))
		}))
		defer exchangeServer.Close()

		// Unset API token to force OIDC flow
		os.Unsetenv("ATMOS_PRO_TOKEN")
		t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", oidcServer.URL+"?token=dummy")
		t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "test-request-token")
		t.Setenv("ATMOS_PRO_WORKSPACE_ID", "test-workspace")
		t.Setenv("ATMOS_PRO_BASE_URL", exchangeServer.URL)

		viper.Reset()
		// Bind environment variables like the main application does
		viper.BindEnv("ATMOS_PRO_BASE_URL", "ATMOS_PRO_BASE_URL")
		viper.BindEnv("ATMOS_PRO_ENDPOINT", "ATMOS_PRO_ENDPOINT")
		viper.BindEnv("ATMOS_PRO_TOKEN", "ATMOS_PRO_TOKEN")
		viper.BindEnv("ATMOS_PRO_WORKSPACE_ID", "ATMOS_PRO_WORKSPACE_ID")

		// Create AtmosConfiguration with Pro settings populated from environment
		atmosConfig := schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				Pro: schema.ProSettings{
					BaseURL:     os.Getenv("ATMOS_PRO_BASE_URL"),
					Endpoint:    os.Getenv("ATMOS_PRO_ENDPOINT"),
					Token:       os.Getenv("ATMOS_PRO_TOKEN"),
					WorkspaceID: os.Getenv("ATMOS_PRO_WORKSPACE_ID"),
					GithubOIDC: schema.GithubOIDCSettings{
						RequestURL:   os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL"),
						RequestToken: os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN"),
					},
				},
			},
		}

		client, err := NewAtmosProAPIClientFromEnv(&atmosConfig)
		assert.NoError(t, err)
		assert.NotNil(t, client)
		assert.Equal(t, "atmos-pro-token-456", client.APIToken)
	})

	t.Run("missing workspace ID for OIDC", func(t *testing.T) {
		// Set up mock server for OIDC token request
		oidcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"value": "github-oidc-token-123"}`))
		}))
		defer oidcServer.Close()

		// Unset API token and workspace ID to trigger error
		os.Unsetenv("ATMOS_PRO_TOKEN")
		os.Unsetenv("ATMOS_PRO_WORKSPACE_ID")
		t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", oidcServer.URL+"?token=dummy")
		t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "test-request-token")

		viper.Reset()
		// Bind environment variables like the main application does
		viper.BindEnv("ATMOS_PRO_BASE_URL", "ATMOS_PRO_BASE_URL")
		viper.BindEnv("ATMOS_PRO_ENDPOINT", "ATMOS_PRO_ENDPOINT")
		viper.BindEnv("ATMOS_PRO_TOKEN", "ATMOS_PRO_TOKEN")
		viper.BindEnv("ATMOS_PRO_WORKSPACE_ID", "ATMOS_PRO_WORKSPACE_ID")

		// Create AtmosConfiguration with Pro settings populated from environment
		atmosConfig := schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				Pro: schema.ProSettings{
					BaseURL:     os.Getenv("ATMOS_PRO_BASE_URL"),
					Endpoint:    os.Getenv("ATMOS_PRO_ENDPOINT"),
					Token:       os.Getenv("ATMOS_PRO_TOKEN"),
					WorkspaceID: os.Getenv("ATMOS_PRO_WORKSPACE_ID"),
					GithubOIDC: schema.GithubOIDCSettings{
						RequestURL:   os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL"),
						RequestToken: os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN"),
					},
				},
			},
		}

		client, err := NewAtmosProAPIClientFromEnv(&atmosConfig)
		assert.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "workspace ID environment variable is required")
	})

	t.Run("GitHub OIDC token fetch fails", func(t *testing.T) {
		// Unset API token to force OIDC flow
		os.Unsetenv("ATMOS_PRO_TOKEN")
		// Unset OIDC env vars to trigger failure
		os.Unsetenv("ACTIONS_ID_TOKEN_REQUEST_URL")
		os.Unsetenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")

		viper.Reset()
		// Bind environment variables like the main application does
		viper.BindEnv("ATMOS_PRO_BASE_URL", "ATMOS_PRO_BASE_URL")
		viper.BindEnv("ATMOS_PRO_ENDPOINT", "ATMOS_PRO_ENDPOINT")
		viper.BindEnv("ATMOS_PRO_TOKEN", "ATMOS_PRO_TOKEN")
		viper.BindEnv("ATMOS_PRO_WORKSPACE_ID", "ATMOS_PRO_WORKSPACE_ID")

		// Create AtmosConfiguration with Pro settings populated from environment
		atmosConfig := schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				Pro: schema.ProSettings{
					BaseURL:     os.Getenv("ATMOS_PRO_BASE_URL"),
					Endpoint:    os.Getenv("ATMOS_PRO_ENDPOINT"),
					Token:       os.Getenv("ATMOS_PRO_TOKEN"),
					WorkspaceID: os.Getenv("ATMOS_PRO_WORKSPACE_ID"),
					GithubOIDC: schema.GithubOIDCSettings{
						RequestURL:   os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL"),
						RequestToken: os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN"),
					},
				},
			},
		}

		client, err := NewAtmosProAPIClientFromEnv(&atmosConfig)
		assert.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "not running in GitHub Actions")
	})

	t.Run("OIDC token exchange fails", func(t *testing.T) {
		// Set up mock server for OIDC token request
		oidcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"value": "github-oidc-token-123"}`))
		}))
		defer oidcServer.Close()

		// Set up mock server for failed token exchange
		exchangeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"success": false, "error": "unauthorized"}`))
		}))
		defer exchangeServer.Close()

		// Unset API token to force OIDC flow
		os.Unsetenv("ATMOS_PRO_TOKEN")
		t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", oidcServer.URL+"?token=dummy")
		t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "test-request-token")
		t.Setenv("ATMOS_PRO_WORKSPACE_ID", "test-workspace")
		t.Setenv("ATMOS_PRO_BASE_URL", exchangeServer.URL)

		viper.Reset()
		// Bind environment variables like the main application does
		viper.BindEnv("ATMOS_PRO_BASE_URL", "ATMOS_PRO_BASE_URL")
		viper.BindEnv("ATMOS_PRO_ENDPOINT", "ATMOS_PRO_ENDPOINT")
		viper.BindEnv("ATMOS_PRO_TOKEN", "ATMOS_PRO_TOKEN")
		viper.BindEnv("ATMOS_PRO_WORKSPACE_ID", "ATMOS_PRO_WORKSPACE_ID")

		// Create AtmosConfiguration with Pro settings populated from environment
		atmosConfig := schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				Pro: schema.ProSettings{
					BaseURL:     os.Getenv("ATMOS_PRO_BASE_URL"),
					Endpoint:    os.Getenv("ATMOS_PRO_ENDPOINT"),
					Token:       os.Getenv("ATMOS_PRO_TOKEN"),
					WorkspaceID: os.Getenv("ATMOS_PRO_WORKSPACE_ID"),
					GithubOIDC: schema.GithubOIDCSettings{
						RequestURL:   os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL"),
						RequestToken: os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN"),
					},
				},
			},
		}

		client, err := NewAtmosProAPIClientFromEnv(&atmosConfig)
		assert.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "failed to exchange OIDC token")
	})

	t.Run("no GitHub Actions environment", func(t *testing.T) {
		// Unset all environment variables
		os.Unsetenv("ATMOS_PRO_TOKEN")
		os.Unsetenv("ACTIONS_ID_TOKEN_REQUEST_URL")
		os.Unsetenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")

		viper.Reset()
		// Bind environment variables like the main application does
		viper.BindEnv("ATMOS_PRO_BASE_URL", "ATMOS_PRO_BASE_URL")
		viper.BindEnv("ATMOS_PRO_ENDPOINT", "ATMOS_PRO_ENDPOINT")
		viper.BindEnv("ATMOS_PRO_TOKEN", "ATMOS_PRO_TOKEN")
		viper.BindEnv("ATMOS_PRO_WORKSPACE_ID", "ATMOS_PRO_WORKSPACE_ID")

		// Create AtmosConfiguration with Pro settings populated from environment
		atmosConfig := schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				Pro: schema.ProSettings{
					BaseURL:     os.Getenv("ATMOS_PRO_BASE_URL"),
					Endpoint:    os.Getenv("ATMOS_PRO_ENDPOINT"),
					Token:       os.Getenv("ATMOS_PRO_TOKEN"),
					WorkspaceID: os.Getenv("ATMOS_PRO_WORKSPACE_ID"),
					GithubOIDC: schema.GithubOIDCSettings{
						RequestURL:   os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL"),
						RequestToken: os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN"),
					},
				},
			},
		}

		client, err := NewAtmosProAPIClientFromEnv(&atmosConfig)
		assert.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "not running in GitHub Actions")
	})
}

// TestGetAuthenticatedRequest tests the getAuthenticatedRequest function with error handling.
func TestGetAuthenticatedRequest(t *testing.T) {
	client := &AtmosProAPIClient{
		APIToken: "test-token",
	}

	t.Run("Valid request", func(t *testing.T) {
		req, err := getAuthenticatedRequest(client, "GET", "https://api.example.com/test", nil)
		assert.NoError(t, err)
		assert.NotNil(t, req)
		assert.Equal(t, "Bearer test-token", req.Header.Get("Authorization"))
		assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
	})

	t.Run("Invalid URL", func(t *testing.T) {
		req, err := getAuthenticatedRequest(client, "GET", "://invalid-url", nil)
		assert.Error(t, err)
		assert.Nil(t, req)
		assert.True(t, errors.Is(err, errUtils.ErrFailedToCreateRequest))
	})
}

// TestUploadAffectedStacks tests the UploadAffectedStacks method with error handling.
func TestUploadAffectedStacks(t *testing.T) {
	t.Run("Request creation error", func(t *testing.T) {
		client := &AtmosProAPIClient{
			BaseURL:         "://invalid-url",
			BaseAPIEndpoint: "v1",
			APIToken:        "test-token",
			HTTPClient:      &http.Client{},
		}

		dto := &dtos.UploadAffectedStacksRequest{
			RepoName: "test-repo",
			HeadSHA:  "abc123",
			BaseSHA:  "def456",
		}

		err := client.UploadAffectedStacks(dto)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, errUtils.ErrFailedToCreateAuthRequest))
	})

	t.Run("HTTP request error", func(t *testing.T) {
		// Create a mock HTTP client that returns an error.
		mockTransport := &MockRoundTripper{}
		mockTransport.On("RoundTrip", mock.Anything).Return(&http.Response{}, errors.New("network error"))

		client := &AtmosProAPIClient{
			BaseURL:         "https://api.example.com",
			BaseAPIEndpoint: "v1",
			APIToken:        "test-token",
			HTTPClient:      &http.Client{Transport: mockTransport},
		}

		dto := &dtos.UploadAffectedStacksRequest{
			RepoName: "test-repo",
			HeadSHA:  "abc123",
			BaseSHA:  "def456",
		}

		err := client.UploadAffectedStacks(dto)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, errUtils.ErrFailedToMakeRequest))
		mockTransport.AssertExpectations(t)
	})

	t.Run("API response error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "Internal Server Error"}`))
		}))
		defer server.Close()

		client := &AtmosProAPIClient{
			BaseURL:         server.URL,
			BaseAPIEndpoint: "v1",
			APIToken:        "test-token",
			HTTPClient:      &http.Client{},
		}

		dto := &dtos.UploadAffectedStacksRequest{
			RepoName: "test-repo",
			HeadSHA:  "abc123",
			BaseSHA:  "def456",
		}

		err := client.UploadAffectedStacks(dto)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, errUtils.ErrFailedToUploadStacks))
	})
}

// TestDoStackLockAction tests the doStackLockAction method with error handling.
func TestDoStackLockAction(t *testing.T) {
	t.Run("Marshal error", func(t *testing.T) {
		client := &AtmosProAPIClient{
			BaseURL:         "https://api.example.com",
			BaseAPIEndpoint: "v1",
			APIToken:        "test-token",
			HTTPClient:      &http.Client{},
		}

		// Create params with unmarshalable body (e.g., channel).
		params := &schema.StackLockActionParams{
			Method: "POST",
			URL:    "https://api.example.com/lock",
			Body:   make(chan int), // This will fail to marshal
			Out:    make(map[string]interface{}),
		}

		err := client.doStackLockAction(params)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, errUtils.ErrFailedToMarshalPayload))
	})

	t.Run("Response body read error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "1000")
			w.WriteHeader(http.StatusOK)
			// Don't write the full content to simulate read error.
		}))
		defer server.Close()

		client := &AtmosProAPIClient{
			BaseURL:         server.URL,
			BaseAPIEndpoint: "v1",
			APIToken:        "test-token",
			HTTPClient:      &http.Client{},
		}

		params := &schema.StackLockActionParams{
			Method: "POST",
			URL:    server.URL + "/lock",
			Body:   map[string]string{"action": "lock"},
			Out:    make(map[string]interface{}),
		}

		err := client.doStackLockAction(params)
		assert.Error(t, err)
	})

	t.Run("Unmarshal error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("not-json"))
		}))
		defer server.Close()

		client := &AtmosProAPIClient{
			BaseURL:         server.URL,
			BaseAPIEndpoint: "v1",
			APIToken:        "test-token",
			HTTPClient:      &http.Client{},
		}

		params := &schema.StackLockActionParams{
			Method: "POST",
			URL:    server.URL + "/lock",
			Body:   map[string]string{"action": "lock"},
			Out:    make(map[string]interface{}),
		}

		err := client.doStackLockAction(params)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, errUtils.ErrFailedToUnmarshalAPIResponse))
	})
}

// TestHandleAPIResponse tests the handleAPIResponse function.
func TestHandleAPIResponse(t *testing.T) {
	t.Run("Success response", func(t *testing.T) {
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(`{"success": true}`)),
		}

		err := handleAPIResponse(resp, "TestOperation")
		assert.NoError(t, err)
	})

	t.Run("Error response", func(t *testing.T) {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "Internal Server Error"}`)),
		}

		err := handleAPIResponse(resp, "TestOperation")
		assert.Error(t, err)
		assert.True(t, errors.Is(err, errUtils.ErrAPIResponseError))
	})

	t.Run("Read error", func(t *testing.T) {
		// Create a reader that fails.
		failingReader := &failingReader{}
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(failingReader),
		}

		err := handleAPIResponse(resp, "TestOperation")
		assert.Error(t, err)
		assert.True(t, errors.Is(err, errUtils.ErrFailedToReadResponseBody))
	})
}

// failingReader is a reader that always fails.
type failingReader struct{}

func (f *failingReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("read error")
}
