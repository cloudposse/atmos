package aws

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/schema"
)

func validEKSConfig(name string) *integrations.IntegrationConfig {
	return &integrations.IntegrationConfig{
		Name: name,
		Config: &schema.Integration{
			Kind: integrations.KindAWSEKS,
			Via: &schema.IntegrationVia{
				Identity: "dev-admin",
			},
			Spec: &schema.IntegrationSpec{
				Cluster: &schema.EKSCluster{
					Name:   "dev-cluster",
					Region: "us-east-2",
					Alias:  "dev-eks",
				},
			},
		},
	}
}

func TestNewEKSIntegration_Success(t *testing.T) {
	config := validEKSConfig("test-eks")

	integration, err := NewEKSIntegration(config)
	require.NoError(t, err)
	require.NotNil(t, integration)

	eksIntegration, ok := integration.(*EKSIntegration)
	require.True(t, ok)
	assert.Equal(t, "test-eks", eksIntegration.name)
	assert.Equal(t, "dev-admin", eksIntegration.identity)
	assert.Equal(t, "dev-cluster", eksIntegration.cluster.Name)
	assert.Equal(t, "us-east-2", eksIntegration.cluster.Region)
	assert.Equal(t, "dev-eks", eksIntegration.cluster.Alias)
}

func TestNewEKSIntegration_NilConfig(t *testing.T) {
	_, err := NewEKSIntegration(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationNotFound)
}

func TestNewEKSIntegration_NilConfigConfig(t *testing.T) {
	config := &integrations.IntegrationConfig{
		Name:   "test-eks",
		Config: nil,
	}

	_, err := NewEKSIntegration(config)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationNotFound)
}

func TestNewEKSIntegration_NoCluster(t *testing.T) {
	config := &integrations.IntegrationConfig{
		Name: "test-eks",
		Config: &schema.Integration{
			Kind: integrations.KindAWSEKS,
			Via: &schema.IntegrationVia{
				Identity: "dev-admin",
			},
			Spec: &schema.IntegrationSpec{
				// No cluster configured.
			},
		},
	}

	_, err := NewEKSIntegration(config)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationFailed)
	assert.Contains(t, err.Error(), "no cluster configured")
}

func TestNewEKSIntegration_NoClusterName(t *testing.T) {
	config := &integrations.IntegrationConfig{
		Name: "test-eks",
		Config: &schema.Integration{
			Kind: integrations.KindAWSEKS,
			Spec: &schema.IntegrationSpec{
				Cluster: &schema.EKSCluster{
					Region: "us-east-2",
					// No Name.
				},
			},
		},
	}

	_, err := NewEKSIntegration(config)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationFailed)
	assert.Contains(t, err.Error(), "no cluster name configured")
}

func TestNewEKSIntegration_NoRegion(t *testing.T) {
	config := &integrations.IntegrationConfig{
		Name: "test-eks",
		Config: &schema.Integration{
			Kind: integrations.KindAWSEKS,
			Spec: &schema.IntegrationSpec{
				Cluster: &schema.EKSCluster{
					Name: "dev-cluster",
					// No Region.
				},
			},
		},
	}

	_, err := NewEKSIntegration(config)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationFailed)
	assert.Contains(t, err.Error(), "no region configured")
}

func TestNewEKSIntegration_NoVia(t *testing.T) {
	config := &integrations.IntegrationConfig{
		Name: "test-eks",
		Config: &schema.Integration{
			Kind: integrations.KindAWSEKS,
			Spec: &schema.IntegrationSpec{
				Cluster: &schema.EKSCluster{
					Name:   "dev-cluster",
					Region: "us-east-2",
				},
			},
		},
	}

	integration, err := NewEKSIntegration(config)
	require.NoError(t, err)
	require.NotNil(t, integration)

	eksIntegration, ok := integration.(*EKSIntegration)
	require.True(t, ok)
	assert.Equal(t, "", eksIntegration.identity)
}

