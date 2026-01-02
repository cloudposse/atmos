package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestManager_GetIntegration(t *testing.T) {
	tests := []struct {
		name            string
		integrations    map[string]schema.Integration
		integrationName string
		expectError     bool
		expectedErrIs   error
	}{
		{
			name:            "nil integrations map",
			integrations:    nil,
			integrationName: "test",
			expectError:     true,
			expectedErrIs:   errUtils.ErrIntegrationNotFound,
		},
		{
			name:            "empty integrations map",
			integrations:    map[string]schema.Integration{},
			integrationName: "test",
			expectError:     true,
			expectedErrIs:   errUtils.ErrIntegrationNotFound,
		},
		{
			name: "integration not found",
			integrations: map[string]schema.Integration{
				"other": {Kind: "aws/ecr"},
			},
			integrationName: "test",
			expectError:     true,
			expectedErrIs:   errUtils.ErrIntegrationNotFound,
		},
		{
			name: "integration found",
			integrations: map[string]schema.Integration{
				"test-ecr": {
					Kind: "aws/ecr",
					Via: &schema.IntegrationVia{
						Identity: "dev-admin",
					},
					Spec: &schema.IntegrationSpec{
						Registry: &schema.ECRRegistry{
							AccountID: "123456789012",
							Region:    "us-east-1",
						},
					},
				},
			},
			integrationName: "test-ecr",
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &manager{
				config: &schema.AuthConfig{
					Integrations: tt.integrations,
				},
			}

			integration, err := m.GetIntegration(tt.integrationName)

			if tt.expectError {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedErrIs)
				assert.Nil(t, integration)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, integration)
			}
		})
	}
}

func TestManager_GetIntegration_ReturnsCorrectData(t *testing.T) {
	expectedRegistry := &schema.ECRRegistry{
		AccountID: "123456789012",
		Region:    "us-east-1",
	}
	expectedVia := &schema.IntegrationVia{
		Identity: "dev-admin",
	}
	autoProvision := true
	expectedSpec := &schema.IntegrationSpec{
		AutoProvision: &autoProvision,
		Registry:      expectedRegistry,
	}

	m := &manager{
		config: &schema.AuthConfig{
			Integrations: map[string]schema.Integration{
				"my-ecr": {
					Kind: "aws/ecr",
					Via:  expectedVia,
					Spec: expectedSpec,
				},
			},
		},
	}

	integration, err := m.GetIntegration("my-ecr")
	require.NoError(t, err)
	require.NotNil(t, integration)

	assert.Equal(t, "aws/ecr", integration.Kind)
	assert.Equal(t, "dev-admin", integration.Via.Identity)
	assert.Equal(t, "123456789012", integration.Spec.Registry.AccountID)
	assert.Equal(t, "us-east-1", integration.Spec.Registry.Region)
	assert.True(t, *integration.Spec.AutoProvision)
}

