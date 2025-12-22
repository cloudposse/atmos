package factory

import (
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewProvider_Factory(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		config       *schema.Provider
		expectError  bool
		errorType    error
	}{
		{
			name:         "aws-sso-valid",
			providerName: "aws-sso",
			config:       &schema.Provider{Kind: "aws/iam-identity-center", Region: "us-east-1", StartURL: "https://example.awsapps.com/start"},
			expectError:  false,
		},
		{
			name:         "aws-saml-valid",
			providerName: "aws-saml",
			config:       &schema.Provider{Kind: "aws/saml", Region: "us-east-1", URL: "https://idp.example.com/saml"},
			expectError:  false,
		},
		{
			name:         "azure-cli-valid",
			providerName: "azure-cli",
			config: &schema.Provider{
				Kind: "azure/cli",
				Spec: map[string]interface{}{
					"tenant_id": "test-tenant-id",
				},
			},
			expectError: false,
		},
		{
			name:         "azure-device-code-valid",
			providerName: "azure-device",
			config: &schema.Provider{
				Kind: "azure/device-code",
				Spec: map[string]interface{}{
					"tenant_id": "test-tenant-id",
				},
			},
			expectError: false,
		},
		{
			name:         "github-oidc-valid",
			providerName: "github-oidc",
			config:       &schema.Provider{Kind: "github/oidc", Region: "us-east-1"},
			expectError:  false,
		},
		{
			name:         "mock-provider",
			providerName: "mock",
			config:       &schema.Provider{Kind: "mock"},
			expectError:  false,
		},
		{
			name:         "mock-aws-provider",
			providerName: "mock-aws",
			config:       &schema.Provider{Kind: "mock-aws"},
			expectError:  false,
		},
		{
			name:         "nil-config",
			providerName: "test",
			config:       nil,
			expectError:  true,
			errorType:    errUtils.ErrInvalidAuthConfig,
		},
		{
			name:         "unknown-kind",
			providerName: "unknown",
			config:       &schema.Provider{Kind: "unknown/kind"},
			expectError:  true,
			errorType:    errUtils.ErrInvalidProviderKind,
		},
		{
			name:         "empty-kind",
			providerName: "empty",
			config:       &schema.Provider{Kind: ""},
			expectError:  true,
			errorType:    errUtils.ErrInvalidProviderKind,
		},
		{
			name:         "empty-name-allowed",
			providerName: "",
			config:       &schema.Provider{Kind: "mock"},
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewProvider(tt.providerName, tt.config)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorType != nil {
					assert.ErrorIs(t, err, tt.errorType)
				}
				assert.Nil(t, provider)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, provider)
			}
		})
	}
}

func TestNewIdentity_Factory(t *testing.T) {
	tests := []struct {
		name         string
		identityName string
		config       *schema.Identity
		expectError  bool
		errorType    error
	}{
		{
			name:         "aws-permission-set-valid",
			identityName: "dev",
			config:       &schema.Identity{Kind: "aws/permission-set"},
			expectError:  false,
		},
		{
			name:         "aws-assume-role-valid",
			identityName: "role",
			config:       &schema.Identity{Kind: "aws/assume-role"},
			expectError:  false,
		},
		{
			name:         "aws-assume-root-valid",
			identityName: "root-access",
			config:       &schema.Identity{Kind: "aws/assume-root"},
			expectError:  false,
		},
		{
			name:         "aws-user-valid",
			identityName: "me",
			config:       &schema.Identity{Kind: "aws/user"},
			expectError:  false,
		},
		{
			name:         "azure-subscription-valid",
			identityName: "azure-sub",
			config: &schema.Identity{
				Kind: "azure/subscription",
				Principal: map[string]interface{}{
					"subscription_id": "test-subscription-id",
				},
			},
			expectError: false,
		},
		{
			name:         "mock-identity",
			identityName: "mock",
			config:       &schema.Identity{Kind: "mock"},
			expectError:  false,
		},
		{
			name:         "mock-aws-identity",
			identityName: "mock-aws",
			config:       &schema.Identity{Kind: "mock-aws"},
			expectError:  false,
		},
		{
			name:         "nil-config",
			identityName: "test",
			config:       nil,
			expectError:  true,
			errorType:    errUtils.ErrInvalidAuthConfig,
		},
		{
			name:         "unknown-kind",
			identityName: "unknown",
			config:       &schema.Identity{Kind: "unknown/kind"},
			expectError:  true,
			errorType:    errUtils.ErrInvalidIdentityKind,
		},
		{
			name:         "empty-kind",
			identityName: "empty",
			config:       &schema.Identity{Kind: ""},
			expectError:  true,
			errorType:    errUtils.ErrInvalidIdentityKind,
		},
		{
			name:         "empty-name-allowed",
			identityName: "",
			config:       &schema.Identity{Kind: "mock"},
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identity, err := NewIdentity(tt.identityName, tt.config)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorType != nil {
					assert.ErrorIs(t, err, tt.errorType)
				}
				assert.Nil(t, identity)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, identity)
			}
		})
	}
}
