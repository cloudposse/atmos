package azure

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewOIDCProvider(t *testing.T) {
	tests := []struct {
		name          string
		providerName  string
		config        *schema.Provider
		expectError   bool
		errorType     error
		checkProvider func(*testing.T, *oidcProvider)
	}{
		{
			name:         "valid OIDC provider config with all fields",
			providerName: "azure-oidc",
			config: &schema.Provider{
				Kind: "azure/oidc",
				Spec: map[string]interface{}{
					"tenant_id":       "tenant-123",
					"client_id":       "client-456",
					"subscription_id": "sub-789",
					"location":        "eastus",
					"audience":        "api://AzureADTokenExchange",
					"token_file_path": "/path/to/token",
				},
			},
			expectError: false,
			checkProvider: func(t *testing.T, p *oidcProvider) {
				assert.Equal(t, "tenant-123", p.tenantID)
				assert.Equal(t, "client-456", p.clientID)
				assert.Equal(t, "sub-789", p.subscriptionID)
				assert.Equal(t, "eastus", p.location)
				assert.Equal(t, "api://AzureADTokenExchange", p.audience)
				assert.Equal(t, "/path/to/token", p.tokenFilePath)
			},
		},
		{
			name:         "valid config with only required fields",
			providerName: "azure-oidc",
			config: &schema.Provider{
				Kind: "azure/oidc",
				Spec: map[string]interface{}{
					"tenant_id": "tenant-123",
					"client_id": "client-456",
				},
			},
			expectError: false,
			checkProvider: func(t *testing.T, p *oidcProvider) {
				assert.Equal(t, "tenant-123", p.tenantID)
				assert.Equal(t, "client-456", p.clientID)
				assert.Equal(t, "", p.subscriptionID)
				assert.Equal(t, "", p.location)
				assert.Equal(t, "", p.audience)
				assert.Equal(t, "", p.tokenFilePath)
			},
		},
		{
			name:         "missing tenant ID",
			providerName: "azure-oidc",
			config: &schema.Provider{
				Kind: "azure/oidc",
				Spec: map[string]interface{}{
					"client_id": "client-456",
				},
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderConfig,
		},
		{
			name:         "missing client ID",
			providerName: "azure-oidc",
			config: &schema.Provider{
				Kind: "azure/oidc",
				Spec: map[string]interface{}{
					"tenant_id": "tenant-123",
				},
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderConfig,
		},
		{
			name:         "nil spec",
			providerName: "azure-oidc",
			config: &schema.Provider{
				Kind: "azure/oidc",
				Spec: nil,
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderConfig,
		},
		{
			name:         "empty spec",
			providerName: "azure-oidc",
			config: &schema.Provider{
				Kind: "azure/oidc",
				Spec: map[string]interface{}{},
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderConfig,
		},
		{
			name:         "nil config",
			providerName: "azure-oidc",
			config:       nil,
			expectError:  true,
			errorType:    errUtils.ErrInvalidProviderConfig,
		},
		{
			name:         "wrong provider kind",
			providerName: "azure-oidc",
			config: &schema.Provider{
				Kind: "azure/cli",
				Spec: map[string]interface{}{
					"tenant_id": "tenant-123",
					"client_id": "client-456",
				},
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderKind,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewOIDCProvider(tt.providerName, tt.config)

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, provider)
				if tt.errorType != nil {
					assert.ErrorIs(t, err, tt.errorType)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, provider)
			assert.Equal(t, tt.providerName, provider.Name())
			assert.Equal(t, "azure/oidc", provider.Kind())

			if tt.checkProvider != nil {
				tt.checkProvider(t, provider)
			}
		})
	}
}

func TestOIDCProvider_Kind(t *testing.T) {
	provider := &oidcProvider{}
	assert.Equal(t, "azure/oidc", provider.Kind())
}

func TestOIDCProvider_Name(t *testing.T) {
	tests := []struct {
		name     string
		provider *oidcProvider
		expected string
	}{
		{
			name:     "provider with name",
			provider: &oidcProvider{name: "my-azure-oidc"},
			expected: "my-azure-oidc",
		},
		{
			name:     "provider with empty name",
			provider: &oidcProvider{name: ""},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.provider.Name())
		})
	}
}

func TestOIDCProvider_PreAuthenticate(t *testing.T) {
	provider := &oidcProvider{
		name: "test-oidc",
		config: &schema.Provider{
			Kind: "azure/oidc",
			Spec: map[string]interface{}{
				"tenant_id": "tenant-123",
				"client_id": "client-456",
			},
		},
		tenantID: "tenant-123",
		clientID: "client-456",
	}

	// PreAuthenticate should be a no-op and always return nil.
	err := provider.PreAuthenticate(nil)
	assert.NoError(t, err)
}

func TestOIDCProvider_Validate(t *testing.T) {
	tests := []struct {
		name        string
		provider    *oidcProvider
		expectError bool
		errorType   error
	}{
		{
			name: "valid provider",
			provider: &oidcProvider{
				tenantID: "tenant-123",
				clientID: "client-456",
			},
			expectError: false,
		},
		{
			name: "missing tenant ID",
			provider: &oidcProvider{
				tenantID: "",
				clientID: "client-456",
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderConfig,
		},
		{
			name: "missing client ID",
			provider: &oidcProvider{
				tenantID: "tenant-123",
				clientID: "",
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderConfig,
		},
		{
			name: "both fields missing",
			provider: &oidcProvider{
				tenantID: "",
				clientID: "",
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.provider.Validate()

			if tt.expectError {
				require.Error(t, err)
				if tt.errorType != nil {
					assert.ErrorIs(t, err, tt.errorType)
				}
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestOIDCProvider_Environment(t *testing.T) {
	tests := []struct {
		name        string
		provider    *oidcProvider
		expectedEnv map[string]string
	}{
		{
			name: "all fields present",
			provider: &oidcProvider{
				tenantID:       "tenant-123",
				clientID:       "client-456",
				subscriptionID: "sub-789",
				location:       "eastus",
			},
			expectedEnv: map[string]string{
				"AZURE_TENANT_ID":       "tenant-123",
				"AZURE_CLIENT_ID":       "client-456",
				"AZURE_SUBSCRIPTION_ID": "sub-789",
				"AZURE_LOCATION":        "eastus",
			},
		},
		{
			name: "only required fields",
			provider: &oidcProvider{
				tenantID:       "tenant-123",
				clientID:       "client-456",
				subscriptionID: "",
				location:       "",
			},
			expectedEnv: map[string]string{
				"AZURE_TENANT_ID": "tenant-123",
				"AZURE_CLIENT_ID": "client-456",
			},
		},
		{
			name: "empty fields",
			provider: &oidcProvider{
				tenantID:       "",
				clientID:       "",
				subscriptionID: "",
				location:       "",
			},
			expectedEnv: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := tt.provider.Environment()
			require.NoError(t, err)
			assert.Equal(t, tt.expectedEnv, env)
		})
	}
}

func TestOIDCProvider_PrepareEnvironment(t *testing.T) {
	tests := []struct {
		name             string
		provider         *oidcProvider
		inputEnv         map[string]string
		expectedContains map[string]string
		expectedMissing  []string
	}{
		{
			name: "basic environment preparation for OIDC",
			provider: &oidcProvider{
				tenantID:       "tenant-123",
				clientID:       "client-456",
				subscriptionID: "sub-789",
				location:       "eastus",
			},
			inputEnv: map[string]string{
				"HOME": "/home/user",
				"PATH": "/usr/bin",
			},
			expectedContains: map[string]string{
				"HOME":                  "/home/user",
				"PATH":                  "/usr/bin",
				"AZURE_SUBSCRIPTION_ID": "sub-789",
				"ARM_SUBSCRIPTION_ID":   "sub-789",
				"AZURE_TENANT_ID":       "tenant-123",
				"ARM_TENANT_ID":         "tenant-123",
				"AZURE_LOCATION":        "eastus",
				"ARM_LOCATION":          "eastus",
				"ARM_USE_OIDC":          "true",
				"ARM_CLIENT_ID":         "client-456",
			},
			expectedMissing: []string{
				"ARM_USE_CLI", // OIDC should not use CLI auth.
			},
		},
		{
			name: "clears conflicting Azure credentials",
			provider: &oidcProvider{
				tenantID:       "tenant-123",
				clientID:       "client-456",
				subscriptionID: "sub-789",
			},
			inputEnv: map[string]string{
				"AZURE_CLIENT_SECRET":            "conflicting-secret",
				"AZURE_CLIENT_CERTIFICATE_PATH":  "/path/to/cert",
				"ARM_CLIENT_SECRET":              "conflicting-arm-secret",
				"ARM_USE_CLI":                    "true",
				"AWS_ACCESS_KEY_ID":              "AKIAIOSFODNN7EXAMPLE",
				"GOOGLE_APPLICATION_CREDENTIALS": "/path/to/gcp/creds.json",
				"HOME":                           "/home/user",
			},
			expectedContains: map[string]string{
				"HOME":                           "/home/user",
				"AZURE_SUBSCRIPTION_ID":          "sub-789",
				"ARM_SUBSCRIPTION_ID":            "sub-789",
				"AZURE_TENANT_ID":                "tenant-123",
				"ARM_TENANT_ID":                  "tenant-123",
				"ARM_USE_OIDC":                   "true",
				"ARM_CLIENT_ID":                  "client-456",
				"AWS_ACCESS_KEY_ID":              "AKIAIOSFODNN7EXAMPLE",
				"GOOGLE_APPLICATION_CREDENTIALS": "/path/to/gcp/creds.json",
			},
			expectedMissing: []string{
				"AZURE_CLIENT_SECRET",
				"AZURE_CLIENT_CERTIFICATE_PATH",
				"ARM_CLIENT_SECRET",
				"ARM_USE_CLI",
			},
		},
		{
			name: "sets token file path from config",
			provider: &oidcProvider{
				tenantID:       "tenant-123",
				clientID:       "client-456",
				subscriptionID: "sub-789",
				tokenFilePath:  "/custom/token/path",
			},
			inputEnv: map[string]string{},
			expectedContains: map[string]string{
				"ARM_USE_OIDC":               "true",
				"ARM_CLIENT_ID":              "client-456",
				"AZURE_FEDERATED_TOKEN_FILE": "/custom/token/path",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := tt.provider.PrepareEnvironment(ctx, tt.inputEnv)

			require.NoError(t, err)

			// Check expected variables are present with correct values.
			for key, expectedValue := range tt.expectedContains {
				assert.Equal(t, expectedValue, result[key], "Expected %s=%s", key, expectedValue)
			}

			// Check unwanted variables are missing.
			for _, key := range tt.expectedMissing {
				_, exists := result[key]
				assert.False(t, exists, "Expected %s to be missing", key)
			}

			// Verify ARM_USE_OIDC is always set to "true".
			assert.Equal(t, "true", result["ARM_USE_OIDC"], "ARM_USE_OIDC should always be true")
		})
	}
}

func TestOIDCProvider_Logout(t *testing.T) {
	provider := &oidcProvider{
		name:     "test-oidc",
		tenantID: "tenant-123",
		clientID: "client-456",
	}

	// Logout should return ErrLogoutNotSupported for OIDC provider.
	ctx := context.Background()
	err := provider.Logout(ctx)
	assert.ErrorIs(t, err, errUtils.ErrLogoutNotSupported)
}

func TestOIDCProvider_Paths(t *testing.T) {
	provider := &oidcProvider{
		name:     "test-oidc",
		tenantID: "tenant-123",
		clientID: "client-456",
	}

	// OIDC provider should return empty paths.
	paths, err := provider.Paths()
	require.NoError(t, err)
	assert.Empty(t, paths, "OIDC provider should not manage files")
}

func TestOIDCProvider_GetFilesDisplayPath(t *testing.T) {
	provider := &oidcProvider{
		name:     "test-oidc",
		tenantID: "tenant-123",
		clientID: "client-456",
	}

	// GetFilesDisplayPath should return empty string for OIDC provider.
	path := provider.GetFilesDisplayPath()
	assert.Equal(t, "", path, "OIDC provider should not manage files")
}

func TestOIDCProvider_ReadTokenFromFile(t *testing.T) {
	// Create a temporary token file.
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "token")

	tests := []struct {
		name        string
		setup       func()
		path        string
		expectError bool
		expectToken string
	}{
		{
			name: "valid token file",
			setup: func() {
				err := os.WriteFile(tokenPath, []byte("eyJhbGciOiJSUzI1NiJ9.test.signature"), 0o600)
				require.NoError(t, err)
			},
			path:        tokenPath,
			expectError: false,
			expectToken: "eyJhbGciOiJSUzI1NiJ9.test.signature",
		},
		{
			name: "token file with whitespace",
			setup: func() {
				err := os.WriteFile(tokenPath, []byte("  eyJhbGciOiJSUzI1NiJ9.test.signature  \n"), 0o600)
				require.NoError(t, err)
			},
			path:        tokenPath,
			expectError: false,
			expectToken: "eyJhbGciOiJSUzI1NiJ9.test.signature",
		},
		{
			name: "empty token file",
			setup: func() {
				err := os.WriteFile(tokenPath, []byte(""), 0o600)
				require.NoError(t, err)
			},
			path:        tokenPath,
			expectError: true,
		},
		{
			name: "whitespace only token file",
			setup: func() {
				err := os.WriteFile(tokenPath, []byte("   \n\t  "), 0o600)
				require.NoError(t, err)
			},
			path:        tokenPath,
			expectError: true,
		},
		{
			name:        "nonexistent file",
			setup:       func() {},
			path:        filepath.Join(tmpDir, "nonexistent"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			provider := &oidcProvider{
				name:     "test-oidc",
				tenantID: "tenant-123",
				clientID: "client-456",
			}

			token, err := provider.readTokenFromFile(tt.path)

			if tt.expectError {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectToken, token)
		})
	}
}

func TestOIDCProvider_IsGitHubActions(t *testing.T) {
	provider := &oidcProvider{
		name:     "test-oidc",
		tenantID: "tenant-123",
		clientID: "client-456",
	}

	tests := []struct {
		name     string
		envValue string
		unsetEnv bool
		expected bool
	}{
		{
			name:     "GITHUB_ACTIONS not set",
			unsetEnv: true,
			expected: false,
		},
		{
			name:     "GITHUB_ACTIONS set to true",
			envValue: "true",
			expected: true,
		},
		{
			name:     "GITHUB_ACTIONS set to false",
			envValue: "false",
			expected: false,
		},
		{
			name:     "GITHUB_ACTIONS set to 1",
			envValue: "1",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.unsetEnv {
				t.Setenv("GITHUB_ACTIONS", "")
				// Clear the env var entirely.
				os.Unsetenv("GITHUB_ACTIONS")
			} else {
				t.Setenv("GITHUB_ACTIONS", tt.envValue)
			}

			assert.Equal(t, tt.expected, provider.isGitHubActions())
		})
	}
}

func TestOIDCProvider_ExchangeToken(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse interface{}
		statusCode     int
		expectError    bool
		checkToken     func(*testing.T, *tokenResponse)
	}{
		{
			name: "successful token exchange",
			serverResponse: tokenResponse{
				AccessToken: "azure-access-token-123",
				TokenType:   "Bearer",
				ExpiresIn:   3600,
			},
			statusCode:  http.StatusOK,
			expectError: false,
			checkToken: func(t *testing.T, resp *tokenResponse) {
				assert.Equal(t, "azure-access-token-123", resp.AccessToken)
				assert.Equal(t, "Bearer", resp.TokenType)
				assert.Equal(t, int64(3600), resp.ExpiresIn)
			},
		},
		{
			name:           "invalid JSON response",
			serverResponse: "not json",
			statusCode:     http.StatusOK,
			expectError:    true,
		},
		{
			name: "empty access token in response",
			serverResponse: tokenResponse{
				AccessToken: "",
				TokenType:   "Bearer",
				ExpiresIn:   3600,
			},
			statusCode:  http.StatusOK,
			expectError: true,
		},
		{
			name:           "server error",
			serverResponse: map[string]string{"error": "invalid_client"},
			statusCode:     http.StatusBadRequest,
			expectError:    true,
		},
		{
			name:           "unauthorized",
			serverResponse: map[string]string{"error": "unauthorized"},
			statusCode:     http.StatusUnauthorized,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request.
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

				// Verify form data.
				err := r.ParseForm()
				require.NoError(t, err)
				assert.Equal(t, "client_credentials", r.FormValue("grant_type"))
				assert.Equal(t, "client-456", r.FormValue("client_id"))
				assert.Equal(t, clientAssertionTypeJWT, r.FormValue("client_assertion_type"))
				assert.Equal(t, "test-federated-token", r.FormValue("client_assertion"))
				assert.Equal(t, azureManagementScope, r.FormValue("scope"))

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)

				switch v := tt.serverResponse.(type) {
				case string:
					_, _ = w.Write([]byte(v))
				default:
					_ = json.NewEncoder(w).Encode(v)
				}
			}))
			defer server.Close()

			// Create provider with test server URL as token endpoint.
			provider := &oidcProvider{
				name:          "test-oidc",
				tenantID:      "tenant-123",
				clientID:      "client-456",
				tokenEndpoint: server.URL,
			}

			// Call exchangeToken.
			ctx := context.Background()
			resp, err := provider.exchangeToken(ctx, "test-federated-token")

			if tt.expectError {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, resp)

			if tt.checkToken != nil {
				tt.checkToken(t, resp)
			}
		})
	}
}

