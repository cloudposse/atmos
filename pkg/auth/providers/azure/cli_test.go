package azure

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewCLIProvider(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		config       *schema.Provider
		expectError  bool
		errorType    error
	}{
		{
			name:         "valid CLI provider config",
			providerName: "azure-cli",
			config: &schema.Provider{
				Kind: "azure/cli",
				Spec: map[string]interface{}{
					"tenant_id":       "tenant-123",
					"subscription_id": "sub-456",
					"location":        "eastus",
				},
			},
			expectError: false,
		},
		{
			name:         "valid config without subscription ID",
			providerName: "azure-cli",
			config: &schema.Provider{
				Kind: "azure/cli",
				Spec: map[string]interface{}{
					"tenant_id": "tenant-123",
					"location":  "westus",
				},
			},
			expectError: false,
		},
		{
			name:         "valid config without location",
			providerName: "azure-cli",
			config: &schema.Provider{
				Kind: "azure/cli",
				Spec: map[string]interface{}{
					"tenant_id":       "tenant-123",
					"subscription_id": "sub-456",
				},
			},
			expectError: false,
		},
		{
			name:         "missing tenant ID",
			providerName: "azure-cli",
			config: &schema.Provider{
				Kind: "azure/cli",
				Spec: map[string]interface{}{
					"subscription_id": "sub-456",
				},
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderConfig,
		},
		{
			name:         "nil spec",
			providerName: "azure-cli",
			config: &schema.Provider{
				Kind: "azure/cli",
				Spec: nil,
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderConfig,
		},
		{
			name:         "empty spec",
			providerName: "azure-cli",
			config: &schema.Provider{
				Kind: "azure/cli",
				Spec: map[string]interface{}{},
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderConfig,
		},
		{
			name:         "nil config",
			providerName: "azure-cli",
			config:       nil,
			expectError:  true,
			errorType:    errUtils.ErrInvalidProviderConfig,
		},
		{
			name:         "wrong provider kind",
			providerName: "azure-cli",
			config: &schema.Provider{
				Kind: "azure/device-code",
				Spec: map[string]interface{}{
					"tenant_id": "tenant-123",
				},
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderKind,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewCLIProvider(tt.providerName, tt.config)

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, provider)
				if tt.errorType != nil {
					assert.True(t, errors.Is(err, tt.errorType))
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, provider)
			assert.Equal(t, tt.providerName, provider.Name())
			assert.Equal(t, "azure/cli", provider.Kind())

			// Verify spec fields were extracted correctly.
			if tt.config.Spec != nil {
				if tenantID, ok := tt.config.Spec["tenant_id"].(string); ok {
					assert.Equal(t, tenantID, provider.tenantID)
				}
				if subID, ok := tt.config.Spec["subscription_id"].(string); ok {
					assert.Equal(t, subID, provider.subscriptionID)
				}
				if loc, ok := tt.config.Spec["location"].(string); ok {
					assert.Equal(t, loc, provider.location)
				}
			}
		})
	}
}

func TestCLIProvider_Kind(t *testing.T) {
	provider := &cliProvider{}
	assert.Equal(t, "azure/cli", provider.Kind())
}

func TestCLIProvider_Name(t *testing.T) {
	tests := []struct {
		name     string
		provider *cliProvider
		expected string
	}{
		{
			name:     "provider with name",
			provider: &cliProvider{name: "my-azure-cli"},
			expected: "my-azure-cli",
		},
		{
			name:     "provider with empty name",
			provider: &cliProvider{name: ""},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.provider.Name())
		})
	}
}

func TestCLIProvider_PreAuthenticate(t *testing.T) {
	provider := &cliProvider{
		name: "test-cli",
		config: &schema.Provider{
			Kind: "azure/cli",
			Spec: map[string]interface{}{
				"tenant_id": "tenant-123",
			},
		},
		tenantID: "tenant-123",
	}

	// PreAuthenticate should be a no-op and always return nil.
	err := provider.PreAuthenticate(nil)
	assert.NoError(t, err)
}

func TestCLIProvider_FieldExtraction(t *testing.T) {
	// Test that fields are correctly extracted from spec.
	config := &schema.Provider{
		Kind: "azure/cli",
		Spec: map[string]interface{}{
			"tenant_id":       "extracted-tenant",
			"subscription_id": "extracted-sub",
			"location":        "extracted-location",
		},
	}

	provider, err := NewCLIProvider("test-provider", config)
	require.NoError(t, err)

	assert.Equal(t, "extracted-tenant", provider.tenantID)
	assert.Equal(t, "extracted-sub", provider.subscriptionID)
	assert.Equal(t, "extracted-location", provider.location)
}

func TestCLIProvider_OptionalFields(t *testing.T) {
	// Test that optional fields default to empty strings.
	config := &schema.Provider{
		Kind: "azure/cli",
		Spec: map[string]interface{}{
			"tenant_id": "tenant-123",
			// subscription_id and location omitted.
		},
	}

	provider, err := NewCLIProvider("test-provider", config)
	require.NoError(t, err)

	assert.Equal(t, "tenant-123", provider.tenantID)
	assert.Equal(t, "", provider.subscriptionID)
	assert.Equal(t, "", provider.location)
}

func TestCLIProvider_SpecFieldTypes(t *testing.T) {
	tests := []struct {
		name        string
		spec        map[string]interface{}
		expectError bool
	}{
		{
			name: "correct string types",
			spec: map[string]interface{}{
				"tenant_id":       "tenant-123",
				"subscription_id": "sub-456",
				"location":        "eastus",
			},
			expectError: false,
		},
		{
			name: "wrong type for tenant_id",
			spec: map[string]interface{}{
				"tenant_id": 12345, // Not a string.
			},
			expectError: true,
		},
		{
			name: "subscription_id as int ignored",
			spec: map[string]interface{}{
				"tenant_id":       "tenant-123",
				"subscription_id": 789, // Wrong type, should be ignored.
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &schema.Provider{
				Kind: "azure/cli",
				Spec: tt.spec,
			}

			provider, err := NewCLIProvider("test", config)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, provider)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, provider)
		})
	}
}
