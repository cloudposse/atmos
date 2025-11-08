package azure

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewSubscriptionIdentity(t *testing.T) {
	tests := []struct {
		name         string
		identityName string
		config       *schema.Identity
		expectError  bool
		errorType    error
	}{
		{
			name:         "valid subscription identity",
			identityName: "azure-dev",
			config: &schema.Identity{
				Kind: "azure/subscription",
				Principal: map[string]interface{}{
					"subscription_id": "sub-123",
					"location":        "eastus",
					"resource_group":  "rg-dev",
				},
				Via: &schema.IdentityVia{
					Provider: "azure-provider",
				},
			},
			expectError: false,
		},
		{
			name:         "valid without resource group",
			identityName: "azure-prod",
			config: &schema.Identity{
				Kind: "azure/subscription",
				Principal: map[string]interface{}{
					"subscription_id": "sub-456",
					"location":        "westus",
				},
				Via: &schema.IdentityVia{
					Provider: "azure-provider",
				},
			},
			expectError: false,
		},
		{
			name:         "valid without location",
			identityName: "azure-test",
			config: &schema.Identity{
				Kind: "azure/subscription",
				Principal: map[string]interface{}{
					"subscription_id": "sub-789",
				},
				Via: &schema.IdentityVia{
					Provider: "azure-provider",
				},
			},
			expectError: false,
		},
		{
			name:         "nil config",
			identityName: "azure-dev",
			config:       nil,
			expectError:  true,
			errorType:    errUtils.ErrInvalidIdentityConfig,
		},
		{
			name:         "wrong kind",
			identityName: "azure-dev",
			config: &schema.Identity{
				Kind: "aws/permission-set",
				Principal: map[string]interface{}{
					"subscription_id": "sub-123",
				},
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidIdentityKind,
		},
		{
			name:         "missing subscription_id",
			identityName: "azure-dev",
			config: &schema.Identity{
				Kind: "azure/subscription",
				Principal: map[string]interface{}{
					"location": "eastus",
				},
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidIdentityConfig,
		},
		{
			name:         "nil principal",
			identityName: "azure-dev",
			config: &schema.Identity{
				Kind:      "azure/subscription",
				Principal: nil,
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidIdentityConfig,
		},
		{
			name:         "empty principal",
			identityName: "azure-dev",
			config: &schema.Identity{
				Kind:      "azure/subscription",
				Principal: map[string]interface{}{},
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidIdentityConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identity, err := NewSubscriptionIdentity(tt.identityName, tt.config)

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, identity)
				if tt.errorType != nil {
					assert.True(t, errors.Is(err, tt.errorType))
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, identity)
			assert.Equal(t, tt.identityName, identity.name)
			assert.Equal(t, "azure/subscription", identity.Kind())

			// Verify fields extracted correctly.
			if tt.config.Principal != nil {
				if subID, ok := tt.config.Principal["subscription_id"].(string); ok {
					assert.Equal(t, subID, identity.subscriptionID)
				}
				if loc, ok := tt.config.Principal["location"].(string); ok {
					assert.Equal(t, loc, identity.location)
				}
				if rg, ok := tt.config.Principal["resource_group"].(string); ok {
					assert.Equal(t, rg, identity.resourceGroup)
				}
			}
		})
	}
}

func TestSubscriptionIdentity_Kind(t *testing.T) {
	identity := &subscriptionIdentity{}
	assert.Equal(t, "azure/subscription", identity.Kind())
}

func TestSubscriptionIdentity_GetProviderName(t *testing.T) {
	tests := []struct {
		name         string
		identity     *subscriptionIdentity
		expectError  bool
		errorType    error
		expectedName string
	}{
		{
			name: "valid provider name",
			identity: &subscriptionIdentity{
				config: &schema.Identity{
					Via: &schema.IdentityVia{
						Provider: "azure-cli",
					},
				},
			},
			expectError:  false,
			expectedName: "azure-cli",
		},
		{
			name: "nil via",
			identity: &subscriptionIdentity{
				config: &schema.Identity{
					Via: nil,
				},
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidIdentityConfig,
		},
		{
			name: "empty provider name",
			identity: &subscriptionIdentity{
				config: &schema.Identity{
					Via: &schema.IdentityVia{
						Provider: "",
					},
				},
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidIdentityConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			providerName, err := tt.identity.GetProviderName()

			if tt.expectError {
				require.Error(t, err)
				assert.Empty(t, providerName)
				if tt.errorType != nil {
					assert.True(t, errors.Is(err, tt.errorType))
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedName, providerName)
		})
	}
}

func TestSubscriptionIdentity_Authenticate(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name        string
		identity    *subscriptionIdentity
		baseCreds   types.ICredentials
		expectError bool
		errorType   error
	}{
		{
			name: "successful authentication",
			identity: &subscriptionIdentity{
				name:           "azure-dev",
				subscriptionID: "identity-sub-123",
				location:       "westus",
			},
			baseCreds: &types.AzureCredentials{
				AccessToken:        "provider-token",
				TokenType:          "Bearer",
				Expiration:         now.Add(1 * time.Hour).Format(time.RFC3339),
				TenantID:           "tenant-123",
				SubscriptionID:     "provider-sub-456", // Should be overridden by identity.
				Location:           "eastus",           // Should be overridden by identity.
				GraphAPIToken:      "graph-token",
				GraphAPIExpiration: now.Add(1 * time.Hour).Format(time.RFC3339),
				KeyVaultToken:      "keyvault-token",
				KeyVaultExpiration: now.Add(1 * time.Hour).Format(time.RFC3339),
			},
			expectError: false,
		},
		{
			name: "identity with no location uses provider location",
			identity: &subscriptionIdentity{
				name:           "azure-dev",
				subscriptionID: "identity-sub-123",
				location:       "", // Empty - should use provider's.
			},
			baseCreds: &types.AzureCredentials{
				AccessToken:    "provider-token",
				TenantID:       "tenant-123",
				SubscriptionID: "provider-sub-456",
				Location:       "provider-location", // Should be preserved.
				Expiration:     now.Add(1 * time.Hour).Format(time.RFC3339),
			},
			expectError: false,
		},
		{
			name: "wrong credential type",
			identity: &subscriptionIdentity{
				name:           "azure-dev",
				subscriptionID: "sub-123",
			},
			baseCreds:   &types.AWSCredentials{}, // Wrong type.
			expectError: true,
			errorType:   errUtils.ErrAuthenticationFailed,
		},
		{
			name: "nil credentials",
			identity: &subscriptionIdentity{
				name:           "azure-dev",
				subscriptionID: "sub-123",
			},
			baseCreds:   nil,
			expectError: true,
			errorType:   errUtils.ErrAuthenticationFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			creds, err := tt.identity.Authenticate(ctx, tt.baseCreds)

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, creds)
				if tt.errorType != nil {
					assert.True(t, errors.Is(err, tt.errorType))
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, creds)

			azureCreds, ok := creds.(*types.AzureCredentials)
			require.True(t, ok, "Should return Azure credentials")

			baseCreds := tt.baseCreds.(*types.AzureCredentials)

			// Verify subscription ID was overridden.
			assert.Equal(t, tt.identity.subscriptionID, azureCreds.SubscriptionID)

			// Verify location handling.
			if tt.identity.location != "" {
				assert.Equal(t, tt.identity.location, azureCreds.Location)
			} else {
				assert.Equal(t, baseCreds.Location, azureCreds.Location)
			}

			// Verify tokens were preserved from provider.
			assert.Equal(t, baseCreds.AccessToken, azureCreds.AccessToken)
			assert.Equal(t, baseCreds.TenantID, azureCreds.TenantID)
			assert.Equal(t, baseCreds.GraphAPIToken, azureCreds.GraphAPIToken)
			assert.Equal(t, baseCreds.KeyVaultToken, azureCreds.KeyVaultToken)
		})
	}
}

func TestSubscriptionIdentity_Validate(t *testing.T) {
	tests := []struct {
		name        string
		identity    *subscriptionIdentity
		expectError bool
		errorType   error
	}{
		{
			name: "valid identity",
			identity: &subscriptionIdentity{
				subscriptionID: "sub-123",
				config: &schema.Identity{
					Via: &schema.IdentityVia{
						Provider: "azure-provider",
					},
				},
			},
			expectError: false,
		},
		{
			name: "missing subscription ID",
			identity: &subscriptionIdentity{
				subscriptionID: "",
				config: &schema.Identity{
					Via: &schema.IdentityVia{
						Provider: "azure-provider",
					},
				},
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidIdentityConfig,
		},
		{
			name: "nil via",
			identity: &subscriptionIdentity{
				subscriptionID: "sub-123",
				config: &schema.Identity{
					Via: nil,
				},
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidIdentityConfig,
		},
		{
			name: "empty provider name",
			identity: &subscriptionIdentity{
				subscriptionID: "sub-123",
				config: &schema.Identity{
					Via: &schema.IdentityVia{
						Provider: "",
					},
				},
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidIdentityConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.identity.Validate()

			if tt.expectError {
				require.Error(t, err)
				if tt.errorType != nil {
					assert.True(t, errors.Is(err, tt.errorType))
				}
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestSubscriptionIdentity_Environment(t *testing.T) {
	tests := []struct {
		name     string
		identity *subscriptionIdentity
		expected map[string]string
	}{
		{
			name: "all fields present",
			identity: &subscriptionIdentity{
				subscriptionID: "sub-123",
				location:       "eastus",
				resourceGroup:  "rg-dev",
			},
			expected: map[string]string{
				"AZURE_SUBSCRIPTION_ID": "sub-123",
				"ARM_SUBSCRIPTION_ID":   "sub-123",
				"AZURE_LOCATION":        "eastus",
				"ARM_LOCATION":          "eastus",
				"AZURE_RESOURCE_GROUP":  "rg-dev",
			},
		},
		{
			name: "only subscription ID",
			identity: &subscriptionIdentity{
				subscriptionID: "sub-456",
			},
			expected: map[string]string{
				"AZURE_SUBSCRIPTION_ID": "sub-456",
				"ARM_SUBSCRIPTION_ID":   "sub-456",
			},
		},
		{
			name: "subscription and location",
			identity: &subscriptionIdentity{
				subscriptionID: "sub-789",
				location:       "westus",
			},
			expected: map[string]string{
				"AZURE_SUBSCRIPTION_ID": "sub-789",
				"ARM_SUBSCRIPTION_ID":   "sub-789",
				"AZURE_LOCATION":        "westus",
				"ARM_LOCATION":          "westus",
			},
		},
		{
			name:     "empty fields",
			identity: &subscriptionIdentity{},
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := tt.identity.Environment()
			require.NoError(t, err)
			assert.Equal(t, tt.expected, env)
		})
	}
}

func TestSubscriptionIdentity_PrepareEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		identity *subscriptionIdentity
		inputEnv map[string]string
		expected map[string]string
	}{
		{
			name: "adds identity vars to existing env",
			identity: &subscriptionIdentity{
				subscriptionID: "sub-123",
				location:       "eastus",
				resourceGroup:  "rg-dev",
			},
			inputEnv: map[string]string{
				"HOME": "/home/user",
				"PATH": "/usr/bin",
			},
			expected: map[string]string{
				"HOME":                  "/home/user",
				"PATH":                  "/usr/bin",
				"AZURE_SUBSCRIPTION_ID": "sub-123",
				"ARM_SUBSCRIPTION_ID":   "sub-123",
				"AZURE_LOCATION":        "eastus",
				"ARM_LOCATION":          "eastus",
				"AZURE_RESOURCE_GROUP":  "rg-dev",
			},
		},
		{
			name: "empty input env",
			identity: &subscriptionIdentity{
				subscriptionID: "sub-456",
			},
			inputEnv: map[string]string{},
			expected: map[string]string{
				"AZURE_SUBSCRIPTION_ID": "sub-456",
				"ARM_SUBSCRIPTION_ID":   "sub-456",
			},
		},
		{
			name: "overwrites existing Azure vars",
			identity: &subscriptionIdentity{
				subscriptionID: "new-sub",
				location:       "new-location",
			},
			inputEnv: map[string]string{
				"AZURE_SUBSCRIPTION_ID": "old-sub",
				"AZURE_LOCATION":        "old-location",
			},
			expected: map[string]string{
				"AZURE_SUBSCRIPTION_ID": "new-sub",
				"ARM_SUBSCRIPTION_ID":   "new-sub",
				"AZURE_LOCATION":        "new-location",
				"ARM_LOCATION":          "new-location",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := tt.identity.PrepareEnvironment(ctx, tt.inputEnv)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)

			// Verify original env not mutated.
			for k, v := range tt.inputEnv {
				// Check original values weren't changed (except for overridden Azure vars).
				if k != "AZURE_SUBSCRIPTION_ID" && k != "AZURE_LOCATION" && k != "ARM_SUBSCRIPTION_ID" && k != "ARM_LOCATION" {
					assert.Equal(t, v, tt.inputEnv[k], "Original environment should not be mutated")
				}
			}
		})
	}
}

func TestSubscriptionIdentity_Logout(t *testing.T) {
	identity := &subscriptionIdentity{
		name: "test-identity",
	}

	ctx := context.Background()
	err := identity.Logout(ctx)
	assert.NoError(t, err, "Logout should always succeed")
}

func TestSubscriptionIdentity_PrincipalFieldTypes(t *testing.T) {
	// Test that non-string types in principal are ignored.
	config := &schema.Identity{
		Kind: "azure/subscription",
		Principal: map[string]interface{}{
			"subscription_id": "sub-123", // Correct type.
			"location":        12345,     // Wrong type - should be ignored.
			"resource_group":  true,      // Wrong type - should be ignored.
		},
		Via: &schema.IdentityVia{
			Provider: "azure-provider",
		},
	}

	identity, err := NewSubscriptionIdentity("test", config)
	require.NoError(t, err)

	assert.Equal(t, "sub-123", identity.subscriptionID)
	assert.Equal(t, "", identity.location, "Wrong type should be ignored")
	assert.Equal(t, "", identity.resourceGroup, "Wrong type should be ignored")
}
