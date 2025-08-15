package pro

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

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
		os.Setenv("ATMOS_PRO_BASE_URL", "https://api.atmos.example.com")
		os.Setenv("ATMOS_PRO_ENDPOINT", "v1")
		os.Setenv("ATMOS_PRO_TOKEN", "direct-api-token")
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
		os.Setenv("ATMOS_PRO_TOKEN", "direct-api-token")
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
		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", oidcServer.URL+"?token=dummy")
		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "test-request-token")
		os.Setenv("ATMOS_PRO_WORKSPACE_ID", "test-workspace")
		os.Setenv("ATMOS_PRO_BASE_URL", exchangeServer.URL)

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
		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", oidcServer.URL+"?token=dummy")
		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "test-request-token")

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
		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", oidcServer.URL+"?token=dummy")
		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "test-request-token")
		os.Setenv("ATMOS_PRO_WORKSPACE_ID", "test-workspace")
		os.Setenv("ATMOS_PRO_BASE_URL", exchangeServer.URL)

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
