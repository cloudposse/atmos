package azure

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	"github.com/cloudposse/atmos/pkg/auth/cloud/docker"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewACRIntegration_Success(t *testing.T) {
	config := &integrations.IntegrationConfig{
		Name: "test-acr",
		Config: &schema.Integration{
			Kind: integrations.KindAzureACR,
			Via: &schema.IntegrationVia{
				Identity: "azure-dev",
			},
			Spec: &schema.IntegrationSpec{
				Registry: &schema.Registry{
					Name: "myregistry",
				},
			},
		},
	}

	integration, err := NewACRIntegration(config)
	require.NoError(t, err)
	require.NotNil(t, integration)

	acrIntegration, ok := integration.(*ACRIntegration)
	require.True(t, ok)
	assert.Equal(t, "test-acr", acrIntegration.name)
	assert.Equal(t, "azure-dev", acrIntegration.identity)
	assert.Equal(t, "myregistry", acrIntegration.registry.Name)
}

func TestNewACRIntegration_NilConfig(t *testing.T) {
	_, err := NewACRIntegration(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationNotFound)
}

func TestNewACRIntegration_NilConfigConfig(t *testing.T) {
	config := &integrations.IntegrationConfig{
		Name:   "test-acr",
		Config: nil,
	}

	_, err := NewACRIntegration(config)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationNotFound)
}

func TestNewACRIntegration_NoRegistry(t *testing.T) {
	config := &integrations.IntegrationConfig{
		Name: "test-acr",
		Config: &schema.Integration{
			Kind: integrations.KindAzureACR,
			Via: &schema.IntegrationVia{
				Identity: "azure-dev",
			},
			Spec: &schema.IntegrationSpec{
				// No registry configured.
			},
		},
	}

	_, err := NewACRIntegration(config)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationFailed)
	assert.Contains(t, err.Error(), "no registry configured")
}

func TestNewACRIntegration_NoName(t *testing.T) {
	config := &integrations.IntegrationConfig{
		Name: "test-acr",
		Config: &schema.Integration{
			Kind: integrations.KindAzureACR,
			Spec: &schema.IntegrationSpec{
				Registry: &schema.Registry{
					// No Name.
				},
			},
		},
	}

	_, err := NewACRIntegration(config)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationFailed)
	assert.Contains(t, err.Error(), "no registry name configured")
}

func TestNewACRIntegration_NoVia(t *testing.T) {
	// Integration without via is valid - identity is optional.
	config := &integrations.IntegrationConfig{
		Name: "test-acr",
		Config: &schema.Integration{
			Kind: integrations.KindAzureACR,
			Spec: &schema.IntegrationSpec{
				Registry: &schema.Registry{
					Name: "myregistry",
				},
			},
		},
	}

	integration, err := NewACRIntegration(config)
	require.NoError(t, err)
	require.NotNil(t, integration)

	acrIntegration, ok := integration.(*ACRIntegration)
	require.True(t, ok)
	assert.Equal(t, "", acrIntegration.identity)
}

func TestACRIntegration_Kind(t *testing.T) {
	integration := &ACRIntegration{
		name:     "test",
		identity: "azure-dev",
		registry: &schema.Registry{Name: "myregistry"},
	}

	assert.Equal(t, integrations.KindAzureACR, integration.Kind())
}

func TestACRIntegration_GetIdentity(t *testing.T) {
	integration := &ACRIntegration{
		name:     "test",
		identity: "azure-dev",
		registry: &schema.Registry{Name: "myregistry"},
	}

	assert.Equal(t, "azure-dev", integration.GetIdentity())
}

func TestACRIntegration_GetRegistry(t *testing.T) {
	registry := &schema.Registry{Name: "myregistry"}

	integration := &ACRIntegration{
		name:     "test",
		identity: "azure-dev",
		registry: registry,
	}

	assert.Equal(t, registry, integration.GetRegistry())
	assert.Equal(t, "myregistry", integration.GetRegistry().Name)
}

func TestACRIntegration_Execute_NilCredentials(t *testing.T) {
	integration := &ACRIntegration{
		name:     "test",
		identity: "azure-dev",
		registry: &schema.Registry{Name: "myregistry"},
	}

	err := integration.Execute(context.Background(), nil)

	// Execute should fail with nil credentials because it can't get auth token.
	require.Error(t, err)
}

