package aws

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/cloud/docker"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/types"
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

func TestECRIntegration_Execute_Success(t *testing.T) {
	dockerDir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dockerDir)

	origGetAuth := ecrGetAuthToken
	origDockerFactory := ecrDockerConfigFactory
	t.Cleanup(func() {
		ecrGetAuthToken = origGetAuth
		ecrDockerConfigFactory = origDockerFactory
	})

	ecrGetAuthToken = func(_ context.Context, _ types.ICredentials, _, _ string) (*awsCloud.ECRAuthResult, error) {
		return &awsCloud.ECRAuthResult{
			Username:  "AWS",
			Password:  "test-password",
			Registry:  "123456789012.dkr.ecr.us-east-1.amazonaws.com",
			ExpiresAt: time.Now().Add(12 * time.Hour),
		}, nil
	}

	ecrDockerConfigFactory = func() (*docker.ConfigManager, error) {
		return docker.NewConfigManager()
	}

	integration := &ECRIntegration{
		name:     "test-ecr",
		identity: "dev-admin",
		registry: &schema.ECRRegistry{
			AccountID: "123456789012",
			Region:    "us-east-1",
		},
	}

	ctx := context.Background()
	err := integration.Execute(ctx, nil)
	require.NoError(t, err)
}

func TestECRIntegration_Execute_AuthTokenError(t *testing.T) {
	dockerDir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dockerDir)

	origGetAuth := ecrGetAuthToken
	origDockerFactory := ecrDockerConfigFactory
	t.Cleanup(func() {
		ecrGetAuthToken = origGetAuth
		ecrDockerConfigFactory = origDockerFactory
	})

	ecrDockerConfigFactory = func() (*docker.ConfigManager, error) {
		return docker.NewConfigManager()
	}

	ecrGetAuthToken = func(_ context.Context, _ types.ICredentials, _, _ string) (*awsCloud.ECRAuthResult, error) {
		return nil, fmt.Errorf("no credentials available")
	}

	integration := &ECRIntegration{
		name:     "test-ecr",
		identity: "dev-admin",
		registry: &schema.ECRRegistry{
			AccountID: "123456789012",
			Region:    "us-east-1",
		},
	}

	err := integration.Execute(context.Background(), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrECRAuthFailed)
}

func TestECRIntegration_Execute_DockerConfigError(t *testing.T) {
	origDockerFactory := ecrDockerConfigFactory
	t.Cleanup(func() {
		ecrDockerConfigFactory = origDockerFactory
	})

	ecrDockerConfigFactory = func() (*docker.ConfigManager, error) {
		return nil, fmt.Errorf("docker config not available")
	}

	integration := &ECRIntegration{
		name:     "test-ecr",
		identity: "dev-admin",
		registry: &schema.ECRRegistry{
			AccountID: "123456789012",
			Region:    "us-east-1",
		},
	}

	err := integration.Execute(context.Background(), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationFailed)
}

func TestECRIntegration_Cleanup_Success(t *testing.T) {
	dockerDir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dockerDir)

	origDockerFactory := ecrDockerConfigFactory
	t.Cleanup(func() {
		ecrDockerConfigFactory = origDockerFactory
	})

	ecrDockerConfigFactory = func() (*docker.ConfigManager, error) {
		return docker.NewConfigManager()
	}

	// First write an auth entry.
	mgr, err := docker.NewConfigManager()
	require.NoError(t, err)
	err = mgr.WriteAuth("123456789012.dkr.ecr.us-east-1.amazonaws.com", "AWS", "test-password")
	require.NoError(t, err)

	// Verify entry exists.
	registries, err := mgr.GetAuthenticatedRegistries()
	require.NoError(t, err)
	assert.Contains(t, registries, "123456789012.dkr.ecr.us-east-1.amazonaws.com")

	// Now cleanup.
	integration := &ECRIntegration{
		name:     "test-ecr",
		identity: "dev-admin",
		registry: &schema.ECRRegistry{
			AccountID: "123456789012",
			Region:    "us-east-1",
		},
	}

	err = integration.Cleanup(context.Background())
	require.NoError(t, err)

	// Verify entry was removed.
	registries, err = mgr.GetAuthenticatedRegistries()
	require.NoError(t, err)
	assert.NotContains(t, registries, "123456789012.dkr.ecr.us-east-1.amazonaws.com")
}

