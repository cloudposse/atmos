package pro

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

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
				os.Setenv(k, v)
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
		t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "")
		t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")
		t.Setenv("ATMOS_PRO_WORKSPACE_ID", "")

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
		t.Setenv("ATMOS_PRO_BASE_URL", "")
		t.Setenv("ATMOS_PRO_ENDPOINT", "")
		t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "")
		t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")
		t.Setenv("ATMOS_PRO_WORKSPACE_ID", "")

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
		// Set up TLS mock server for OIDC token request (HTTPS required by SSRF validation).
		oidcServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "Bearer test-request-token", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"value": "github-oidc-token-123"}`))
		}))
		defer oidcServer.Close()
		// Inject TLS-aware client so the test server certificate is trusted.
		oidcHTTPClientOverride = oidcServer.Client()
		defer func() { oidcHTTPClientOverride = nil }()

		// Set up mock server for token exchange
		exchangeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success": true, "data": {"token": "atmos-pro-token-456"}}`))
		}))
		defer exchangeServer.Close()

		// Unset API token to force OIDC flow
		t.Setenv("ATMOS_PRO_TOKEN", "")
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
		// Set up TLS mock server for OIDC token request (HTTPS required by SSRF validation).
		oidcServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"value": "github-oidc-token-123"}`))
		}))
		defer oidcServer.Close()
		// Inject TLS-aware client so the test server certificate is trusted.
		oidcHTTPClientOverride = oidcServer.Client()
		defer func() { oidcHTTPClientOverride = nil }()

		// Unset API token and workspace ID to trigger error
		t.Setenv("ATMOS_PRO_TOKEN", "")
		t.Setenv("ATMOS_PRO_WORKSPACE_ID", "")
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
		t.Setenv("ATMOS_PRO_TOKEN", "")
		// Unset OIDC env vars to trigger failure
		t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "")
		t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")

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
		// Set up TLS mock server for OIDC token request (HTTPS required by SSRF validation).
		oidcServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"value": "github-oidc-token-123"}`))
		}))
		defer oidcServer.Close()
		// Inject TLS-aware client so the test server certificate is trusted.
		oidcHTTPClientOverride = oidcServer.Client()
		defer func() { oidcHTTPClientOverride = nil }()

		// Set up mock server for failed token exchange
		exchangeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"success": false, "error": "unauthorized"}`))
		}))
		defer exchangeServer.Close()

		// Unset API token to force OIDC flow
		t.Setenv("ATMOS_PRO_TOKEN", "")
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
		t.Setenv("ATMOS_PRO_TOKEN", "")
		t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "")
		t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")

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

// TestUserAgent verifies the User-Agent string format.
func TestUserAgent(t *testing.T) {
	ua := userAgent()
	assert.Contains(t, ua, "atmos/")
	assert.Contains(t, ua, runtime.GOOS)
	assert.Contains(t, ua, runtime.GOARCH)
	// Verify format: "atmos/<version> (<os>; <arch>)".
	assert.Regexp(t, `^atmos/\S+ \(\w+; \w+\)$`, ua)
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
		assert.Contains(t, req.Header.Get("User-Agent"), "atmos/")
		assert.Contains(t, req.Header.Get("User-Agent"), runtime.GOOS)
		assert.Contains(t, req.Header.Get("User-Agent"), runtime.GOARCH)
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

func TestHandleAPIResponse_NonJSONErrorResponse(t *testing.T) {
	// Non-JSON body with error status → should return enriched error with troubleshooting link.
	resp := &http.Response{
		StatusCode: http.StatusBadGateway,
		Status:     "502 Bad Gateway",
		Body:       io.NopCloser(bytes.NewBufferString("<html>Bad Gateway</html>")),
	}
	err := handleAPIResponse(resp, "TestOp")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrFailedToUnmarshalAPIResponse))

	hints := cockroachErrors.GetAllHints(err)
	allHints := strings.Join(hints, "\n")
	assert.Contains(t, allHints, "troubleshooting")

	var apiErr *APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
}

func TestHandleAPIResponse_NonJSONSuccessResponse(t *testing.T) {
	// Non-JSON body with success status → should return nil (not an error).
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString("OK")),
	}
	err := handleAPIResponse(resp, "TestOp")
	assert.NoError(t, err)
}

func TestHandleAPIResponse_SuccessHTTPStatusRange(t *testing.T) {
	// 201 Created with valid JSON but no Success field → trusts HTTP status.
	resp := &http.Response{
		StatusCode: http.StatusCreated,
		Body:       io.NopCloser(bytes.NewBufferString(`{"message": "created"}`)),
	}
	err := handleAPIResponse(resp, "TestOp")
	assert.NoError(t, err)
}

func TestBuildProAPIError_WithTraceID(t *testing.T) {
	err := buildProAPIError("TestOp", http.StatusForbidden, dtos.AtmosApiResponse{
		Status:  http.StatusForbidden,
		TraceID: "abc-123-trace",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "abc-123-trace")

	hints := cockroachErrors.GetAllHints(err)
	allHints := strings.Join(hints, "\n")
	assert.Contains(t, allHints, "permissions")
}

func TestBuildProAPIError_HintsPerStatusCode(t *testing.T) {
	tests := []struct {
		name               string
		statusCode         int
		expectedHints      []string // Substrings that must appear in at least one hint.
		unexpectedHint     string   // Substring that must NOT appear in any hint.
		zeroResponseStatus bool     // When true, apiResponse.Status is set to 0 to simulate missing status in JSON body.
	}{
		{
			name:       "403 includes permissions link",
			statusCode: http.StatusForbidden,
			expectedHints: []string{
				"per-repository",
				"atmos-pro.com/docs/learn/permissions",
				"atmos-pro.com/docs/install",
			},
		},
		{
			name:       "401 includes authentication and workflow links",
			statusCode: http.StatusUnauthorized,
			expectedHints: []string{
				"id-token: write",
				"atmos-pro.com/docs/configure/github-workflows",
				"atmos-pro.com/docs/learn/authentication",
			},
		},
		{
			name:       "404 includes install link",
			statusCode: http.StatusNotFound,
			expectedHints: []string{
				"workspace ID",
				"GitHub App",
				"atmos-pro.com/docs/install",
			},
		},
		{
			name:       "500 includes troubleshooting link",
			statusCode: http.StatusInternalServerError,
			expectedHints: []string{
				"server-side error",
				"`trace_id`",
				"atmos-pro.com/docs/learn/troubleshooting",
			},
		},
		{
			name:       "400 includes settings.pro hint",
			statusCode: http.StatusBadRequest,
			expectedHints: []string{
				"settings.pro",
				"atmos-pro.com/docs/configure/atmos",
			},
			unexpectedHint: "drift-detection",
		},
		{
			name:       "401 with missing response status (status=0 in body)",
			statusCode: http.StatusUnauthorized,
			expectedHints: []string{
				"id-token: write",
				"atmos-pro.com/docs/configure/github-workflows",
				"atmos-pro.com/docs/learn/authentication",
			},
			zeroResponseStatus: true,
		},
		{
			name:       "403 with missing response status (status=0 in body)",
			statusCode: http.StatusForbidden,
			expectedHints: []string{
				"per-repository",
				"atmos-pro.com/docs/learn/permissions",
				"atmos-pro.com/docs/install",
			},
			zeroResponseStatus: true,
		},
		{
			name:       "500 with missing response status (status=0 in body)",
			statusCode: http.StatusInternalServerError,
			expectedHints: []string{
				"server-side error",
				"`trace_id`",
				"atmos-pro.com/docs/learn/troubleshooting",
			},
			zeroResponseStatus: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			responseStatus := tt.statusCode
			if tt.zeroResponseStatus {
				responseStatus = 0
			}
			apiResponse := dtos.AtmosApiResponse{
				Status:       responseStatus,
				ErrorMessage: "test error",
				TraceID:      "abc123",
			}

			err := buildProAPIError("TestOp", tt.statusCode, apiResponse)
			assert.Error(t, err)
			assert.True(t, errors.Is(err, errUtils.ErrAPIResponseError))

			hints := cockroachErrors.GetAllHints(err)
			allHints := strings.Join(hints, "\n")
			for _, expected := range tt.expectedHints {
				assert.Contains(t, allHints, expected, "hints should contain substring: %s", expected)
			}
			if tt.unexpectedHint != "" {
				assert.NotContains(t, allHints, tt.unexpectedHint, "hints should not contain: %s", tt.unexpectedHint)
			}
		})
	}
}

func TestHandleAPIResponse_NonJSON_IncludesTroubleshootingHint(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Status:     "403 Forbidden",
		Body:       io.NopCloser(bytes.NewBufferString(`not json`)),
	}

	err := handleAPIResponse(resp, "TestOp")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrFailedToUnmarshalAPIResponse))

	hints := cockroachErrors.GetAllHints(err)
	allHints := strings.Join(hints, "\n")
	assert.Contains(t, allHints, "atmos-pro.com/docs/learn/troubleshooting")
}

// TestBuildProAPIError_400DriftDetection covers the user-facing rendering
// for the canonical drift-detection 400 case: bullet list, drift-detection
// hint, and trace_id preserved for support.
func TestBuildProAPIError_400DriftDetection(t *testing.T) {
	apiResponse := dtos.AtmosApiResponse{
		Status:       http.StatusBadRequest,
		ErrorTag:     "DriftDetectionValidationError",
		ErrorMessage: "Drift detection validation failed: A; B",
		Data: &dtos.AtmosApiResponseData{
			ValidationErrors: []string{"A", "B"},
		},
		TraceID: "abc-trace",
	}

	err := buildProAPIError("UploadInstances", http.StatusBadRequest, apiResponse)
	require.Error(t, err)
	require.True(t, errors.Is(err, errUtils.ErrAPIResponseError))

	msg := err.Error()
	assert.Contains(t, msg, "- A", "bullet for first validation error")
	assert.Contains(t, msg, "- B", "bullet for second validation error")
	assert.Contains(t, msg, "trace_id: abc-trace", "trace_id preserved on 4xx for support")

	hints := cockroachErrors.GetAllHints(err)
	allHints := strings.Join(hints, "\n")
	assert.Contains(t, allHints, "settings.pro")
	assert.Contains(t, allHints, "atmos-pro.com/docs/configure/drift-detection")
}

// TestBuildProAPIError_400LegacyErrorField verifies the DTO falls back to the
// legacy `error` field when `errorMessage` isn't populated.
func TestBuildProAPIError_400LegacyErrorField(t *testing.T) {
	apiResponse := dtos.AtmosApiResponse{
		Status: http.StatusBadRequest,
		Error:  "Bad input",
	}

	err := buildProAPIError("UploadInstances", http.StatusBadRequest, apiResponse)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Bad input")
}

// TestHandleAPIResponse_400IsNotRetried verifies that a 400 is non-retryable
// and that the user-facing error contains the server's message and bullets,
// without the redundant "HTTP 400:" prefix.
func TestHandleAPIResponse_400IsNotRetried(t *testing.T) {
	body := `{
		"success": false,
		"status": 400,
		"errorTag": "DriftDetectionValidationError",
		"errorMessage": "Drift detection validation failed: A; B",
		"data": {"validationErrors": ["A", "B"]},
		"traceId": "abc-trace"
	}`

	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Status:     "400 Bad Request",
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}

	err := handleAPIResponse(resp, "UploadInstances")
	require.Error(t, err)

	var apiErr *APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.False(t, apiErr.IsRetryable(), "4xx must not be retryable")

	msg := apiErr.Error()
	assert.Contains(t, msg, "UploadInstances:", "operation prefix")
	assert.NotContains(t, msg, "HTTP 400:", "HTTP <code> prefix dropped on 4xx")
	assert.Contains(t, msg, "- A")
	assert.Contains(t, msg, "- B")
	assert.Contains(t, msg, "trace_id: abc-trace")
}

// TestHandleAPIResponse_500KeepsHTTPPrefixAndTraceID verifies 5xx responses
// retain the diagnostic "HTTP <code>:" prefix and the trace_id.
func TestHandleAPIResponse_500KeepsHTTPPrefixAndTraceID(t *testing.T) {
	body := `{"success":false,"status":500,"errorMessage":"boom","traceId":"srv-trace"}`

	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Status:     "500 Internal Server Error",
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}

	err := handleAPIResponse(resp, "UploadInstances")
	require.Error(t, err)

	var apiErr *APIError
	require.True(t, errors.As(err, &apiErr))
	assert.True(t, apiErr.IsRetryable(), "5xx must be retryable")

	msg := apiErr.Error()
	assert.Contains(t, msg, "HTTP 500:", "diagnostic prefix preserved on 5xx")
	assert.Contains(t, msg, "boom")
	assert.Contains(t, msg, "trace_id: srv-trace")

	hints := cockroachErrors.GetAllHints(err)
	allHints := strings.Join(hints, "\n")
	assert.Contains(t, allHints, "server-side error")
}

// TestRenderValidationErrors covers the dedupe/bullet helper directly so the
// formatting is exercised without rebuilding a full API error.
func TestRenderValidationErrors(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		errors   []string
		contains []string
		excludes []string
	}{
		{
			name:     "strips trailing concatenation when message includes the errors",
			message:  "Drift detection validation failed: A; B",
			errors:   []string{"A", "B"},
			contains: []string{"Drift detection validation failed", "- A", "- B"},
			excludes: []string{"failed: A; B"},
		},
		{
			name:     "appends bullets when message is just a headline",
			message:  "Validation failed",
			errors:   []string{"foo", "bar"},
			contains: []string{"Validation failed", "- foo", "- bar"},
		},
		{
			name:     "leaves message intact when errors not present in tail",
			message:  "Some other message",
			errors:   []string{"completely unrelated"},
			contains: []string{"Some other message", "- completely unrelated"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderValidationErrors(tt.message, tt.errors)
			for _, s := range tt.contains {
				assert.Contains(t, got, s)
			}
			for _, s := range tt.excludes {
				assert.NotContains(t, got, s)
			}
		})
	}
}

// TestIsDriftDetectionError covers both the structured tag path and the
// fallback substring matcher.
func TestIsDriftDetectionError(t *testing.T) {
	tests := []struct {
		name string
		in   *dtos.AtmosApiResponse
		want bool
	}{
		{"matches by errorTag", &dtos.AtmosApiResponse{ErrorTag: "DriftDetectionValidationError"}, true},
		{"matches by message: drift detection", &dtos.AtmosApiResponse{ErrorMessage: "Drift detection failed"}, true},
		{"matches by message: remediate workflow", &dtos.AtmosApiResponse{ErrorMessage: "Missing remediate workflow"}, true},
		{"matches by message: detect workflow", &dtos.AtmosApiResponse{ErrorMessage: "Missing detect workflow"}, true},
		{"matches by legacy error field", &dtos.AtmosApiResponse{Error: "drift detection failed"}, true},
		{"no match for unrelated 400", &dtos.AtmosApiResponse{ErrorMessage: "Bad input"}, false},
		{"no match for empty response", &dtos.AtmosApiResponse{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isDriftDetectionError(tt.in))
		})
	}
}

// failingReader is a reader that always fails.
type failingReader struct{}

func (f *failingReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("read error")
}