func TestACRIntegration_Execute_Success(t *testing.T) {
	dockerDir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dockerDir)

	origGetAuth := acrGetAuthToken
	origDockerFactory := acrDockerConfigFactory
	t.Cleanup(func() {
		acrGetAuthToken = origGetAuth
		acrDockerConfigFactory = origDockerFactory
	})

	acrGetAuthToken = func(_ context.Context, _ types.ICredentials, _ string) (*azureCloud.ACRAuthResult, error) {
		return &azureCloud.ACRAuthResult{
			Username:  "00000000-0000-0000-0000-000000000000",
			Password:  "test-refresh-token",
			Registry:  "myregistry.azurecr.io",
			ExpiresAt: time.Now().Add(3 * time.Hour),
		}, nil
	}

	acrDockerConfigFactory = docker.NewConfigManager

	integration := &ACRIntegration{
		name:     "test-acr",
		identity: "azure-dev",
		registry: &schema.Registry{Name: "myregistry"},
	}

	err := integration.Execute(context.Background(), nil)
	require.NoError(t, err)
}

func TestACRIntegration_Execute_ZeroExpiresAt(t *testing.T) {
	dockerDir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dockerDir)

	origGetAuth := acrGetAuthToken
	origDockerFactory := acrDockerConfigFactory
	t.Cleanup(func() {
		acrGetAuthToken = origGetAuth
		acrDockerConfigFactory = origDockerFactory
	})

	acrGetAuthToken = func(_ context.Context, _ types.ICredentials, _ string) (*azureCloud.ACRAuthResult, error) {
		return &azureCloud.ACRAuthResult{
			Username: "00000000-0000-0000-0000-000000000000",
			Password: "test-refresh-token",
			Registry: "myregistry.azurecr.io",
			// ExpiresAt intentionally left zero (refresh token without a decodable exp claim).
		}, nil
	}

	acrDockerConfigFactory = docker.NewConfigManager

	integration := &ACRIntegration{
		name:     "test-acr",
		identity: "azure-dev",
		registry: &schema.Registry{Name: "myregistry"},
	}

	err := integration.Execute(context.Background(), nil)
	require.NoError(t, err)
}

func TestACRIntegration_Execute_AuthTokenError(t *testing.T) {
	dockerDir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dockerDir)

	origGetAuth := acrGetAuthToken
	origDockerFactory := acrDockerConfigFactory
	t.Cleanup(func() {
		acrGetAuthToken = origGetAuth
		acrDockerConfigFactory = origDockerFactory
	})

	acrDockerConfigFactory = docker.NewConfigManager

	acrGetAuthToken = func(_ context.Context, _ types.ICredentials, _ string) (*azureCloud.ACRAuthResult, error) {
		return nil, fmt.Errorf("no credentials available")
	}

	integration := &ACRIntegration{
		name:     "test-acr",
		identity: "azure-dev",
		registry: &schema.Registry{Name: "myregistry"},
	}

	err := integration.Execute(context.Background(), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrACRAuthFailed)
}

func TestACRIntegration_Execute_DockerConfigError(t *testing.T) {
	origDockerFactory := acrDockerConfigFactory
	t.Cleanup(func() {
		acrDockerConfigFactory = origDockerFactory
	})

	acrDockerConfigFactory = func() (*docker.ConfigManager, error) {
		return nil, fmt.Errorf("docker config not available")
	}

	integration := &ACRIntegration{
		name:     "test-acr",
		identity: "azure-dev",
		registry: &schema.Registry{Name: "myregistry"},
	}

	err := integration.Execute(context.Background(), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationFailed)
}

func TestACRIntegration_Cleanup_Success(t *testing.T) {
	dockerDir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dockerDir)

	origDockerFactory := acrDockerConfigFactory
	t.Cleanup(func() {
		acrDockerConfigFactory = origDockerFactory
	})

	acrDockerConfigFactory = docker.NewConfigManager

	// First write an auth entry.
	mgr, err := docker.NewConfigManager()
	require.NoError(t, err)
	err = mgr.WriteAuth("myregistry.azurecr.io", "00000000-0000-0000-0000-000000000000", "test-refresh-token")
	require.NoError(t, err)

	// Verify entry exists.
	registries, err := mgr.GetAuthenticatedRegistries()
	require.NoError(t, err)
	assert.Contains(t, registries, "myregistry.azurecr.io")

	// Now cleanup.
	integration := &ACRIntegration{
		name:     "test-acr",
		identity: "azure-dev",
		registry: &schema.Registry{Name: "myregistry"},
	}

	err = integration.Cleanup(context.Background())
	require.NoError(t, err)

	// Verify entry was removed.
	registries, err = mgr.GetAuthenticatedRegistries()
	require.NoError(t, err)
	assert.NotContains(t, registries, "myregistry.azurecr.io")
}