func TestOIDCProvider_FetchGitHubActionsToken(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse interface{}
		statusCode     int
		audience       string
		expectError    bool
		expectedToken  string
	}{
		{
			name: "successful token fetch",
			serverResponse: map[string]string{
				"value": "github-oidc-token-abc123",
			},
			statusCode:    http.StatusOK,
			expectError:   false,
			expectedToken: "github-oidc-token-abc123",
		},
		{
			name: "successful token fetch with custom audience",
			serverResponse: map[string]string{
				"value": "github-oidc-token-custom",
			},
			statusCode:    http.StatusOK,
			audience:      "custom-audience",
			expectError:   false,
			expectedToken: "github-oidc-token-custom",
		},
		{
			name:           "empty token in response",
			serverResponse: map[string]string{"value": ""},
			statusCode:     http.StatusOK,
			expectError:    true,
		},
		{
			name:           "server error",
			serverResponse: map[string]string{"error": "forbidden"},
			statusCode:     http.StatusForbidden,
			expectError:    true,
		},
		{
			name:           "invalid JSON response",
			serverResponse: "not json",
			statusCode:     http.StatusOK,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request.
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "application/json", r.Header.Get("Accept"))
				assert.Equal(t, "bearer test-request-token", r.Header.Get("Authorization"))

				// Verify audience parameter.
				expectedAudience := tt.audience
				if expectedAudience == "" {
					expectedAudience = "api://AzureADTokenExchange"
				}
				assert.Equal(t, expectedAudience, r.URL.Query().Get("audience"))

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)

				switch v := tt.serverResponse.(type) {
				case string:
					_, _ = w.Write([]byte(v))
				default:
					_ = json.NewEncoder(w).Encode(v)
				}
			}))
			defer server.Close()

			// Set up environment variables for GitHub Actions.
			t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", server.URL)
			t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "test-request-token")

			// Create provider.
			provider := &oidcProvider{
				name:     "test-oidc",
				tenantID: "tenant-123",
				clientID: "client-456",
				audience: tt.audience,
			}

			// Call fetchGitHubActionsToken.
			token, err := provider.fetchGitHubActionsToken()

			if tt.expectError {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedToken, token)
		})
	}
}