func TestManager_findIntegrationsForIdentity(t *testing.T) {
	autoProvisionTrue := true
	autoProvisionFalse := false

	tests := []struct {
		name              string
		integrations      map[string]schema.Integration
		identityName      string
		autoProvisionOnly bool
		expected          []string
	}{
		{
			name:              "nil integrations",
			integrations:      nil,
			identityName:      "dev-admin",
			autoProvisionOnly: true,
			expected:          nil,
		},
		{
			name:              "empty integrations",
			integrations:      map[string]schema.Integration{},
			identityName:      "dev-admin",
			autoProvisionOnly: true,
			expected:          nil,
		},
		{
			name: "no matching identity",
			integrations: map[string]schema.Integration{
				"ecr1": {
					Kind: "aws/ecr",
					Via: &schema.IntegrationVia{
						Identity: "other-admin",
					},
				},
			},
			identityName:      "dev-admin",
			autoProvisionOnly: true,
			expected:          nil,
		},
		{
			name: "matching identity with auto_provision default (true)",
			integrations: map[string]schema.Integration{
				"ecr1": {
					Kind: "aws/ecr",
					Via: &schema.IntegrationVia{
						Identity: "dev-admin",
					},
					// Spec is nil, auto_provision defaults to true.
				},
			},
			identityName:      "dev-admin",
			autoProvisionOnly: true,
			expected:          []string{"ecr1"},
		},
		{
			name: "matching identity with auto_provision explicitly true",
			integrations: map[string]schema.Integration{
				"ecr1": {
					Kind: "aws/ecr",
					Via: &schema.IntegrationVia{
						Identity: "dev-admin",
					},
					Spec: &schema.IntegrationSpec{
						AutoProvision: &autoProvisionTrue,
					},
				},
			},
			identityName:      "dev-admin",
			autoProvisionOnly: true,
			expected:          []string{"ecr1"},
		},
		{
			name: "matching identity with auto_provision false",
			integrations: map[string]schema.Integration{
				"ecr1": {
					Kind: "aws/ecr",
					Via: &schema.IntegrationVia{
						Identity: "dev-admin",
					},
					Spec: &schema.IntegrationSpec{
						AutoProvision: &autoProvisionFalse,
					},
				},
			},
			identityName:      "dev-admin",
			autoProvisionOnly: true,
			expected:          nil, // Should be excluded because auto_provision is false.
		},
		{
			name: "matching identity with auto_provision false but autoProvisionOnly=false",
			integrations: map[string]schema.Integration{
				"ecr1": {
					Kind: "aws/ecr",
					Via: &schema.IntegrationVia{
						Identity: "dev-admin",
					},
					Spec: &schema.IntegrationSpec{
						AutoProvision: &autoProvisionFalse,
					},
				},
			},
			identityName:      "dev-admin",
			autoProvisionOnly: false, // Don't filter by auto_provision.
			expected:          []string{"ecr1"},
		},
		{
			name: "multiple integrations - mixed",
			integrations: map[string]schema.Integration{
				"ecr1": {
					Kind: "aws/ecr",
					Via: &schema.IntegrationVia{
						Identity: "dev-admin",
					},
					Spec: &schema.IntegrationSpec{
						AutoProvision: &autoProvisionTrue,
					},
				},
				"ecr2": {
					Kind: "aws/ecr",
					Via: &schema.IntegrationVia{
						Identity: "dev-admin",
					},
					Spec: &schema.IntegrationSpec{
						AutoProvision: &autoProvisionFalse,
					},
				},
				"ecr3": {
					Kind: "aws/ecr",
					Via: &schema.IntegrationVia{
						Identity: "other-admin",
					},
				},
			},
			identityName:      "dev-admin",
			autoProvisionOnly: true,
			expected:          []string{"ecr1"}, // Only ecr1 has matching identity AND auto_provision=true.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &manager{
				config: &schema.AuthConfig{
					Integrations: tt.integrations,
				},
			}

			result := m.findIntegrationsForIdentity(tt.identityName, tt.autoProvisionOnly)

			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.ElementsMatch(t, tt.expected, result)
			}
		})
	}
}

func TestManager_ExecuteIntegration_IntegrationNotFound(t *testing.T) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	m, err := NewAuthManager(&schema.AuthConfig{
		Providers:    map[string]schema.Provider{},
		Identities:   map[string]schema.Identity{},
		Integrations: nil, // No integrations.
	}, credStore, validator, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = m.ExecuteIntegration(ctx, "non-existent")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationNotFound)
}

func TestManager_ExecuteIntegration_NoIdentity(t *testing.T) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	m, err := NewAuthManager(&schema.AuthConfig{
		Providers:  map[string]schema.Provider{},
		Identities: map[string]schema.Identity{},
		Integrations: map[string]schema.Integration{
			"test-ecr": {
				Kind: "aws/ecr",
				// No Via configured.
				Spec: &schema.IntegrationSpec{
					Registry: &schema.ECRRegistry{
						AccountID: "123456789012",
						Region:    "us-east-1",
					},
				},
			},
		},
	}, credStore, validator, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = m.ExecuteIntegration(ctx, "test-ecr")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationFailed)
	assert.Contains(t, err.Error(), "no identity configured")
}

func TestManager_ExecuteIntegration_EmptyIdentity(t *testing.T) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	m, err := NewAuthManager(&schema.AuthConfig{
		Providers:  map[string]schema.Provider{},
		Identities: map[string]schema.Identity{},
		Integrations: map[string]schema.Integration{
			"test-ecr": {
				Kind: "aws/ecr",
				Via: &schema.IntegrationVia{
					Identity: "", // Empty identity.
				},
				Spec: &schema.IntegrationSpec{
					Registry: &schema.ECRRegistry{
						AccountID: "123456789012",
						Region:    "us-east-1",
					},
				},
			},
		},
	}, credStore, validator, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = m.ExecuteIntegration(ctx, "test-ecr")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationFailed)
	assert.Contains(t, err.Error(), "no identity configured")
}

func TestManager_ExecuteIdentityIntegrations_IdentityNotFound(t *testing.T) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	m, err := NewAuthManager(&schema.AuthConfig{
		Providers:    map[string]schema.Provider{},
		Identities:   map[string]schema.Identity{},
		Integrations: nil,
	}, credStore, validator, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = m.ExecuteIdentityIntegrations(ctx, "non-existent")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIdentityNotFound)
}

func TestManager_ExecuteIdentityIntegrations_NoLinkedIntegrations(t *testing.T) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	m, err := NewAuthManager(&schema.AuthConfig{
		Providers: map[string]schema.Provider{},
		Identities: map[string]schema.Identity{
			"dev-admin": {
				Kind: "aws/user",
			},
		},
		Integrations: map[string]schema.Integration{
			"test-ecr": {
				Kind: "aws/ecr",
				Via: &schema.IntegrationVia{
					Identity: "other-identity", // Different identity.
				},
			},
		},
	}, credStore, validator, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = m.ExecuteIdentityIntegrations(ctx, "dev-admin")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNoLinkedIntegrations)
}

