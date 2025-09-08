package github

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/internal/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewOIDCProvider(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		config       *schema.Provider
		expectError  bool
		errorMsg     string
	}{
		{
			name:         "valid config",
			providerName: "github-oidc",
			config: &schema.Provider{
				Kind:   "github/oidc",
				Region: "us-east-1",
			},
			expectError: false,
		},
		{
			name:         "nil config",
			providerName: "github-oidc",
			config:       nil,
			expectError:  true,
			errorMsg:     "provider config is required",
		},
		{
			name:         "empty name",
			providerName: "",
			config: &schema.Provider{
				Kind:   "github/oidc",
				Region: "us-east-1",
			},
			expectError: true,
			errorMsg:    "provider name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewOIDCProvider(tt.providerName, tt.config)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, provider)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, provider)
				assert.Equal(t, "github/oidc", provider.Kind())
			}
		})
	}
}

func TestOIDCProvider_Authenticate(t *testing.T) {
	tests := []struct {
		name        string
		setupEnv    func()
		cleanupEnv  func()
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid GitHub Actions environment",
			setupEnv:    func() {},
			cleanupEnv:  func() {},
			expectError: false,
		},
		{
			name:        "missing GitHub Actions environment",
			setupEnv:    func() {},
			cleanupEnv:  func() {},
			expectError: true,
			errorMsg:    "GitHub OIDC authentication is only available in GitHub Actions environment",
			expectError: false,
		},
		{
			name: "missing GitHub Actions environment",
			setupEnv: func() {
				os.Unsetenv("GITHUB_ACTIONS")
				os.Unsetenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
			},
			cleanupEnv:  func() {},
			expectError: true,
			errorMsg:    "GitHub OIDC authentication is only available in GitHub Actions environment",
		},
		{
			name: "missing OIDC token",
			setupEnv: func() {
				os.Setenv("GITHUB_ACTIONS", "true")
				os.Unsetenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
			},
			cleanupEnv: func() {
				os.Unsetenv("GITHUB_ACTIONS")
			},
			expectError: true,
			errorMsg:    "ACTIONS_ID_TOKEN_REQUEST_TOKEN not found",
		},
		{
			name: "missing OIDC URL",
			setupEnv: func() {
				os.Setenv("GITHUB_ACTIONS", "true")
				os.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "test-token")
				os.Unsetenv("ACTIONS_ID_TOKEN_REQUEST_URL")
			},
			cleanupEnv: func() {
				os.Unsetenv("GITHUB_ACTIONS")
				os.Unsetenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
			},
			expectError: true,
			errorMsg:    "ACTIONS_ID_TOKEN_REQUEST_URL not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			tt.setupEnv()
			defer tt.cleanupEnv()

			config := &schema.Provider{
				Kind:   "github/oidc",
				Region: "us-east-1",
			}
			provider, err := NewOIDCProvider("github-oidc", config)
			require.NoError(t, err)

			// Test
			ctx := context.Background()
			// Local OIDC endpoint.
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"value":"eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.test-jwt-token"}`))
			}))
			defer srv.Close()
			t.Setenv("GITHUB_ACTIONS", "true")
			t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "test-token")
			t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", srv.URL)

			creds, err := provider.Authenticate(ctx)
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, creds)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, creds)
				oidc, ok := creds.(*types.OIDCCredentials)
				require.True(t, ok, "expected OIDC credentials type")
				assert.Equal(t, "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.test-jwt-token", oidc.Token)
			}
		})
	}
}

func TestOIDCProvider_Validate(t *testing.T) {
	config := &schema.Provider{
		Kind:   "github/oidc",
		Region: "us-east-1",
	}
	provider, err := NewOIDCProvider("github-oidc", config)
	require.NoError(t, err)

	err = provider.Validate()
	assert.NoError(t, err)
}

func TestOIDCProvider_Environment(t *testing.T) {
	config := &schema.Provider{
		Kind:   "github/oidc",
		Region: "us-east-1",
	}
	provider, err := NewOIDCProvider("github-oidc", config)
	require.NoError(t, err)

	env, err := provider.Environment()
	assert.NoError(t, err)
	assert.Empty(t, env) // GitHub OIDC provider doesn't set additional environment variables
}

func TestOIDCProvider_isGitHubActions(t *testing.T) {
	tests := []struct {
		name     string
		setupEnv func()
		cleanup  func()
		expected bool
	}{
		{
			name: "GitHub Actions environment",
			setupEnv: func() {
				os.Setenv("GITHUB_ACTIONS", "true")
			},
			cleanup: func() {
				os.Unsetenv("GITHUB_ACTIONS")
			},
			expected: true,
		},
		{
			name: "non-GitHub Actions environment",
			setupEnv: func() {
				os.Unsetenv("GITHUB_ACTIONS")
			},
			cleanup:  func() {},
			expected: false,
		},
		{
			name: "GitHub Actions set to false",
			setupEnv: func() {
				os.Setenv("GITHUB_ACTIONS", "false")
			},
			cleanup: func() {
				os.Unsetenv("GITHUB_ACTIONS")
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupEnv()
			defer tt.cleanup()

			config := &schema.Provider{
				Kind:   "github/oidc",
				Region: "us-east-1",
			}
			provider, err := NewOIDCProvider("github-oidc", config)
			require.NoError(t, err)

			// Access the private method through reflection or make it public for testing
			// For now, we'll test the behavior through Authenticate
			ctx := context.Background()
			_, err = provider.Authenticate(ctx)

			if tt.expected {
				// Should not fail due to GitHub Actions check (may fail for other reasons like missing tokens)
				if err != nil {
					assert.NotContains(t, err.Error(), "GitHub OIDC authentication is only available in GitHub Actions environment")
				}
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "GitHub OIDC authentication is only available in GitHub Actions environment")
			}
		})
	}
}