func TestOIDCProvider_FetchGitHubActionsToken_MissingEnvVars(t *testing.T) {
	provider := &oidcProvider{
		name:     "test-oidc",
		tenantID: "tenant-123",
		clientID: "client-456",
	}

	// Test missing ACTIONS_ID_TOKEN_REQUEST_URL.
	t.Run("missing request URL", func(t *testing.T) {
		t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "")
		os.Unsetenv("ACTIONS_ID_TOKEN_REQUEST_URL")
		t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "test-token")

		_, err := provider.fetchGitHubActionsToken()
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
		assert.Contains(t, err.Error(), "ACTIONS_ID_TOKEN_REQUEST_URL")
	})

	// Test missing ACTIONS_ID_TOKEN_REQUEST_TOKEN.
	t.Run("missing request token", func(t *testing.T) {
		t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "https://example.com")
		t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")
		os.Unsetenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")

		_, err := provider.fetchGitHubActionsToken()
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
		assert.Contains(t, err.Error(), "ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	})
}

func TestOIDCProvider_Authenticate(t *testing.T) {
	// Create a test token file.
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "token")
	err := os.WriteFile(tokenPath, []byte("test-federated-token"), 0o600)
	require.NoError(t, err)

	tests := []struct {
		name             string
		serverResponse   interface{}
		statusCode       int
		expectError      bool
		checkCredentials func(*testing.T, *authTypes.AzureCredentials)
	}{
		{
			name: "successful authentication",
			serverResponse: tokenResponse{
				AccessToken: "azure-access-token-xyz",
				TokenType:   "Bearer",
				ExpiresIn:   7200,
			},
			statusCode:  http.StatusOK,
			expectError: false,
			checkCredentials: func(t *testing.T, creds *authTypes.AzureCredentials) {
				assert.Equal(t, "azure-access-token-xyz", creds.AccessToken)
				assert.Equal(t, "Bearer", creds.TokenType)
				assert.Equal(t, "tenant-123", creds.TenantID)
				assert.Equal(t, "sub-789", creds.SubscriptionID)
				assert.NotEmpty(t, creds.Expiration)
			},
		},
		{
			name:           "authentication failure - server error",
			serverResponse: map[string]string{"error": "invalid_grant"},
			statusCode:     http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				_ = json.NewEncoder(w).Encode(tt.serverResponse)
			}))
			defer server.Close()

			// Create provider with test server and token file.
			provider := &oidcProvider{
				name:           "test-oidc",
				tenantID:       "tenant-123",
				clientID:       "client-456",
				subscriptionID: "sub-789",
				location:       "eastus",
				tokenFilePath:  tokenPath,
				tokenEndpoint:  server.URL,
			}

			// Call Authenticate.
			ctx := context.Background()
			creds, err := provider.Authenticate(ctx)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, creds)

			azureCreds, ok := creds.(*authTypes.AzureCredentials)
			require.True(t, ok, "expected AzureCredentials type")

			if tt.checkCredentials != nil {
				tt.checkCredentials(t, azureCreds)
			}
		})
	}
}

