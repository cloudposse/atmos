package azure

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewServicePrincipalProvider(t *testing.T) {
	// Clear environment variable for consistent test results.
	t.Setenv("AZURE_CLIENT_SECRET", "")
	os.Unsetenv("AZURE_CLIENT_SECRET")

	tests := []struct {
		name          string
		providerName  string
		config        *schema.Provider
		expectError   bool
		errorType     error
		checkProvider func(*testing.T, *servicePrincipalProvider)
	}{
		{
			name:         "valid config with all fields",
			providerName: "azure-sp",
			config: &schema.Provider{
				Kind: "azure/service-principal",
				Spec: map[string]interface{}{
					"tenant_id":       "tenant-123",
					"client_id":       "client-456",
					"client_secret":   "secret-789",
					"subscription_id": "sub-abc",
					"location":        "eastus",
				},
			},
			expectError: false,
			checkProvider: func(t *testing.T, p *servicePrincipalProvider) {
				assert.Equal(t, "tenant-123", p.tenantID)
				assert.Equal(t, "client-456", p.clientID)
				assert.Equal(t, "secret-789", p.clientSecret)
				assert.Equal(t, "sub-abc", p.subscriptionID)
				assert.Equal(t, "eastus", p.location)
			},
		},
		{
			name:         "valid config with only required fields",
			providerName: "azure-sp",
			config: &schema.Provider{
				Kind: "azure/service-principal",
				Spec: map[string]interface{}{
					"tenant_id": "tenant-123",
					"client_id": "client-456",
				},
			},
			expectError: false,
			checkProvider: func(t *testing.T, p *servicePrincipalProvider) {
				assert.Equal(t, "tenant-123", p.tenantID)
				assert.Equal(t, "client-456", p.clientID)
				assert.Empty(t, p.clientSecret)
			},
		},
		{
			name:         "missing tenant ID",
			providerName: "azure-sp",
			config: &schema.Provider{
				Kind: "azure/service-principal",
				Spec: map[string]interface{}{"client_id": "client-456"},
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderConfig,
		},
		{
			name:         "missing client ID",
			providerName: "azure-sp",
			config: &schema.Provider{
				Kind: "azure/service-principal",
				Spec: map[string]interface{}{"tenant_id": "tenant-123"},
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderConfig,
		},
		{
			name:         "nil config",
			providerName: "azure-sp",
			config:       nil,
			expectError:  true,
			errorType:    errUtils.ErrInvalidProviderConfig,
		},
		{
			name:         "wrong provider kind",
			providerName: "azure-sp",
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
			provider, err := NewServicePrincipalProvider(tt.providerName, tt.config)

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
			assert.Equal(t, "azure/service-principal", provider.Kind())

			if tt.checkProvider != nil {
				tt.checkProvider(t, provider)
			}
		})
	}
}

func TestServicePrincipalProvider_ClientSecretSources(t *testing.T) {
	t.Run("secret from environment variable", func(t *testing.T) {
		t.Setenv("AZURE_CLIENT_SECRET", "env-secret-123")

		config := &schema.Provider{
			Kind: "azure/service-principal",
			Spec: map[string]interface{}{
				"tenant_id": "tenant-123",
				"client_id": "client-456",
			},
		}

		provider, err := NewServicePrincipalProvider("azure-sp", config)
		require.NoError(t, err)
		assert.Equal(t, "env-secret-123", provider.clientSecret)
	})

	t.Run("config secret overrides env var", func(t *testing.T) {
		t.Setenv("AZURE_CLIENT_SECRET", "env-secret-123")

		config := &schema.Provider{
			Kind: "azure/service-principal",
			Spec: map[string]interface{}{
				"tenant_id":     "tenant-123",
				"client_id":     "client-456",
				"client_secret": "config-secret-456",
			},
		}

		provider, err := NewServicePrincipalProvider("azure-sp", config)
		require.NoError(t, err)
		assert.Equal(t, "config-secret-456", provider.clientSecret)
	})
}

func TestServicePrincipalProvider_CertificateSources(t *testing.T) {
	// Clear environment variables for consistent test results.
	t.Setenv("AZURE_CLIENT_SECRET", "")
	os.Unsetenv("AZURE_CLIENT_SECRET")
	t.Setenv("AZURE_CLIENT_CERTIFICATE_PATH", "")
	os.Unsetenv("AZURE_CLIENT_CERTIFICATE_PATH")
	t.Setenv("AZURE_CLIENT_CERTIFICATE_PASSWORD", "")
	os.Unsetenv("AZURE_CLIENT_CERTIFICATE_PASSWORD")

	t.Run("certificate path from config", func(t *testing.T) {
		config := &schema.Provider{
			Kind: "azure/service-principal",
			Spec: map[string]interface{}{
				"tenant_id":               "tenant-123",
				"client_id":               "client-456",
				"client_certificate_path": "/path/to/cert.pem",
			},
		}

		provider, err := NewServicePrincipalProvider("azure-sp", config)
		require.NoError(t, err)
		assert.Equal(t, "/path/to/cert.pem", provider.clientCertificatePath)
		assert.Empty(t, provider.clientCertificatePassword)
	})

	t.Run("certificate with password from config", func(t *testing.T) {
		config := &schema.Provider{
			Kind: "azure/service-principal",
			Spec: map[string]interface{}{
				"tenant_id":                   "tenant-123",
				"client_id":                   "client-456",
				"client_certificate_path":     "/path/to/cert.pfx",
				"client_certificate_password": "cert-password-123",
			},
		}

		provider, err := NewServicePrincipalProvider("azure-sp", config)
		require.NoError(t, err)
		assert.Equal(t, "/path/to/cert.pfx", provider.clientCertificatePath)
		assert.Equal(t, "cert-password-123", provider.clientCertificatePassword)
	})

	t.Run("certificate from environment variable", func(t *testing.T) {
		t.Setenv("AZURE_CLIENT_CERTIFICATE_PATH", "/env/path/to/cert.pem")

		config := &schema.Provider{
			Kind: "azure/service-principal",
			Spec: map[string]interface{}{
				"tenant_id": "tenant-123",
				"client_id": "client-456",
			},
		}

		provider, err := NewServicePrincipalProvider("azure-sp", config)
		require.NoError(t, err)
		assert.Equal(t, "/env/path/to/cert.pem", provider.clientCertificatePath)
	})

	t.Run("config certificate path overrides env var", func(t *testing.T) {
		t.Setenv("AZURE_CLIENT_CERTIFICATE_PATH", "/env/path/to/cert.pem")

		config := &schema.Provider{
			Kind: "azure/service-principal",
			Spec: map[string]interface{}{
				"tenant_id":               "tenant-123",
				"client_id":               "client-456",
				"client_certificate_path": "/config/path/to/cert.pem",
			},
		}

		provider, err := NewServicePrincipalProvider("azure-sp", config)
		require.NoError(t, err)
		assert.Equal(t, "/config/path/to/cert.pem", provider.clientCertificatePath)
	})

	t.Run("certificate password from environment variable", func(t *testing.T) {
		t.Setenv("AZURE_CLIENT_CERTIFICATE_PATH", "/path/to/cert.pfx")
		t.Setenv("AZURE_CLIENT_CERTIFICATE_PASSWORD", "env-cert-password")

		config := &schema.Provider{
			Kind: "azure/service-principal",
			Spec: map[string]interface{}{
				"tenant_id": "tenant-123",
				"client_id": "client-456",
			},
		}

		provider, err := NewServicePrincipalProvider("azure-sp", config)
		require.NoError(t, err)
		assert.Equal(t, "/path/to/cert.pfx", provider.clientCertificatePath)
		assert.Equal(t, "env-cert-password", provider.clientCertificatePassword)
	})
}

func TestServicePrincipalProvider_InterfaceMethods(t *testing.T) {
	provider := &servicePrincipalProvider{
		name:     "test-sp",
		tenantID: "tenant-123",
		clientID: "client-456",
	}

	t.Run("Kind returns correct value", func(t *testing.T) {
		assert.Equal(t, "azure/service-principal", provider.Kind())
	})

	t.Run("Name returns configured name", func(t *testing.T) {
		assert.Equal(t, "test-sp", provider.Name())
	})

	t.Run("PreAuthenticate is no-op", func(t *testing.T) {
		err := provider.PreAuthenticate(nil)
		assert.NoError(t, err)
	})

	t.Run("Logout returns ErrLogoutNotSupported", func(t *testing.T) {
		err := provider.Logout(context.Background())
		assert.ErrorIs(t, err, errUtils.ErrLogoutNotSupported)
	})

	t.Run("Paths returns empty slice", func(t *testing.T) {
		paths, err := provider.Paths()
		require.NoError(t, err)
		assert.Empty(t, paths)
	})

	t.Run("GetFilesDisplayPath returns empty string", func(t *testing.T) {
		assert.Empty(t, provider.GetFilesDisplayPath())
	})
}

func TestServicePrincipalProvider_Validate(t *testing.T) {
	tests := []struct {
		name        string
		tenantID    string
		clientID    string
		expectError bool
	}{
		{"valid", "tenant-123", "client-456", false},
		{"missing tenant ID", "", "client-456", true},
		{"missing client ID", "tenant-123", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &servicePrincipalProvider{tenantID: tt.tenantID, clientID: tt.clientID}
			err := provider.Validate()
			if tt.expectError {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrInvalidProviderConfig)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestServicePrincipalProvider_Environment(t *testing.T) {
	provider := &servicePrincipalProvider{
		tenantID:       "tenant-123",
		clientID:       "client-456",
		subscriptionID: "sub-789",
		location:       "eastus",
	}

	env, err := provider.Environment()
	require.NoError(t, err)

	expected := map[string]string{
		"AZURE_TENANT_ID":       "tenant-123",
		"AZURE_CLIENT_ID":       "client-456",
		"AZURE_SUBSCRIPTION_ID": "sub-789",
		"AZURE_LOCATION":        "eastus",
	}
	assert.Equal(t, expected, env)
}

func TestServicePrincipalProvider_PrepareEnvironment(t *testing.T) {
	t.Run("with client secret", func(t *testing.T) {
		provider := &servicePrincipalProvider{
			tenantID:       "tenant-123",
			clientID:       "client-456",
			clientSecret:   "secret-789",
			subscriptionID: "sub-789",
			location:       "eastus",
		}

		result, err := provider.PrepareEnvironment(context.Background(), map[string]string{"HOME": "/home/user"})
		require.NoError(t, err)

		// Check key environment variables are set.
		assert.Equal(t, "/home/user", result["HOME"])
		assert.Equal(t, "false", result["ARM_USE_CLI"])
		assert.Equal(t, "false", result["ARM_USE_OIDC"])
		assert.Equal(t, "client-456", result["ARM_CLIENT_ID"])
		assert.Equal(t, "secret-789", result["ARM_CLIENT_SECRET"])
		assert.Equal(t, "sub-789", result["ARM_SUBSCRIPTION_ID"])
		assert.Equal(t, "tenant-123", result["ARM_TENANT_ID"])
	})

	t.Run("with certificate", func(t *testing.T) {
		provider := &servicePrincipalProvider{
			tenantID:              "tenant-123",
			clientID:              "client-456",
			clientCertificatePath: "/path/to/cert.pem",
			subscriptionID:        "sub-789",
			location:              "eastus",
		}

		result, err := provider.PrepareEnvironment(context.Background(), map[string]string{"HOME": "/home/user"})
		require.NoError(t, err)

		// Check certificate environment variables are set.
		assert.Equal(t, "false", result["ARM_USE_CLI"])
		assert.Equal(t, "false", result["ARM_USE_OIDC"])
		assert.Equal(t, "/path/to/cert.pem", result["ARM_CLIENT_CERTIFICATE_PATH"])
		assert.Equal(t, "/path/to/cert.pem", result["AZURE_CLIENT_CERTIFICATE_PATH"])
		// No password should be set.
		_, hasPassword := result["ARM_CLIENT_CERTIFICATE_PASSWORD"]
		assert.False(t, hasPassword)
	})

	t.Run("with certificate and password", func(t *testing.T) {
		provider := &servicePrincipalProvider{
			tenantID:                  "tenant-123",
			clientID:                  "client-456",
			clientCertificatePath:     "/path/to/cert.pfx",
			clientCertificatePassword: "cert-password",
			subscriptionID:            "sub-789",
			location:                  "eastus",
		}

		result, err := provider.PrepareEnvironment(context.Background(), map[string]string{"HOME": "/home/user"})
		require.NoError(t, err)

		// Check certificate environment variables are set.
		assert.Equal(t, "false", result["ARM_USE_CLI"])
		assert.Equal(t, "false", result["ARM_USE_OIDC"])
		assert.Equal(t, "/path/to/cert.pfx", result["ARM_CLIENT_CERTIFICATE_PATH"])
		assert.Equal(t, "/path/to/cert.pfx", result["AZURE_CLIENT_CERTIFICATE_PATH"])
		assert.Equal(t, "cert-password", result["ARM_CLIENT_CERTIFICATE_PASSWORD"])
		assert.Equal(t, "cert-password", result["AZURE_CLIENT_CERTIFICATE_PASSWORD"])
	})
}

func TestServicePrincipalProvider_ExchangeToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

		err := r.ParseForm()
		require.NoError(t, err)

		// Verify service principal specific form data.
		assert.Equal(t, "client_credentials", r.FormValue("grant_type"))
		assert.Equal(t, "client-456", r.FormValue("client_id"))
		assert.Equal(t, "secret-789", r.FormValue("client_secret"))
		assert.Equal(t, azureManagementScope, r.FormValue("scope"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "azure-access-token-123",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		})
	}))
	defer server.Close()

	provider := &servicePrincipalProvider{
		name:          "test-sp",
		tenantID:      "tenant-123",
		clientID:      "client-456",
		clientSecret:  "secret-789",
		tokenEndpoint: server.URL,
	}

	resp, err := provider.exchangeToken(context.Background(), azureManagementScope)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, "azure-access-token-123", resp.AccessToken)
	assert.Equal(t, "Bearer", resp.TokenType)
	assert.Equal(t, int64(3600), resp.ExpiresIn)
}

