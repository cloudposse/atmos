package exec

import (
	"os"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestGitHubAuth tests GitHub Container Registry authentication.
func TestGitHubAuth(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		atmosConfig *schema.AtmosConfiguration
		expectError bool
		errorMsg    string
	}{
		{
			name:        "GitHub Container Registry with token",
			registry:    "ghcr.io",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: false, // Will succeed with GITHUB_TOKEN
		},
		{
			name:     "GitHub Container Registry with Atmos token",
			registry: "ghcr.io",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					OCI: schema.OCISettings{
						GithubToken: "test-token",
					},
				},
			},
			expectError: false,
		},
		{
			name:        "Non-GitHub registry",
			registry:    "docker.io",
			atmosConfig: &schema.AtmosConfiguration{},
			expectError: true,
			errorMsg:    "no GitHub authentication found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment for tests that need it
			if tt.name == "GitHub Container Registry with token" {
				os.Setenv("GITHUB_TOKEN", "test-token")
				defer os.Unsetenv("GITHUB_TOKEN")
			}

			auth, err := getGitHubAuth(tt.registry, tt.atmosConfig)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, auth)

				// Verify the authenticator is of the correct type
				basicAuth, ok := auth.(*authn.Basic)
				assert.True(t, ok)
				assert.Equal(t, "oauth2", basicAuth.Username)
				assert.Equal(t, "test-token", basicAuth.Password)
			}
		})
	}
}

// TestGitHubAuthWithEnvironment tests GitHub authentication with environment variables.
func TestGitHubAuthWithEnvironment(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		atmosConfig *schema.AtmosConfiguration
		envToken    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "GitHub CR with GITHUB_TOKEN environment variable",
			registry:    "ghcr.io",
			atmosConfig: &schema.AtmosConfiguration{},
			envToken:    "env-token",
			expectError: false,
		},
		{
			name:        "GitHub CR with ATMOS_OCI_GITHUB_TOKEN environment variable",
			registry:    "ghcr.io",
			atmosConfig: &schema.AtmosConfiguration{},
			envToken:    "atmos-token",
			expectError: false,
		},
		{
			name:        "GitHub CR with no environment token",
			registry:    "ghcr.io",
			atmosConfig: &schema.AtmosConfiguration{},
			envToken:    "",
			expectError: true,
			errorMsg:    "no GitHub authentication found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment
			if tt.envToken != "" {
				if tt.name == "GitHub CR with ATMOS_OCI_GITHUB_TOKEN environment variable" {
					os.Setenv("ATMOS_OCI_GITHUB_TOKEN", tt.envToken)
					defer os.Unsetenv("ATMOS_OCI_GITHUB_TOKEN")
				} else {
					os.Setenv("GITHUB_TOKEN", tt.envToken)
					defer os.Unsetenv("GITHUB_TOKEN")
				}
			}

			auth, err := getGitHubAuth(tt.registry, tt.atmosConfig)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, auth)

				// Verify the authenticator
				basicAuth, ok := auth.(*authn.Basic)
				assert.True(t, ok)
				assert.Equal(t, "oauth2", basicAuth.Username)
				assert.Equal(t, tt.envToken, basicAuth.Password)
			}
		})
	}
}

// TestGitHubAuthPrecedence tests token precedence (Atmos config vs environment).
func TestGitHubAuthPrecedence(t *testing.T) {
	// Test that Atmos config token takes precedence over environment
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			OCI: schema.OCISettings{
				GithubToken: "atmos-config-token",
			},
		},
	}

	// Set environment token
	os.Setenv("GITHUB_TOKEN", "env-token")
	defer os.Unsetenv("GITHUB_TOKEN")

	auth, err := getGitHubAuth("ghcr.io", atmosConfig)

	assert.NoError(t, err)
	assert.NotNil(t, auth)

	// Should use Atmos config token, not environment token
	basicAuth, ok := auth.(*authn.Basic)
	assert.True(t, ok)
	assert.Equal(t, "oauth2", basicAuth.Username)
	assert.Equal(t, "atmos-config-token", basicAuth.Password)
}