func TestOIDCProvider_GetHTTPClient(t *testing.T) {
	t.Run("returns default client when none injected", func(t *testing.T) {
		provider := &oidcProvider{
			name:       "test-oidc",
			tenantID:   "tenant-123",
			clientID:   "client-456",
			httpClient: nil,
		}

		client := provider.getHTTPClient()
		require.NotNil(t, client)
	})

	t.Run("returns injected client when provided", func(t *testing.T) {
		mockClient := &http.Client{Timeout: 5 * time.Second}
		provider := &oidcProvider{
			name:       "test-oidc",
			tenantID:   "tenant-123",
			clientID:   "client-456",
			httpClient: mockClient,
		}

		client := provider.getHTTPClient()
		require.NotNil(t, client)
		assert.Equal(t, mockClient, client)
	})
}

func TestOIDCProvider_GetTokenEndpoint(t *testing.T) {
	t.Run("returns default Azure AD endpoint when none set", func(t *testing.T) {
		provider := &oidcProvider{
			name:          "test-oidc",
			tenantID:      "my-tenant-id",
			clientID:      "client-456",
			tokenEndpoint: "",
		}

		endpoint := provider.getTokenEndpoint()
		assert.Equal(t, "https://login.microsoftonline.com/my-tenant-id/oauth2/v2.0/token", endpoint)
	})

	t.Run("returns custom endpoint when set", func(t *testing.T) {
		provider := &oidcProvider{
			name:          "test-oidc",
			tenantID:      "tenant-123",
			clientID:      "client-456",
			tokenEndpoint: "https://custom.endpoint.com/token",
		}

		endpoint := provider.getTokenEndpoint()
		assert.Equal(t, "https://custom.endpoint.com/token", endpoint)
	})
}