func TestServicePrincipalProvider_ExchangeToken_Errors(t *testing.T) {
	tests := []struct {
		name       string
		response   interface{}
		statusCode int
	}{
		{
			name:       "server error",
			response:   map[string]string{"error": "invalid_client"},
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "empty access token",
			response:   tokenResponse{AccessToken: "", TokenType: "Bearer", ExpiresIn: 3600},
			statusCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				_ = json.NewEncoder(w).Encode(tt.response)
			}))
			defer server.Close()

			provider := &servicePrincipalProvider{
				name:          "test-sp",
				tenantID:      "tenant-123",
				clientID:      "client-456",
				clientSecret:  "secret-789",
				tokenEndpoint: server.URL,
			}

			_, err := provider.exchangeToken(context.Background(), azureManagementScope)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
		})
	}
}

func TestServicePrincipalProvider_Authenticate(t *testing.T) {
	t.Run("successful authentication", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(tokenResponse{
				AccessToken: "azure-access-token-xyz",
				TokenType:   "Bearer",
				ExpiresIn:   7200,
			})
		}))
		defer server.Close()

		provider := &servicePrincipalProvider{
			name:           "test-sp",
			tenantID:       "tenant-123",
			clientID:       "client-456",
			clientSecret:   "secret-789",
			subscriptionID: "sub-789",
			location:       "eastus",
			tokenEndpoint:  server.URL,
		}

		creds, err := provider.Authenticate(context.Background())
		require.NoError(t, err)
		require.NotNil(t, creds)

		azureCreds, ok := creds.(*authTypes.AzureCredentials)
		require.True(t, ok, "expected AzureCredentials type")

		assert.Equal(t, "azure-access-token-xyz", azureCreds.AccessToken)
		assert.Equal(t, "Bearer", azureCreds.TokenType)
		assert.Equal(t, "tenant-123", azureCreds.TenantID)
		assert.Equal(t, "sub-789", azureCreds.SubscriptionID)
		assert.True(t, azureCreds.IsServicePrincipal)
	})

	t.Run("missing client secret fails", func(t *testing.T) {
		provider := &servicePrincipalProvider{
			name:         "test-sp",
			tenantID:     "tenant-123",
			clientID:     "client-456",
			clientSecret: "", // Missing secret.
		}

		_, err := provider.Authenticate(context.Background())
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	})

	t.Run("certificate-only authentication succeeds", func(t *testing.T) {
		provider := &servicePrincipalProvider{
			name:                  "test-sp",
			tenantID:              "tenant-123",
			clientID:              "client-456",
			clientCertificatePath: "/path/to/cert.pem",
			subscriptionID:        "sub-789",
			location:              "eastus",
		}

		creds, err := provider.Authenticate(context.Background())
		require.NoError(t, err)
		require.NotNil(t, creds)

		azureCreds, ok := creds.(*authTypes.AzureCredentials)
		require.True(t, ok, "expected AzureCredentials type")

		// Certificate-only auth returns credentials without access token.
		assert.Empty(t, azureCreds.AccessToken)
		assert.Equal(t, "tenant-123", azureCreds.TenantID)
		assert.Equal(t, "sub-789", azureCreds.SubscriptionID)
		assert.Equal(t, "client-456", azureCreds.ClientID)
		assert.True(t, azureCreds.IsServicePrincipal)
	})
}