func TestNewEKSIntegration_InvalidMode(t *testing.T) {
	config := &integrations.IntegrationConfig{
		Name: "test-eks",
		Config: &schema.Integration{
			Kind: integrations.KindAWSEKS,
			Spec: &schema.IntegrationSpec{
				Cluster: &schema.EKSCluster{
					Name:   "dev-cluster",
					Region: "us-east-2",
					Kubeconfig: &schema.KubeconfigSettings{
						Mode: "abc",
					},
				},
			},
		},
	}

	_, err := NewEKSIntegration(config)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationFailed)
	assert.Contains(t, err.Error(), "invalid kubeconfig mode")
}

func TestNewEKSIntegration_InvalidUpdateMode(t *testing.T) {
	config := &integrations.IntegrationConfig{
		Name: "test-eks",
		Config: &schema.Integration{
			Kind: integrations.KindAWSEKS,
			Spec: &schema.IntegrationSpec{
				Cluster: &schema.EKSCluster{
					Name:   "dev-cluster",
					Region: "us-east-2",
					Kubeconfig: &schema.KubeconfigSettings{
						Update: "invalid",
					},
				},
			},
		},
	}

	_, err := NewEKSIntegration(config)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationFailed)
	assert.Contains(t, err.Error(), "invalid kubeconfig update mode")
}

func TestNewEKSIntegration_ValidKubeconfigSettings(t *testing.T) {
	config := &integrations.IntegrationConfig{
		Name: "test-eks",
		Config: &schema.Integration{
			Kind: integrations.KindAWSEKS,
			Spec: &schema.IntegrationSpec{
				Cluster: &schema.EKSCluster{
					Name:   "dev-cluster",
					Region: "us-east-2",
					Kubeconfig: &schema.KubeconfigSettings{
						Path:   "/tmp/kubeconfig",
						Mode:   "0644",
						Update: "replace",
					},
				},
			},
		},
	}

	integration, err := NewEKSIntegration(config)
	require.NoError(t, err)
	require.NotNil(t, integration)

	eksIntegration, ok := integration.(*EKSIntegration)
	require.True(t, ok)
	assert.Equal(t, "/tmp/kubeconfig", eksIntegration.cluster.Kubeconfig.Path)
	assert.Equal(t, "0644", eksIntegration.cluster.Kubeconfig.Mode)
	assert.Equal(t, "replace", eksIntegration.cluster.Kubeconfig.Update)
}

func TestEKSIntegration_Kind(t *testing.T) {
	integration := &EKSIntegration{
		name:     "test",
		identity: "dev-admin",
		cluster: &schema.EKSCluster{
			Name:   "dev-cluster",
			Region: "us-east-2",
		},
	}

	assert.Equal(t, integrations.KindAWSEKS, integration.Kind())
}

func TestEKSIntegration_GetIdentity(t *testing.T) {
	integration := &EKSIntegration{
		name:     "test",
		identity: "dev-admin",
		cluster: &schema.EKSCluster{
			Name:   "dev-cluster",
			Region: "us-east-2",
		},
	}

	assert.Equal(t, "dev-admin", integration.GetIdentity())
}

func TestEKSIntegration_GetCluster(t *testing.T) {
	cluster := &schema.EKSCluster{
		Name:   "dev-cluster",
		Region: "us-east-2",
		Alias:  "dev-eks",
	}

	integration := &EKSIntegration{
		name:     "test",
		identity: "dev-admin",
		cluster:  cluster,
	}

	assert.Equal(t, cluster, integration.GetCluster())
	assert.Equal(t, "dev-cluster", integration.GetCluster().Name)
	assert.Equal(t, "us-east-2", integration.GetCluster().Region)
}

