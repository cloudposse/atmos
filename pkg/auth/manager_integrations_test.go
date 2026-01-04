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

func TestManager_ExecuteIdentityIntegrations_Errors(t *testing.T) {
	tests := []struct {
		name          string
		identities    map[string]schema.Identity
		integrations  map[string]schema.Integration
		identityName  string
		expectedError error
	}{
		{
			name:          "identity not found",
			identities:    map[string]schema.Identity{},
			integrations:  nil,
			identityName:  "non-existent",
			expectedError: errUtils.ErrIdentityNotFound,
		},
		{
			name: "nil integrations map",
			identities: map[string]schema.Identity{
				"dev-admin": {Kind: "aws/user"},
			},
			integrations:  nil,
			identityName:  "dev-admin",
			expectedError: errUtils.ErrNoLinkedIntegrations,
		},
		{
			name: "empty integrations map",
			identities: map[string]schema.Identity{
				"dev-admin": {Kind: "aws/user"},
			},
			integrations:  map[string]schema.Integration{},
			identityName:  "dev-admin",
			expectedError: errUtils.ErrNoLinkedIntegrations,
		},
		{
			name: "no matching identity in integrations",
			identities: map[string]schema.Identity{
				"dev-admin": {Kind: "aws/user"},
			},
			integrations: map[string]schema.Integration{
				"test-ecr": {
					Kind: "aws/ecr",
					Via:  &schema.IntegrationVia{Identity: "other-identity"},
				},
			},
			identityName:  "dev-admin",
			expectedError: errUtils.ErrNoLinkedIntegrations,
		},
		// Note: auto_provision filtering is tested implicitly through successful integration execution.
		// Testing auto_provision=false through ExecuteIdentityIntegrations requires authentication
		// which adds complexity. The filtering behavior is verified through behavior when integrations
		// are properly configured.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credStore := credentials.NewCredentialStore()
			validator := validation.NewValidator()

			m, err := NewAuthManager(&schema.AuthConfig{
				Providers:    map[string]schema.Provider{},
				Identities:   tt.identities,
				Integrations: tt.integrations,
			}, credStore, validator, nil)
			require.NoError(t, err)

			ctx := context.Background()
			err = m.ExecuteIdentityIntegrations(ctx, tt.identityName)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.expectedError)
		})
	}
}
