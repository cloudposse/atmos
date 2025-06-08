package pro

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/spf13/viper"
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

func TestNewAtmosProAPIClientFromEnv(t *testing.T) {
	mockLogger, err := logger.NewLogger("test", "/dev/stdout")
	assert.NoError(t, err)

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
		os.Setenv("ATMOS_PRO_TOKEN", "direct-api-token")
		os.Setenv("ATMOS_PRO_BASE_URL", "https://api.atmos.example.com")
		os.Setenv("ATMOS_PRO_ENDPOINT", "v1")
		// Unset OIDC env vars to ensure they're not used
		os.Unsetenv("ACTIONS_ID_TOKEN_REQUEST_URL")
		os.Unsetenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
		os.Unsetenv("ATMOS_PRO_WORKSPACE_ID")

		viper.Reset()
		viper.AutomaticEnv()

		client, err := NewAtmosProAPIClientFromEnv(mockLogger)
		assert.NoError(t, err)
		assert.NotNil(t, client)
		assert.Equal(t, "direct-api-token", client.APIToken)
		assert.Equal(t, "https://api.atmos.example.com", client.BaseURL)
		assert.Equal(t, "v1", client.BaseAPIEndpoint)
	})

	t.Run("api token set with defaults", func(t *testing.T) {
		os.Setenv("ATMOS_PRO_TOKEN", "direct-api-token")
		// Unset custom URLs to test defaults
		os.Unsetenv("ATMOS_PRO_BASE_URL")
		os.Unsetenv("ATMOS_PRO_ENDPOINT")
		os.Unsetenv("ACTIONS_ID_TOKEN_REQUEST_URL")
		os.Unsetenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
		os.Unsetenv("ATMOS_PRO_WORKSPACE_ID")

		viper.Reset()
		viper.AutomaticEnv()

		client, err := NewAtmosProAPIClientFromEnv(mockLogger)
		assert.NoError(t, err)
		assert.NotNil(t, client)
		assert.Equal(t, "direct-api-token", client.APIToken)
		// Should use default values
		assert.NotEmpty(t, client.BaseURL)
		assert.NotEmpty(t, client.BaseAPIEndpoint)
	})

	t.Run("successful OIDC flow", func(t *testing.T) {
		// Set up GitHub OIDC mock server
		githubOIDCServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "Bearer github-request-token", r.Header.Get("Authorization"))
			assert.Contains(t, r.URL.RawQuery, "audience=atmos-pro.com")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"value": "github-oidc-token"}`))
		}))
		defer githubOIDCServer.Close()

		// Set up Atmos API mock server
		atmosAPIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/auth/github-oidc" {
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				assert.Contains(t, string(body), "github-oidc-token")
				assert.Contains(t, string(body), "test-workspace")

				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{
					"success": true,
					"data": {"token": "exchanged-atmos-token"}
				}`))
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer atmosAPIServer.Close()

		// Set environment variables
		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", githubOIDCServer.URL+"?token=dummy")
		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "github-request-token")
		os.Setenv("ATMOS_PRO_WORKSPACE_ID", "test-workspace")
		os.Setenv("ATMOS_PRO_BASE_URL", atmosAPIServer.URL)
		os.Setenv("ATMOS_PRO_ENDPOINT", "api")
		os.Unsetenv("ATMOS_PRO_TOKEN")

		viper.Reset()
		viper.AutomaticEnv()

		client, err := NewAtmosProAPIClientFromEnv(mockLogger)
		assert.NoError(t, err)
		assert.NotNil(t, client)
		if client != nil {
			assert.Equal(t, "exchanged-atmos-token", client.APIToken)
			assert.Equal(t, atmosAPIServer.URL, client.BaseURL)
			assert.Equal(t, "api", client.BaseAPIEndpoint)
		}
	})

	t.Run("missing workspace ID for OIDC", func(t *testing.T) {
		githubOIDCServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"value": "github-oidc-token"}`))
		}))
		defer githubOIDCServer.Close()

		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", githubOIDCServer.URL+"?token=dummy")
		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "github-request-token")
		os.Unsetenv("ATMOS_PRO_WORKSPACE_ID") // Missing workspace ID
		os.Unsetenv("ATMOS_PRO_TOKEN")

		viper.Reset()
		viper.AutomaticEnv()

		client, err := NewAtmosProAPIClientFromEnv(mockLogger)
		assert.Error(t, err)
		assert.Nil(t, client)
		assert.ErrorIs(t, err, ErrOIDCWorkspaceIDRequired)
		assert.Contains(t, err.Error(), "workspace ID environment variable is required")
	})

	t.Run("GitHub OIDC token fetch fails", func(t *testing.T) {
		githubOIDCServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error": "unauthorized"}`))
		}))
		defer githubOIDCServer.Close()

		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", githubOIDCServer.URL+"?token=dummy")
		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "invalid-github-token")
		os.Setenv("ATMOS_PRO_WORKSPACE_ID", "test-workspace")
		os.Unsetenv("ATMOS_PRO_TOKEN")

		viper.Reset()
		viper.AutomaticEnv()

		client, err := NewAtmosProAPIClientFromEnv(mockLogger)
		assert.Error(t, err)
		assert.Nil(t, client)
		assert.ErrorIs(t, err, ErrOIDCAuthFailedNoToken)
	})

	t.Run("OIDC token exchange fails", func(t *testing.T) {
		githubOIDCServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"value": "github-oidc-token"}`))
		}))
		defer githubOIDCServer.Close()

		atmosAPIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/auth/github-oidc" {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error": "invalid oidc token"}`))
			}
		}))
		defer atmosAPIServer.Close()

		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", githubOIDCServer.URL+"?token=dummy")
		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "github-request-token")
		os.Setenv("ATMOS_PRO_WORKSPACE_ID", "invalid-workspace")
		os.Setenv("ATMOS_PRO_BASE_URL", atmosAPIServer.URL)
		os.Setenv("ATMOS_PRO_ENDPOINT", "api")
		os.Unsetenv("ATMOS_PRO_TOKEN")

		viper.Reset()
		viper.AutomaticEnv()

		client, err := NewAtmosProAPIClientFromEnv(mockLogger)
		assert.Error(t, err)
		assert.Nil(t, client)
		assert.ErrorIs(t, err, ErrOIDCTokenExchangeFailed)
	})

	t.Run("no GitHub Actions environment", func(t *testing.T) {
		// Unset all GitHub Actions environment variables
		os.Unsetenv("ACTIONS_ID_TOKEN_REQUEST_URL")
		os.Unsetenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
		os.Unsetenv("ATMOS_PRO_TOKEN")
		os.Setenv("ATMOS_PRO_WORKSPACE_ID", "test-workspace")

		viper.Reset()
		viper.AutomaticEnv()

		client, err := NewAtmosProAPIClientFromEnv(mockLogger)
		assert.Error(t, err)
		assert.Nil(t, client)
		assert.ErrorIs(t, err, ErrOIDCAuthFailedNoToken)
	})
}