func TestManager_triggerIntegrations_SkippedWithContextKey(t *testing.T) {
	m := &manager{
		config: &schema.AuthConfig{
			Integrations: map[string]schema.Integration{
				"test-ecr": {
					Kind: "aws/ecr",
					Via: &schema.IntegrationVia{
						Identity: "dev-admin",
					},
				},
			},
		},
	}

	// Create context with skipIntegrationsKey.
	ctx := context.WithValue(context.Background(), skipIntegrationsKey, true)

	// Should return immediately without executing integrations.
	// This is a no-op test - just verifying no panic occurs.
	m.triggerIntegrations(ctx, "dev-admin", nil)
}

func TestManager_triggerIntegrations_NoLinkedIntegrations(t *testing.T) {
	m := &manager{
		config: &schema.AuthConfig{
			Integrations: map[string]schema.Integration{
				"test-ecr": {
					Kind: "aws/ecr",
					Via: &schema.IntegrationVia{
						Identity: "other-identity",
					},
				},
			},
		},
	}

	ctx := context.Background()

	// Should return immediately without executing integrations (no linked integrations).
	// This is a no-op test - just verifying no panic occurs.
	m.triggerIntegrations(ctx, "dev-admin", nil)
}

func TestManager_executeIntegration_NilIntegrations(t *testing.T) {
	m := &manager{
		config: &schema.AuthConfig{
			Integrations: nil,
		},
	}

	ctx := context.Background()
	err := m.executeIntegration(ctx, "non-existent", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationNotFound)
}

func TestManager_executeIntegration_NotFound(t *testing.T) {
	m := &manager{
		config: &schema.AuthConfig{
			Integrations: map[string]schema.Integration{
				"other-integration": {Kind: "aws/ecr"},
			},
		},
	}

	ctx := context.Background()
	err := m.executeIntegration(ctx, "non-existent", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationNotFound)
}

func TestManager_findIntegrationsForIdentity_NilVia(t *testing.T) {
	m := &manager{
		config: &schema.AuthConfig{
			Integrations: map[string]schema.Integration{
				"no-via": {
					Kind: "aws/ecr",
					Via:  nil, // No Via configured.
				},
			},
		},
	}

	result := m.findIntegrationsForIdentity("dev-admin", true)
	assert.Nil(t, result) // Should not match because Via is nil.
}

func TestManager_GetIntegration_NilConfig(t *testing.T) {
	// Test with non-nil config but nil integrations map.
	m := &manager{
		config: &schema.AuthConfig{
			Integrations: nil,
		},
	}

	integration, err := m.GetIntegration("test")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationNotFound)
	assert.Nil(t, integration)
}

func TestManager_triggerIntegrations_NilIntegrations(t *testing.T) {
	m := &manager{
		config: &schema.AuthConfig{
			Integrations: nil,
		},
	}

	ctx := context.Background()

	// Should return immediately without panicking when integrations is nil.
	m.triggerIntegrations(ctx, "dev-admin", nil)
}

func TestManager_findIntegrationsForIdentity_WithAutoProvisionNil(t *testing.T) {
	// Test that auto_provision defaults to true when Spec.AutoProvision is nil.
	m := &manager{
		config: &schema.AuthConfig{
			Integrations: map[string]schema.Integration{
				"ecr1": {
					Kind: "aws/ecr",
					Via: &schema.IntegrationVia{
						Identity: "dev-admin",
					},
					Spec: &schema.IntegrationSpec{
						AutoProvision: nil, // nil should default to true.
						Registry: &schema.ECRRegistry{
							AccountID: "123456789012",
							Region:    "us-east-1",
						},
					},
				},
			},
		},
	}

	result := m.findIntegrationsForIdentity("dev-admin", true)
	assert.Equal(t, []string{"ecr1"}, result)
}

func TestManager_ExecuteIntegration_EmptyIntegrationsMap(t *testing.T) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	m, err := NewAuthManager(&schema.AuthConfig{
		Providers:    map[string]schema.Provider{},
		Identities:   map[string]schema.Identity{},
		Integrations: map[string]schema.Integration{}, // Empty map.
	}, credStore, validator, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = m.ExecuteIntegration(ctx, "test")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationNotFound)
}

func TestManager_ExecuteIdentityIntegrations_EmptyIntegrationsMap(t *testing.T) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	m, err := NewAuthManager(&schema.AuthConfig{
		Providers: map[string]schema.Provider{},
		Identities: map[string]schema.Identity{
			"dev-admin": {
				Kind: "aws/user",
			},
		},
		Integrations: map[string]schema.Integration{}, // Empty map.
	}, credStore, validator, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = m.ExecuteIdentityIntegrations(ctx, "dev-admin")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNoLinkedIntegrations)
}