func TestACRIntegration_Cleanup_DockerConfigError(t *testing.T) {
	origDockerFactory := acrDockerConfigFactory
	t.Cleanup(func() {
		acrDockerConfigFactory = origDockerFactory
	})

	acrDockerConfigFactory = func() (*docker.ConfigManager, error) {
		return nil, fmt.Errorf("docker config not available")
	}

	integration := &ACRIntegration{
		name:     "test-acr",
		identity: "azure-dev",
		registry: &schema.Registry{Name: "myregistry"},
	}

	err := integration.Cleanup(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDockerConfigWrite)
}

func TestACRIntegration_Environment_Success(t *testing.T) {
	dockerDir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dockerDir)

	origDockerFactory := acrDockerConfigFactory
	t.Cleanup(func() {
		acrDockerConfigFactory = origDockerFactory
	})

	acrDockerConfigFactory = docker.NewConfigManager

	integration := &ACRIntegration{
		name:     "test-acr",
		identity: "azure-dev",
		registry: &schema.Registry{Name: "myregistry"},
	}

	env, err := integration.Environment()
	require.NoError(t, err)
	require.Contains(t, env, "DOCKER_CONFIG")
	assert.Equal(t, dockerDir, env["DOCKER_CONFIG"])
}

func TestACRIntegration_Environment_DockerConfigError(t *testing.T) {
	origDockerFactory := acrDockerConfigFactory
	t.Cleanup(func() {
		acrDockerConfigFactory = origDockerFactory
	})

	acrDockerConfigFactory = func() (*docker.ConfigManager, error) {
		return nil, fmt.Errorf("docker config not available")
	}

	integration := &ACRIntegration{
		name:     "test-acr",
		identity: "azure-dev",
		registry: &schema.Registry{Name: "myregistry"},
	}

	_, err := integration.Environment()
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDockerConfigWrite)
}

func TestACRIntegrationRegistration(t *testing.T) {
	assert.True(t, integrations.IsRegistered(integrations.KindAzureACR))
}

func TestACRIntegrationRegistration_ViaRegistry(t *testing.T) {
	config := &integrations.IntegrationConfig{
		Name: "test-via-registry",
		Config: &schema.Integration{
			Kind: integrations.KindAzureACR,
			Spec: &schema.IntegrationSpec{
				Registry: &schema.Registry{Name: "myregistry"},
			},
		},
	}

	integration, err := integrations.Create(config)
	require.NoError(t, err)
	require.NotNil(t, integration)
	assert.Equal(t, integrations.KindAzureACR, integration.Kind())
}

func TestACRIntegration_Execute_Idempotent(t *testing.T) {
	dockerDir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dockerDir)

	origGetAuth := acrGetAuthToken
	origDockerFactory := acrDockerConfigFactory
	t.Cleanup(func() {
		acrGetAuthToken = origGetAuth
		acrDockerConfigFactory = origDockerFactory
	})

	acrGetAuthToken = func(_ context.Context, _ types.ICredentials, _ string) (*azureCloud.ACRAuthResult, error) {
		return &azureCloud.ACRAuthResult{
			Username:  "00000000-0000-0000-0000-000000000000",
			Password:  "test-refresh-token",
			Registry:  "myregistry.azurecr.io",
			ExpiresAt: time.Now().Add(3 * time.Hour),
		}, nil
	}

	acrDockerConfigFactory = docker.NewConfigManager

	integration := &ACRIntegration{
		name:     "test-acr",
		identity: "azure-dev",
		registry: &schema.Registry{Name: "myregistry"},
	}

	err := integration.Execute(context.Background(), nil)
	require.NoError(t, err)

	err = integration.Execute(context.Background(), nil)
	require.NoError(t, err)
}