func TestServicePrincipalProvider_HTTPClientAndEndpoint(t *testing.T) {
	t.Run("returns default client when none injected", func(t *testing.T) {
		provider := &servicePrincipalProvider{httpClient: nil}
		client := provider.getHTTPClient()
		require.NotNil(t, client)
	})

	t.Run("returns injected client when provided", func(t *testing.T) {
		mockClient := &http.Client{Timeout: 5 * time.Second}
		provider := &servicePrincipalProvider{httpClient: mockClient}
		assert.Equal(t, mockClient, provider.getHTTPClient())
	})

	t.Run("returns default Azure AD endpoint", func(t *testing.T) {
		provider := &servicePrincipalProvider{tenantID: "my-tenant-id", tokenEndpoint: ""}
		assert.Equal(t, "https://login.microsoftonline.com/my-tenant-id/oauth2/v2.0/token", provider.getTokenEndpoint())
	})

	t.Run("returns custom endpoint when set", func(t *testing.T) {
		provider := &servicePrincipalProvider{tokenEndpoint: "https://custom.endpoint.com/token"}
		assert.Equal(t, "https://custom.endpoint.com/token", provider.getTokenEndpoint())
	})
}

func TestExtractServicePrincipalConfig(t *testing.T) {
	t.Run("with client secret", func(t *testing.T) {
		cfg := extractServicePrincipalConfig(map[string]interface{}{
			"tenant_id":       "tenant-123",
			"client_id":       "client-456",
			"client_secret":   "secret-789",
			"subscription_id": "sub-abc",
			"location":        "eastus",
		})

		assert.Equal(t, "tenant-123", cfg.TenantID)
		assert.Equal(t, "client-456", cfg.ClientID)
		assert.Equal(t, "secret-789", cfg.ClientSecret)
		assert.Equal(t, "sub-abc", cfg.SubscriptionID)
		assert.Equal(t, "eastus", cfg.Location)
		assert.Empty(t, cfg.ClientCertificatePath)
		assert.Empty(t, cfg.ClientCertificatePassword)
	})

	t.Run("with client certificate", func(t *testing.T) {
		cfg := extractServicePrincipalConfig(map[string]interface{}{
			"tenant_id":                   "tenant-123",
			"client_id":                   "client-456",
			"client_certificate_path":     "/path/to/cert.pfx",
			"client_certificate_password": "cert-password",
			"subscription_id":             "sub-abc",
			"location":                    "eastus",
		})

		assert.Equal(t, "tenant-123", cfg.TenantID)
		assert.Equal(t, "client-456", cfg.ClientID)
		assert.Empty(t, cfg.ClientSecret)
		assert.Equal(t, "/path/to/cert.pfx", cfg.ClientCertificatePath)
		assert.Equal(t, "cert-password", cfg.ClientCertificatePassword)
		assert.Equal(t, "sub-abc", cfg.SubscriptionID)
		assert.Equal(t, "eastus", cfg.Location)
	})
}

func TestExtractServicePrincipalConfig_Empty(t *testing.T) {
	cfg := extractServicePrincipalConfig(nil)
	assert.Empty(t, cfg.TenantID)
	assert.Empty(t, cfg.ClientID)
	assert.Empty(t, cfg.ClientSecret)
}