func TestEKSIntegration_Environment_DefaultPath(t *testing.T) {
	integration := &EKSIntegration{
		name:     "test",
		identity: "dev-admin",
		cluster: &schema.EKSCluster{
			Name:   "dev-cluster",
			Region: "us-east-2",
		},
	}

	env, err := integration.Environment()
	require.NoError(t, err)
	require.Contains(t, env, "KUBECONFIG")
	require.Contains(t, env, "KUBE_CONFIG_PATH")
	// Default path should contain "atmos/kube/config".
	assert.Contains(t, env["KUBECONFIG"], "kube")
	assert.Contains(t, env["KUBECONFIG"], "config")
	// Both variables should point to the same path.
	assert.Equal(t, env["KUBECONFIG"], env["KUBE_CONFIG_PATH"])
}

func TestEKSIntegration_Environment_CustomPath(t *testing.T) {
	integration := &EKSIntegration{
		name:     "test",
		identity: "dev-admin",
		cluster: &schema.EKSCluster{
			Name:   "dev-cluster",
			Region: "us-east-2",
			Kubeconfig: &schema.KubeconfigSettings{
				Path: "/tmp/custom/kubeconfig",
			},
		},
	}

	env, err := integration.Environment()
	require.NoError(t, err)
	require.Contains(t, env, "KUBECONFIG")
	require.Contains(t, env, "KUBE_CONFIG_PATH")
	assert.Equal(t, "/tmp/custom/kubeconfig", env["KUBECONFIG"])
	assert.Equal(t, "/tmp/custom/kubeconfig", env["KUBE_CONFIG_PATH"])
}

func TestEKSIntegration_Cleanup_NoAlias(t *testing.T) {
	// Cleanup without alias should be a no-op (can't determine context name).
	integration := &EKSIntegration{
		name:     "test",
		identity: "dev-admin",
		cluster: &schema.EKSCluster{
			Name:   "dev-cluster",
			Region: "us-east-2",
			// No alias.
		},
	}

	err := integration.Cleanup(t.Context())
	require.NoError(t, err)
}

func TestEKSIntegration_Cleanup_NonexistentFile(t *testing.T) {
	// Cleanup with nonexistent kubeconfig should succeed (idempotent).
	integration := &EKSIntegration{
		name:     "test",
		identity: "dev-admin",
		cluster: &schema.EKSCluster{
			Name:   "dev-cluster",
			Region: "us-east-2",
			Alias:  "dev-eks",
			Kubeconfig: &schema.KubeconfigSettings{
				Path: t.TempDir() + "/nonexistent/kubeconfig",
			},
		},
	}

	// Should not error - file doesn't exist, nothing to clean up.
	err := integration.Cleanup(t.Context())
	require.NoError(t, err)
}

func TestEKSIntegrationRegistration(t *testing.T) {
	assert.True(t, integrations.IsRegistered(integrations.KindAWSEKS))
}

func TestEKSIntegrationRegistration_ViaRegistry(t *testing.T) {
	// Verify EKS integration can be created through the registry.
	config := validEKSConfig("test-via-registry")

	integration, err := integrations.Create(config)
	require.NoError(t, err)
	require.NotNil(t, integration)
	assert.Equal(t, integrations.KindAWSEKS, integration.Kind())
}

func TestEKSIntegration_ResolveKubeconfigSettings_NilKubeconfig(t *testing.T) {
	integration := &EKSIntegration{
		name: "test",
		cluster: &schema.EKSCluster{
			Name:   "dev-cluster",
			Region: "us-east-2",
			// No Kubeconfig settings.
		},
	}

	path, mode, update := integration.resolveKubeconfigSettings()
	assert.Equal(t, "", path)
	assert.Equal(t, "", mode)
	assert.Equal(t, "", update)
}

func TestEKSIntegration_ResolveKubeconfigSettings_WithSettings(t *testing.T) {
	integration := &EKSIntegration{
		name: "test",
		cluster: &schema.EKSCluster{
			Name:   "dev-cluster",
			Region: "us-east-2",
			Kubeconfig: &schema.KubeconfigSettings{
				Path:   "/custom/path",
				Mode:   "0644",
				Update: "replace",
			},
		},
	}

	path, mode, update := integration.resolveKubeconfigSettings()
	assert.Equal(t, "/custom/path", path)
	assert.Equal(t, "0644", mode)
	assert.Equal(t, "replace", update)
}