func TestExtractOIDCConfig(t *testing.T) {
	tests := []struct {
		name              string
		spec              map[string]interface{}
		expectedTenantID  string
		expectedClientID  string
		expectedSubID     string
		expectedLocation  string
		expectedAudience  string
		expectedTokenPath string
	}{
		{
			name: "all fields present",
			spec: map[string]interface{}{
				"tenant_id":       "tenant-123",
				"client_id":       "client-456",
				"subscription_id": "sub-789",
				"location":        "eastus",
				"audience":        "api://custom",
				"token_file_path": "/path/to/token",
			},
			expectedTenantID:  "tenant-123",
			expectedClientID:  "client-456",
			expectedSubID:     "sub-789",
			expectedLocation:  "eastus",
			expectedAudience:  "api://custom",
			expectedTokenPath: "/path/to/token",
		},
		{
			name: "only required fields",
			spec: map[string]interface{}{
				"tenant_id": "tenant-123",
				"client_id": "client-456",
			},
			expectedTenantID:  "tenant-123",
			expectedClientID:  "client-456",
			expectedSubID:     "",
			expectedLocation:  "",
			expectedAudience:  "",
			expectedTokenPath: "",
		},
		{
			name:              "nil spec",
			spec:              nil,
			expectedTenantID:  "",
			expectedClientID:  "",
			expectedSubID:     "",
			expectedLocation:  "",
			expectedAudience:  "",
			expectedTokenPath: "",
		},
		{
			name:              "empty spec",
			spec:              map[string]interface{}{},
			expectedTenantID:  "",
			expectedClientID:  "",
			expectedSubID:     "",
			expectedLocation:  "",
			expectedAudience:  "",
			expectedTokenPath: "",
		},
		{
			name: "wrong types are ignored",
			spec: map[string]interface{}{
				"tenant_id":       123,     // Wrong type.
				"client_id":       true,    // Wrong type.
				"subscription_id": 45.67,   // Wrong type.
				"location":        []int{}, // Wrong type.
				"audience":        789,     // Wrong type.
				"token_file_path": false,   // Wrong type.
			},
			expectedTenantID:  "",
			expectedClientID:  "",
			expectedSubID:     "",
			expectedLocation:  "",
			expectedAudience:  "",
			expectedTokenPath: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := extractOIDCConfig(tt.spec)

			assert.Equal(t, tt.expectedTenantID, cfg.TenantID, "tenant_id mismatch")
			assert.Equal(t, tt.expectedClientID, cfg.ClientID, "client_id mismatch")
			assert.Equal(t, tt.expectedSubID, cfg.SubscriptionID, "subscription_id mismatch")
			assert.Equal(t, tt.expectedLocation, cfg.Location, "location mismatch")
			assert.Equal(t, tt.expectedAudience, cfg.Audience, "audience mismatch")
			assert.Equal(t, tt.expectedTokenPath, cfg.TokenFilePath, "token_file_path mismatch")
		})
	}
}

