package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/authvalidation"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestInitCliConfig_ValidatesAuthConfig(t *testing.T) {
	tests := []struct {
		name        string
		authConfig  string
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid auth config",
			authConfig: `auth:
  providers:
    aws-sso:
      kind: aws/iam-identity-center
      region: us-east-1
      start_url: https://example.awsapps.com/start
  identities:
    dev:
      kind: aws/permission-set
      via:
        provider: aws-sso
      principal:
        name: DevAccess
        account:
          name: dev`,
			expectError: false,
		},
		{
			name: "invalid provider kind",
			authConfig: `auth:
  providers:
    invalid-provider:
      kind: azure-ad
      region: us-east-1`,
			expectError: true,
			errorMsg:    "invalid provider kind",
		},
		{
			name: "invalid identity kind",
			authConfig: `auth:
  providers:
    aws-sso:
      kind: aws/iam-identity-center
      region: us-east-1
      start_url: https://example.awsapps.com/start
  identities:
    bad-identity:
      kind: unknown/kind
      via:
        provider: aws-sso`,
			expectError: true,
			errorMsg:    "invalid identity kind",
		},
		{
			name: "empty auth config - should pass",
			authConfig: `base_path: .
components:
  terraform:
    base_path: components/terraform`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory with atmos.yaml.
			tempDir := t.TempDir()
			configFile := filepath.Join(tempDir, "atmos.yaml")

			err := os.WriteFile(configFile, []byte(tt.authConfig), 0o600)
			require.NoError(t, err)

			// Change to temp directory.
			t.Chdir(tempDir)

			// Call InitCliConfig.
			_, err = InitCliConfig(schema.ConfigAndStacksInfo{}, false)

			if tt.expectError {
				assert.Error(t, err, "Expected an error but got none")
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg, "Error message should contain expected text")
				}
			} else {
				assert.NoError(t, err, "Expected no error but got: %v", err)
			}
		})
	}
}

func TestValidateAuthConfigSyntax_EmptyConfig(t *testing.T) {
	// Empty auth config should not trigger validation.
	authConfig := &schema.AuthConfig{}
	err := authvalidation.ValidateSyntax(authConfig)
	assert.NoError(t, err)
}

func TestValidateAuthConfigSyntax_OnlyProvidersConfigured(t *testing.T) {
	// Valid provider configuration should pass.
	authConfig := &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"aws-sso": {
				Kind:     "aws/iam-identity-center",
				Region:   "us-east-1",
				StartURL: "https://example.awsapps.com/start",
			},
		},
	}
	err := authvalidation.ValidateSyntax(authConfig)
	assert.NoError(t, err)
}

func TestValidateAuthConfigSyntax_InvalidProviderKind(t *testing.T) {
	// Invalid provider kind should fail validation.
	authConfig := &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"bad-provider": {
				Kind: "invalid/kind",
			},
		},
	}
	err := authvalidation.ValidateSyntax(authConfig)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid provider kind")
}
