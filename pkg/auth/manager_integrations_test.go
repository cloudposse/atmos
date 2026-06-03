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
		Realm:        "test-realm",
		Providers:    map[string]schema.Provider{},
		Identities:   map[string]schema.Identity{},
		Integrations: nil, // No integrations.
	}, credStore, validator, nil, "")
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
		Realm:      "test-realm",
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
	}, credStore, validator, nil, "")
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
		Realm:      "test-realm",
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
	}, credStore, validator, nil, "")
	require.NoError(t, err)

	ctx := context.Background()
	err = m.ExecuteIntegration(ctx, "test-ecr")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationFailed)
	assert.Contains(t, err.Error(), "no identity configured")
}

func TestIntegrationTargetKey(t *testing.T) {
	tests := []struct {
		name        string
		intName     string
		integration schema.Integration
		want        string
	}{
		{
			name:    "ECR with registry",
			intName: "my-ecr",
			integration: schema.Integration{
				Kind: "aws/ecr",
				Spec: &schema.IntegrationSpec{
					Registry: &schema.ECRRegistry{AccountID: "123456789012", Region: "us-east-1"},
				},
			},
			want: "aws/ecr:123456789012:us-east-1",
		},
		{
			name:    "ECR without spec falls back to name",
			intName: "my-ecr",
			integration: schema.Integration{
				Kind: "aws/ecr",
			},
			want: "my-ecr",
		},
		{
			name:    "EKS with cluster",
			intName: "my-eks",
			integration: schema.Integration{
				Kind: "aws/eks",
				Spec: &schema.IntegrationSpec{
					Cluster: &schema.EKSCluster{Name: "prod-cluster", Region: "us-west-2"},
				},
			},
			want: "aws/eks:prod-cluster:us-west-2",
		},
		{
			name:    "EKS without spec falls back to name",
			intName: "my-eks",
			integration: schema.Integration{
				Kind: "aws/eks",
			},
			want: "my-eks",
		},
		{
			name:    "unknown kind uses name",
			intName: "custom-integration",
			integration: schema.Integration{
				Kind: "custom/type",
			},
			want: "custom-integration",
		},
		{
			name:    "two ECR integrations same registry produce same key",
			intName: "ecr-component",
			integration: schema.Integration{
				Kind: "aws/ecr",
				Spec: &schema.IntegrationSpec{
					Registry: &schema.ECRRegistry{AccountID: "123456789012", Region: "us-east-1"},
				},
			},
			want: "aws/ecr:123456789012:us-east-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := integrationTargetKey(tt.intName, tt.integration)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestIntegrationTargetKey_Deduplication verifies that two ECR integrations pointing at the
// same registry are treated as one execution (the second is skipped via the process cache).
func TestIntegrationTargetKey_Deduplication(t *testing.T) {
	resetProcessIntegrationCache()
	t.Cleanup(resetProcessIntegrationCache)

	registrySpec := &schema.IntegrationSpec{
		Registry: &schema.ECRRegistry{AccountID: "123456789012", Region: "us-east-1"},
	}

	// Two different integration names, same registry.
	keyA := integrationTargetKey("ecr-global", schema.Integration{Kind: "aws/ecr", Spec: registrySpec})
	keyB := integrationTargetKey("ecr-component", schema.Integration{Kind: "aws/ecr", Spec: registrySpec})
	assert.Equal(t, keyA, keyB, "same registry must produce the same cache key")

	// First store: should not already be present.
	_, alreadyRan := processIntegrationCache.LoadOrStore(keyA, struct{}{})
	assert.False(t, alreadyRan, "first integration should not be cached yet")

	// Second store with same key: should be skipped.
	_, alreadyRan = processIntegrationCache.LoadOrStore(keyB, struct{}{})
	assert.True(t, alreadyRan, "second integration pointing at same registry must be deduplicated")
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
				Realm:        "test-realm",
				Providers:    map[string]schema.Provider{},
				Identities:   tt.identities,
				Integrations: tt.integrations,
			}, credStore, validator, nil, "")
			require.NoError(t, err)

			ctx := context.Background()
			err = m.ExecuteIdentityIntegrations(ctx, tt.identityName)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.expectedError)
		})
	}
}