func TestOIDCProvider_ReadFederatedToken_Priority(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test token files.
	configTokenPath := filepath.Join(tmpDir, "config_token")
	envTokenPath := filepath.Join(tmpDir, "env_token")

	err := os.WriteFile(configTokenPath, []byte("config-token"), 0o600)
	require.NoError(t, err)
	err = os.WriteFile(envTokenPath, []byte("env-token"), 0o600)
	require.NoError(t, err)

	// Note: t.Setenv in subtests handles cleanup automatically.
	tests := []struct {
		name          string
		tokenFilePath string
		envTokenFile  string
		isGitHub      bool
		expectToken   string
		expectError   bool
	}{
		{
			name:          "config token file takes priority over env",
			tokenFilePath: configTokenPath,
			envTokenFile:  envTokenPath,
			isGitHub:      false,
			expectToken:   "config-token",
			expectError:   false,
		},
		{
			name:          "env token file used when no config",
			tokenFilePath: "",
			envTokenFile:  envTokenPath,
			isGitHub:      false,
			expectToken:   "env-token",
			expectError:   false,
		},
		{
			name:          "error when no token source and not in GitHub Actions",
			tokenFilePath: "",
			envTokenFile:  "",
			isGitHub:      false,
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment using t.Setenv for automatic cleanup.
			if tt.envTokenFile != "" {
				t.Setenv("AZURE_FEDERATED_TOKEN_FILE", tt.envTokenFile)
			} else {
				// Set to empty then clear to ensure test isolation.
				t.Setenv("AZURE_FEDERATED_TOKEN_FILE", "")
				os.Unsetenv("AZURE_FEDERATED_TOKEN_FILE")
			}
			if tt.isGitHub {
				t.Setenv("GITHUB_ACTIONS", "true")
			} else {
				t.Setenv("GITHUB_ACTIONS", "")
				os.Unsetenv("GITHUB_ACTIONS")
			}

			provider := &oidcProvider{
				name:          "test-oidc",
				tenantID:      "tenant-123",
				clientID:      "client-456",
				tokenFilePath: tt.tokenFilePath,
			}

			token, err := provider.readFederatedToken()

			if tt.expectError {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectToken, token)
		})
	}
}