func TestECRIntegration_Cleanup_DockerConfigError(t *testing.T) {
	origDockerFactory := ecrDockerConfigFactory
	t.Cleanup(func() {
		ecrDockerConfigFactory = origDockerFactory
	})

	ecrDockerConfigFactory = func() (*docker.ConfigManager, error) {
		return nil, fmt.Errorf("docker config not available")
	}

	integration := &ECRIntegration{
		name:     "test-ecr",
		identity: "dev-admin",
		registry: &schema.ECRRegistry{
			AccountID: "123456789012",
			Region:    "us-east-1",
		},
	}

	err := integration.Cleanup(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDockerConfigWrite)
}

func TestECRIntegration_Environment_Success(t *testing.T) {
	dockerDir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dockerDir)

	origDockerFactory := ecrDockerConfigFactory
	t.Cleanup(func() {
		ecrDockerConfigFactory = origDockerFactory
	})

	ecrDockerConfigFactory = func() (*docker.ConfigManager, error) {
		return docker.NewConfigManager()
	}

	integration := &ECRIntegration{
		name:     "test-ecr",
		identity: "dev-admin",
		registry: &schema.ECRRegistry{
			AccountID: "123456789012",
			Region:    "us-east-1",
		},
	}

	env, err := integration.Environment()
	require.NoError(t, err)
	require.Contains(t, env, "DOCKER_CONFIG")
	assert.Equal(t, dockerDir, env["DOCKER_CONFIG"])
}

func TestECRIntegration_Environment_DockerConfigError(t *testing.T) {
	origDockerFactory := ecrDockerConfigFactory
	t.Cleanup(func() {
		ecrDockerConfigFactory = origDockerFactory
	})

	ecrDockerConfigFactory = func() (*docker.ConfigManager, error) {
		return nil, fmt.Errorf("docker config not available")
	}

	integration := &ECRIntegration{
		name:     "test-ecr",
		identity: "dev-admin",
		registry: &schema.ECRRegistry{
			AccountID: "123456789012",
			Region:    "us-east-1",
		},
	}

	_, err := integration.Environment()
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDockerConfigWrite)
}

func TestECRIntegrationRegistration(t *testing.T) {
	// Verify that the ECR integration is registered.
	assert.True(t, integrations.IsRegistered(integrations.KindAWSECR))
}

func TestECRIntegration_Execute_Idempotent(t *testing.T) {
	// Running Execute twice should succeed (second write overwrites first).
	dockerDir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dockerDir)

	origGetAuth := ecrGetAuthToken
	origDockerFactory := ecrDockerConfigFactory
	t.Cleanup(func() {
		ecrGetAuthToken = origGetAuth
		ecrDockerConfigFactory = origDockerFactory
	})

	ecrGetAuthToken = func(_ context.Context, _ types.ICredentials, _, _ string) (*awsCloud.ECRAuthResult, error) {
		return &awsCloud.ECRAuthResult{
			Username:  "AWS",
			Password:  "test-password",
			Registry:  "123456789012.dkr.ecr.us-east-1.amazonaws.com",
			ExpiresAt: time.Now().Add(12 * time.Hour),
		}, nil
	}

	ecrDockerConfigFactory = func() (*docker.ConfigManager, error) {
		return docker.NewConfigManager()
	}

	integration := &ECRIntegration{
		name:     "test-ecr",
		identity: "dev-admin",
		registry: &schema.ECRRegistry{
			AccountID: "123456789012",
			Region:    "us-east-1",
		},
	}

	err := integration.Execute(context.Background(), nil)
	require.NoError(t, err)

	err = integration.Execute(context.Background(), nil)
	require.NoError(t, err)
}
