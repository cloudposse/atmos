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

func TestNewECRIntegration_Success(t *testing.T) {
	config := &integrations.IntegrationConfig{
		Name: "test-ecr",
		Config: &schema.Integration{
			Kind: integrations.KindAWSECR,
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
	}

	integration, err := NewECRIntegration(config)
	require.NoError(t, err)
	require.NotNil(t, integration)

	ecrIntegration, ok := integration.(*ECRIntegration)
	require.True(t, ok)
	assert.Equal(t, "test-ecr", ecrIntegration.name)
	assert.Equal(t, "dev-admin", ecrIntegration.identity)
	assert.Equal(t, "123456789012", ecrIntegration.registry.AccountID)
	assert.Equal(t, "us-east-1", ecrIntegration.registry.Region)
}

func TestNewECRIntegration_NilConfig(t *testing.T) {
	_, err := NewECRIntegration(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationNotFound)
}

func TestNewECRIntegration_NilConfigConfig(t *testing.T) {
	config := &integrations.IntegrationConfig{
		Name:   "test-ecr",
		Config: nil,
	}

	_, err := NewECRIntegration(config)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationNotFound)
}

func TestNewECRIntegration_NoRegistry(t *testing.T) {
	config := &integrations.IntegrationConfig{
		Name: "test-ecr",
		Config: &schema.Integration{
			Kind: integrations.KindAWSECR,
			Via: &schema.IntegrationVia{
				Identity: "dev-admin",
			},
			Spec: &schema.IntegrationSpec{
				// No registry configured.
			},
		},
	}

	_, err := NewECRIntegration(config)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationFailed)
	assert.Contains(t, err.Error(), "no registry configured")
}

func TestNewECRIntegration_NoAccountID(t *testing.T) {
	config := &integrations.IntegrationConfig{
		Name: "test-ecr",
		Config: &schema.Integration{
			Kind: integrations.KindAWSECR,
			Spec: &schema.IntegrationSpec{
				Registry: &schema.ECRRegistry{
					Region: "us-east-1",
					// No AccountID.
				},
			},
		},
	}

	_, err := NewECRIntegration(config)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationFailed)
	assert.Contains(t, err.Error(), "no account_id configured")
}

func TestNewECRIntegration_NoRegion(t *testing.T) {
	config := &integrations.IntegrationConfig{
		Name: "test-ecr",
		Config: &schema.Integration{
			Kind: integrations.KindAWSECR,
			Spec: &schema.IntegrationSpec{
				Registry: &schema.ECRRegistry{
					AccountID: "123456789012",
					// No Region.
				},
			},
		},
	}

	_, err := NewECRIntegration(config)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationFailed)
	assert.Contains(t, err.Error(), "no region configured")
}

func TestNewECRIntegration_NoVia(t *testing.T) {
	// Integration without via is valid - identity is optional.
	config := &integrations.IntegrationConfig{
		Name: "test-ecr",
		Config: &schema.Integration{
			Kind: integrations.KindAWSECR,
			Spec: &schema.IntegrationSpec{
				Registry: &schema.ECRRegistry{
					AccountID: "123456789012",
					Region:    "us-east-1",
				},
			},
		},
	}

	integration, err := NewECRIntegration(config)
	require.NoError(t, err)
	require.NotNil(t, integration)

	ecrIntegration, ok := integration.(*ECRIntegration)
	require.True(t, ok)
	assert.Equal(t, "", ecrIntegration.identity)
}

func TestECRIntegration_Kind(t *testing.T) {
	integration := &ECRIntegration{
		name:     "test",
		identity: "dev-admin",
		registry: &schema.ECRRegistry{
			AccountID: "123456789012",
			Region:    "us-east-1",
		},
	}

	assert.Equal(t, integrations.KindAWSECR, integration.Kind())
}

func TestECRIntegration_GetIdentity(t *testing.T) {
	integration := &ECRIntegration{
		name:     "test",
		identity: "dev-admin",
		registry: &schema.ECRRegistry{
			AccountID: "123456789012",
			Region:    "us-east-1",
		},
	}

	assert.Equal(t, "dev-admin", integration.GetIdentity())
}

func TestECRIntegration_GetIdentity_Empty(t *testing.T) {
	integration := &ECRIntegration{
		name:     "test",
		identity: "",
		registry: &schema.ECRRegistry{
			AccountID: "123456789012",
			Region:    "us-east-1",
		},
	}

	assert.Equal(t, "", integration.GetIdentity())
}

func TestECRIntegration_GetRegistry(t *testing.T) {
	registry := &schema.ECRRegistry{
		AccountID: "123456789012",
		Region:    "us-east-1",
	}

	integration := &ECRIntegration{
		name:     "test",
		identity: "dev-admin",
		registry: registry,
	}

	assert.Equal(t, registry, integration.GetRegistry())
	assert.Equal(t, "123456789012", integration.GetRegistry().AccountID)
	assert.Equal(t, "us-east-1", integration.GetRegistry().Region)
}

func TestECRIntegration_Execute_NilCredentials(t *testing.T) {
	integration := &ECRIntegration{
		name:     "test",
		identity: "dev-admin",
		registry: &schema.ECRRegistry{
			AccountID: "123456789012",
			Region:    "us-east-1",
		},
	}

	ctx := context.Background()
	err := integration.Execute(ctx, nil)

	// Execute should fail with nil credentials because it can't get auth token.
	require.Error(t, err)
}

func TestECRIntegrationRegistration(t *testing.T) {
	// Verify that the ECR integration is registered.
	assert.True(t, integrations.IsRegistered(integrations.KindAWSECR))
}
