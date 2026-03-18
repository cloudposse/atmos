package aws

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewECRPublicIntegration_Success(t *testing.T) {
	config := &integrations.IntegrationConfig{
		Name: "test-ecr-public",
		Config: &schema.Integration{
			Kind: integrations.KindAWSECRPublic,
			Via: &schema.IntegrationVia{
				Identity: "dev-admin",
			},
			Spec: &schema.IntegrationSpec{},
		},
	}

	integration, err := NewECRPublicIntegration(config)
	require.NoError(t, err)
	require.NotNil(t, integration)

	ecrPublicIntegration, ok := integration.(*ECRPublicIntegration)
	require.True(t, ok)
	assert.Equal(t, "test-ecr-public", ecrPublicIntegration.name)
	assert.Equal(t, "dev-admin", ecrPublicIntegration.identity)
}

func TestNewECRPublicIntegration_MinimalConfig(t *testing.T) {
	// Minimal config: just kind, no via, no spec.
	config := &integrations.IntegrationConfig{
		Name: "test-ecr-public",
		Config: &schema.Integration{
			Kind: integrations.KindAWSECRPublic,
		},
	}

	integration, err := NewECRPublicIntegration(config)
	require.NoError(t, err)
	require.NotNil(t, integration)

	ecrPublicIntegration, ok := integration.(*ECRPublicIntegration)
	require.True(t, ok)
	assert.Equal(t, "", ecrPublicIntegration.identity)
}

func TestNewECRPublicIntegration_NilConfig(t *testing.T) {
	_, err := NewECRPublicIntegration(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationNotFound)
}

func TestNewECRPublicIntegration_NilConfigConfig(t *testing.T) {
	config := &integrations.IntegrationConfig{
		Name:   "test-ecr-public",
		Config: nil,
	}

	_, err := NewECRPublicIntegration(config)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationNotFound)
}

func TestNewECRPublicIntegration_NoVia(t *testing.T) {
	// Integration without via is valid - identity is optional.
	config := &integrations.IntegrationConfig{
		Name: "test-ecr-public",
		Config: &schema.Integration{
			Kind: integrations.KindAWSECRPublic,
			Spec: &schema.IntegrationSpec{},
		},
	}

	integration, err := NewECRPublicIntegration(config)
	require.NoError(t, err)
	require.NotNil(t, integration)

	ecrPublicIntegration, ok := integration.(*ECRPublicIntegration)
	require.True(t, ok)
	assert.Equal(t, "", ecrPublicIntegration.identity)
}

func TestNewECRPublicIntegration_WithValidRegion(t *testing.T) {
	// User specifies a valid region in spec.registry — should succeed.
	config := &integrations.IntegrationConfig{
		Name: "test-ecr-public",
		Config: &schema.Integration{
			Kind: integrations.KindAWSECRPublic,
			Via: &schema.IntegrationVia{
				Identity: "dev-admin",
			},
			Spec: &schema.IntegrationSpec{
				Registry: &schema.ECRRegistry{
					Region: "us-east-1",
				},
			},
		},
	}

	integration, err := NewECRPublicIntegration(config)
	require.NoError(t, err)
	require.NotNil(t, integration)
}

func TestNewECRPublicIntegration_WithInvalidRegion(t *testing.T) {
	// User specifies an invalid region — should fail validation.
	config := &integrations.IntegrationConfig{
		Name: "test-ecr-public",
		Config: &schema.Integration{
			Kind: integrations.KindAWSECRPublic,
			Via: &schema.IntegrationVia{
				Identity: "dev-admin",
			},
			Spec: &schema.IntegrationSpec{
				Registry: &schema.ECRRegistry{
					Region: "eu-west-1",
				},
			},
		},
	}

	_, err := NewECRPublicIntegration(config)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationFailed)
	assert.Contains(t, err.Error(), "eu-west-1")
}

func TestNewECRPublicIntegration_WithChinaRegion(t *testing.T) {
	// China regions are not supported for ECR Public.
	config := &integrations.IntegrationConfig{
		Name: "test-ecr-public",
		Config: &schema.Integration{
			Kind: integrations.KindAWSECRPublic,
			Spec: &schema.IntegrationSpec{
				Registry: &schema.ECRRegistry{
					Region: "cn-north-1",
				},
			},
		},
	}

	_, err := NewECRPublicIntegration(config)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationFailed)
}

func TestECRPublicIntegration_Kind(t *testing.T) {
	integration := &ECRPublicIntegration{
		name:     "test",
		identity: "dev-admin",
	}

	assert.Equal(t, integrations.KindAWSECRPublic, integration.Kind())
}

func TestECRPublicIntegration_GetIdentity(t *testing.T) {
	integration := &ECRPublicIntegration{
		name:     "test",
		identity: "dev-admin",
	}

	assert.Equal(t, "dev-admin", integration.GetIdentity())
}

func TestECRPublicIntegration_GetIdentity_Empty(t *testing.T) {
	integration := &ECRPublicIntegration{
		name:     "test",
		identity: "",
	}

	assert.Equal(t, "", integration.GetIdentity())
}

func TestECRPublicIntegration_Execute_NilCredentials(t *testing.T) {
	integration := &ECRPublicIntegration{
		name:     "test",
		identity: "dev-admin",
	}

	ctx := context.Background()
	err := integration.Execute(ctx, nil)

	// Execute should fail with nil credentials because it can't get auth token.
	require.Error(t, err)
}

func TestECRPublicIntegrationRegistration(t *testing.T) {
	// Verify that the ECR Public integration is registered.
	assert.True(t, integrations.IsRegistered(integrations.KindAWSECRPublic))
}
