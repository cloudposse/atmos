package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func validOidcSpec() *schema.Provider {
	return &schema.Provider{
		Kind:   "github/oidc",
		Region: "us-east-1",
		Spec: map[string]interface{}{
			"audience": "sts.us-east-1.amazonaws.com",
		},
	}
}

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
			config:       validOidcSpec(),
			expectError:  false,
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
		setOidcUrl  bool
	}{
		{
			name: "valid GitHub Actions environment",
			setupEnv: func() {
				t.Setenv("GITHUB_ACTIONS", "true")
				t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "test-token")
			},
			cleanupEnv: func() {
				t.Setenv("GITHUB_ACTIONS", "")
				t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")
			},
			setOidcUrl:  true,
			expectError: false,
		},
		{
			name: "missing GitHub Actions environment",
			setupEnv: func() {
				t.Setenv("GITHUB_ACTIONS", "")
				t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")
			},
			cleanupEnv: func() {
				t.Setenv("GITHUB_ACTIONS", "")
				t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")
			},
			setOidcUrl:  true,
			expectError: true,
			errorMsg:    "GitHub OIDC authentication is only available in GitHub Actions environment",
		},
		{
			name: "missing OIDC token",
			setupEnv: func() {
				t.Setenv("GITHUB_ACTIONS", "true")
			},
			cleanupEnv: func() {
				t.Setenv("GITHUB_ACTIONS", "")
			},
			setOidcUrl:  true,
			expectError: true,
			errorMsg:    "ACTIONS_ID_TOKEN_REQUEST_TOKEN not found",
		},
		{
			name: "missing OIDC URL",
			setupEnv: func() {
				t.Setenv("GITHUB_ACTIONS", "true")
				t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "test-token")
			},
			cleanupEnv: func() {
				t.Setenv("GITHUB_ACTIONS", "")
				t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")
			},
			expectError: true,
			setOidcUrl:  false,
			errorMsg:    "ACTIONS_ID_TOKEN_REQUEST_URL not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup.
			tt.setupEnv()
			defer tt.cleanupEnv()

			config := validOidcSpec()
			provider, err := NewOIDCProvider("github-oidc", config)
			require.NoError(t, err)

			// Test.
			ctx := context.Background()
			// Local OIDC endpoint.
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"value":"eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.test-jwt-token"}`))
			}))
			defer srv.Close()
			if tt.setOidcUrl {
				t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", srv.URL)
			}

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
	config := validOidcSpec()
	provider, err := NewOIDCProvider("github-oidc", config)
	require.NoError(t, err)

	err = provider.Validate()
	assert.NoError(t, err)
}

func TestOIDCProvider_Environment(t *testing.T) {
	config := validOidcSpec()
	provider, err := NewOIDCProvider("github-oidc", config)
	require.NoError(t, err)

	env, err := provider.Environment()
	assert.NoError(t, err)
	assert.Empty(t, env) // GitHub OIDC provider doesn't set additional environment variables.
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
				t.Setenv("GITHUB_ACTIONS", "true")
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
				t.Setenv("GITHUB_ACTIONS", "false")
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

			config := validOidcSpec()
			provider, err := NewOIDCProvider("github-oidc", config)
			require.NoError(t, err)

			// Access the private method through reflection or make it public for testing.
			// For now, we'll test the behavior through Authenticate.
			ctx := context.Background()
			_, err = provider.Authenticate(ctx)

			if tt.expected {
				// Should not fail due to GitHub Actions check (may fail for other reasons like missing tokens).
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

func TestOIDCProvider_NameAndPreAuthenticate(t *testing.T) {
	p, err := NewOIDCProvider("github-oidc", &schema.Provider{Kind: "github/oidc", Region: "us-east-1"})
	require.NoError(t, err)
	require.Equal(t, "github-oidc", p.Name())
	require.NoError(t, p.PreAuthenticate(nil))
}

func TestOIDCProvider_Logout(t *testing.T) {
	p, err := NewOIDCProvider("github-oidc", &schema.Provider{Kind: "github/oidc", Region: "us-east-1"})
	require.NoError(t, err)

	ctx := context.Background()
	err = p.Logout(ctx)

	// GitHub OIDC provider's Logout returns ErrLogoutNotSupported (exit 0).
	assert.ErrorIs(t, err, errUtils.ErrLogoutNotSupported)
}
