package azure

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewDeviceCodeProvider(t *testing.T) {
	tests := []struct {
		name          string
		providerName  string
		config        *schema.Provider
		expectError   bool
		errorType     error
		checkProvider func(*testing.T, *deviceCodeProvider)
	}{
		{
			name:         "valid device code provider config",
			providerName: "azure-device-code",
			config: &schema.Provider{
				Kind: "azure/device-code",
				Spec: map[string]interface{}{
					"tenant_id":       "tenant-123",
					"subscription_id": "sub-456",
					"location":        "eastus",
				},
			},
			expectError: false,
			checkProvider: func(t *testing.T, p *deviceCodeProvider) {
				assert.Equal(t, "tenant-123", p.tenantID)
				assert.Equal(t, "sub-456", p.subscriptionID)
				assert.Equal(t, "eastus", p.location)
				assert.Equal(t, defaultAzureClientID, p.clientID)
			},
		},
		{
			name:         "valid config without subscription ID",
			providerName: "azure-device-code",
			config: &schema.Provider{
				Kind: "azure/device-code",
				Spec: map[string]interface{}{
					"tenant_id": "tenant-123",
					"location":  "westus",
				},
			},
			expectError: false,
			checkProvider: func(t *testing.T, p *deviceCodeProvider) {
				assert.Equal(t, "tenant-123", p.tenantID)
				assert.Equal(t, "", p.subscriptionID)
				assert.Equal(t, "westus", p.location)
			},
		},
		{
			name:         "valid config without location",
			providerName: "azure-device-code",
			config: &schema.Provider{
				Kind: "azure/device-code",
				Spec: map[string]interface{}{
					"tenant_id":       "tenant-123",
					"subscription_id": "sub-456",
				},
			},
			expectError: false,
			checkProvider: func(t *testing.T, p *deviceCodeProvider) {
				assert.Equal(t, "tenant-123", p.tenantID)
				assert.Equal(t, "sub-456", p.subscriptionID)
				assert.Equal(t, "", p.location)
			},
		},
		{
			name:         "custom client ID",
			providerName: "azure-device-code",
			config: &schema.Provider{
				Kind: "azure/device-code",
				Spec: map[string]interface{}{
					"tenant_id": "tenant-123",
					"client_id": "custom-client-123",
				},
			},
			expectError: false,
			checkProvider: func(t *testing.T, p *deviceCodeProvider) {
				assert.Equal(t, "custom-client-123", p.clientID)
			},
		},
		{
			name:         "missing tenant ID",
			providerName: "azure-device-code",
			config: &schema.Provider{
				Kind: "azure/device-code",
				Spec: map[string]interface{}{
					"subscription_id": "sub-456",
				},
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderConfig,
		},
		{
			name:         "nil spec",
			providerName: "azure-device-code",
			config: &schema.Provider{
				Kind: "azure/device-code",
				Spec: nil,
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderConfig,
		},
		{
			name:         "empty spec",
			providerName: "azure-device-code",
			config: &schema.Provider{
				Kind: "azure/device-code",
				Spec: map[string]interface{}{},
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderConfig,
		},
		{
			name:         "nil config",
			providerName: "azure-device-code",
			config:       nil,
			expectError:  true,
			errorType:    errUtils.ErrInvalidProviderConfig,
		},
		{
			name:         "wrong provider kind",
			providerName: "azure-device-code",
			config: &schema.Provider{
				Kind: "azure/cli",
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
			provider, err := NewDeviceCodeProvider(tt.providerName, tt.config)

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
			assert.Equal(t, "azure/device-code", provider.Kind())

			if tt.checkProvider != nil {
				tt.checkProvider(t, provider)
			}
		})
	}
}

func TestDeviceCodeProvider_Kind(t *testing.T) {
	provider := &deviceCodeProvider{}
	assert.Equal(t, "azure/device-code", provider.Kind())
}

func TestDeviceCodeProvider_Name(t *testing.T) {
	tests := []struct {
		name     string
		provider *deviceCodeProvider
		expected string
	}{
		{
			name:     "provider with name",
			provider: &deviceCodeProvider{name: "my-azure-device-code"},
			expected: "my-azure-device-code",
		},
		{
			name:     "provider with empty name",
			provider: &deviceCodeProvider{name: ""},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.provider.Name())
		})
	}
}

func TestDeviceCodeProvider_PreAuthenticate(t *testing.T) {
	provider := &deviceCodeProvider{
		name: "test-device-code",
		config: &schema.Provider{
			Kind: "azure/device-code",
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

func TestDeviceCodeProvider_Validate(t *testing.T) {
	tests := []struct {
		name        string
		provider    *deviceCodeProvider
		expectError bool
		errorType   error
	}{
		{
			name: "valid provider",
			provider: &deviceCodeProvider{
				tenantID: "tenant-123",
				clientID: "client-456",
			},
			expectError: false,
		},
		{
			name: "missing tenant ID",
			provider: &deviceCodeProvider{
				tenantID: "",
				clientID: "client-456",
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderConfig,
		},
		{
			name: "missing client ID",
			provider: &deviceCodeProvider{
				tenantID: "tenant-123",
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

func TestDeviceCodeProvider_Environment(t *testing.T) {
	tests := []struct {
		name        string
		provider    *deviceCodeProvider
		expectedEnv map[string]string
	}{
		{
			name: "all fields present",
			provider: &deviceCodeProvider{
				tenantID:       "tenant-123",
				subscriptionID: "sub-456",
				location:       "eastus",
			},
			expectedEnv: map[string]string{
				"AZURE_TENANT_ID":       "tenant-123",
				"AZURE_SUBSCRIPTION_ID": "sub-456",
				"AZURE_LOCATION":        "eastus",
			},
		},
		{
			name: "only tenant ID",
			provider: &deviceCodeProvider{
				tenantID:       "tenant-123",
				subscriptionID: "",
				location:       "",
			},
			expectedEnv: map[string]string{
				"AZURE_TENANT_ID": "tenant-123",
			},
		},
		{
			name: "tenant and subscription",
			provider: &deviceCodeProvider{
				tenantID:       "tenant-123",
				subscriptionID: "sub-456",
				location:       "",
			},
			expectedEnv: map[string]string{
				"AZURE_TENANT_ID":       "tenant-123",
				"AZURE_SUBSCRIPTION_ID": "sub-456",
			},
		},
		{
			name: "empty fields",
			provider: &deviceCodeProvider{
				tenantID:       "",
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

func TestDeviceCodeProvider_PrepareEnvironment(t *testing.T) {
	tests := []struct {
		name             string
		provider         *deviceCodeProvider
		inputEnv         map[string]string
		expectedContains map[string]string
		expectedMissing  []string
	}{
		{
			name: "basic environment preparation",
			provider: &deviceCodeProvider{
				tenantID:       "tenant-123",
				subscriptionID: "sub-456",
				location:       "eastus",
			},
			inputEnv: map[string]string{
				"HOME": "/home/user",
				"PATH": "/usr/bin",
			},
			expectedContains: map[string]string{
				"HOME":                  "/home/user",
				"PATH":                  "/usr/bin",
				"AZURE_SUBSCRIPTION_ID": "sub-456",
				"ARM_SUBSCRIPTION_ID":   "sub-456",
				"AZURE_TENANT_ID":       "tenant-123",
				"ARM_TENANT_ID":         "tenant-123",
				"AZURE_LOCATION":        "eastus",
				"ARM_LOCATION":          "eastus",
				"ARM_USE_CLI":           "true",
			},
		},
		{
			name: "clears conflicting Azure credential environment variables",
			provider: &deviceCodeProvider{
				tenantID:       "tenant-123",
				subscriptionID: "sub-456",
			},
			inputEnv: map[string]string{
				"AZURE_CLIENT_ID":                "conflicting-client-id",
				"AZURE_CLIENT_SECRET":            "conflicting-secret",
				"AZURE_CLIENT_CERTIFICATE_PATH":  "/path/to/cert",
				"ARM_CLIENT_ID":                  "conflicting-arm-client",
				"ARM_CLIENT_SECRET":              "conflicting-arm-secret",
				"AWS_ACCESS_KEY_ID":              "AKIAIOSFODNN7EXAMPLE",
				"GOOGLE_APPLICATION_CREDENTIALS": "/path/to/gcp/creds.json",
				"HOME":                           "/home/user",
			},
			expectedContains: map[string]string{
				"HOME":                  "/home/user",
				"AZURE_SUBSCRIPTION_ID": "sub-456",
				"ARM_SUBSCRIPTION_ID":   "sub-456",
				"AZURE_TENANT_ID":       "tenant-123",
				"ARM_TENANT_ID":         "tenant-123",
				"ARM_USE_CLI":           "true",
			},
			expectedMissing: []string{
				"AZURE_CLIENT_ID",
				"AZURE_CLIENT_SECRET",
				"AZURE_CLIENT_CERTIFICATE_PATH",
				"ARM_CLIENT_ID",
				"ARM_CLIENT_SECRET",
				"AWS_ACCESS_KEY_ID",
				"GOOGLE_APPLICATION_CREDENTIALS",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := tt.provider.PrepareEnvironment(ctx, tt.inputEnv)

			require.NoError(t, err)

			// Check that expected variables are present with correct values.
			for key, expectedValue := range tt.expectedContains {
				assert.Equal(t, expectedValue, result[key], "Expected %s=%s", key, expectedValue)
			}

			// Check that unwanted variables are not present.
			for _, key := range tt.expectedMissing {
				_, exists := result[key]
				assert.False(t, exists, "Expected %s to be missing", key)
			}

			// Verify ARM_USE_CLI is always set to "true".
			assert.Equal(t, "true", result["ARM_USE_CLI"], "ARM_USE_CLI should always be true")
		})
	}
}

func TestDeviceCodeProvider_Logout(t *testing.T) {
	mockStorage := newMockCacheStorage()

	provider := &deviceCodeProvider{
		name:         "test-provider",
		tenantID:     "tenant-123",
		cacheStorage: mockStorage,
	}

	// Create a cached token first.
	now := time.Now().UTC()
	err := provider.saveCachedToken("test-token", "Bearer", now.Add(1*time.Hour), "graph-token", now.Add(2*time.Hour))
	require.NoError(t, err)

	// Verify token exists.
	token, expiresAt, graphToken, graphExpiresAt, err := provider.loadCachedToken()
	require.NoError(t, err)
	assert.Equal(t, "test-token", token)
	assert.False(t, expiresAt.IsZero())
	assert.Equal(t, "graph-token", graphToken)
	assert.False(t, graphExpiresAt.IsZero())

	// Logout should delete the token.
	ctx := context.Background()
	err = provider.Logout(ctx)
	require.NoError(t, err)

	// Verify token was deleted.
	token, expiresAt, graphToken, graphExpiresAt, err = provider.loadCachedToken()
	require.NoError(t, err)
	assert.Empty(t, token)
	assert.True(t, expiresAt.IsZero())
	assert.Empty(t, graphToken)
	assert.True(t, graphExpiresAt.IsZero())
}

func TestDeviceCodeProvider_GetFilesDisplayPath(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		expected     string
	}{
		{
			name:         "basic provider name",
			providerName: "my-azure-provider",
			expected:     "~/.azure/atmos/my-azure-provider",
		},
		{
			name:         "provider with hyphens",
			providerName: "prod-azure-device-code",
			expected:     "~/.azure/atmos/prod-azure-device-code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &deviceCodeProvider{
				name: tt.providerName,
			}
			assert.Equal(t, tt.expected, provider.GetFilesDisplayPath())
		})
	}
}

func TestSpinnerModel(t *testing.T) {
	t.Run("Init returns Tick command", func(t *testing.T) {
		model := newSpinnerModel()
		cmd := model.Init()
		assert.NotNil(t, cmd)
	})

	t.Run("Update handles authCompleteMsg success", func(t *testing.T) {
		model := newSpinnerModel()
		now := time.Now().UTC()

		msg := authCompleteMsg{
			token:     "test-token",
			expiresOn: now,
			err:       nil,
		}

		updatedModel, cmd := model.Update(msg)
		assert.NotNil(t, cmd)

		m := updatedModel.(spinnerModel)
		assert.True(t, m.quitting)
		assert.Equal(t, "test-token", m.token)
		assert.Equal(t, now, m.expiresOn)
		assert.NoError(t, m.authErr)
	})

	t.Run("Update handles authCompleteMsg error", func(t *testing.T) {
		model := newSpinnerModel()
		testErr := errUtils.ErrAuthenticationFailed

		msg := authCompleteMsg{
			token:     "",
			expiresOn: time.Time{},
			err:       testErr,
		}

		updatedModel, cmd := model.Update(msg)
		assert.NotNil(t, cmd)

		m := updatedModel.(spinnerModel)
		assert.True(t, m.quitting)
		assert.Empty(t, m.token)
		assert.ErrorIs(t, m.authErr, testErr)
	})

	t.Run("Update handles Ctrl+C", func(t *testing.T) {
		model := newSpinnerModel()

		msg := tea.KeyMsg{Type: tea.KeyCtrlC}

		updatedModel, cmd := model.Update(msg)
		assert.NotNil(t, cmd)

		m := updatedModel.(spinnerModel)
		assert.True(t, m.quitting)
		assert.Error(t, m.authErr)
		assert.ErrorIs(t, m.authErr, errUtils.ErrAuthenticationFailed)
	})

	t.Run("View shows spinner when not quitting", func(t *testing.T) {
		model := newSpinnerModel()
		view := model.View()
		assert.Contains(t, view, "Waiting for authentication...")
	})

	t.Run("View shows success when quitting without error", func(t *testing.T) {
		model := newSpinnerModel()
		model.quitting = true
		model.authErr = nil
		view := model.View()
		assert.Contains(t, view, "✓")
		assert.Contains(t, view, "Authentication successful!")
	})

	t.Run("View shows empty string when quitting with error", func(t *testing.T) {
		model := newSpinnerModel()
		model.quitting = true
		model.authErr = errUtils.ErrAuthenticationFailed
		view := model.View()
		assert.Empty(t, view)
	})
}

func TestDeviceCodeProvider_FieldExtraction(t *testing.T) {
	// Test that fields are correctly extracted from spec.
	config := &schema.Provider{
		Kind: "azure/device-code",
		Spec: map[string]interface{}{
			"tenant_id":       "extracted-tenant",
			"subscription_id": "extracted-sub",
			"location":        "extracted-location",
			"client_id":       "extracted-client",
		},
	}

	provider, err := NewDeviceCodeProvider("test-provider", config)
	require.NoError(t, err)

	assert.Equal(t, "extracted-tenant", provider.tenantID)
	assert.Equal(t, "extracted-sub", provider.subscriptionID)
	assert.Equal(t, "extracted-location", provider.location)
	assert.Equal(t, "extracted-client", provider.clientID)
}

func TestDeviceCodeProvider_OptionalFields(t *testing.T) {
	// Test that optional fields default to empty strings or defaults.
	config := &schema.Provider{
		Kind: "azure/device-code",
		Spec: map[string]interface{}{
			"tenant_id": "tenant-123",
			// subscription_id, location, client_id omitted.
		},
	}

	provider, err := NewDeviceCodeProvider("test-provider", config)
	require.NoError(t, err)

	assert.Equal(t, "tenant-123", provider.tenantID)
	assert.Equal(t, "", provider.subscriptionID)
	assert.Equal(t, "", provider.location)
	assert.Equal(t, defaultAzureClientID, provider.clientID)
}

func TestDeviceCodeProvider_SpecFieldTypes(t *testing.T) {
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
				"client_id":       "client-789",
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
				Kind: "azure/device-code",
				Spec: tt.spec,
			}

			provider, err := NewDeviceCodeProvider("test", config)

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
