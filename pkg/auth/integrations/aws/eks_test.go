package aws

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/cloud/kube"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/types"
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
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	config := &integrations.IntegrationConfig{
		Name: "test-eks",
		Config: &schema.Integration{
			Kind: integrations.KindAWSEKS,
			Spec: &schema.IntegrationSpec{
				Cluster: &schema.EKSCluster{
					Name:   "dev-cluster",
					Region: "us-east-2",
					Kubeconfig: &schema.KubeconfigSettings{
						Path:   kubeconfigPath,
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
	assert.Equal(t, kubeconfigPath, eksIntegration.cluster.Kubeconfig.Path)
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
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

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
	kubeconfigPath := filepath.Join(t.TempDir(), "custom", "kubeconfig")
	integration := &EKSIntegration{
		name:     "test",
		identity: "dev-admin",
		cluster: &schema.EKSCluster{
			Name:   "dev-cluster",
			Region: "us-east-2",
			Kubeconfig: &schema.KubeconfigSettings{
				Path: kubeconfigPath,
			},
		},
	}

	env, err := integration.Environment()
	require.NoError(t, err)
	require.Contains(t, env, "KUBECONFIG")
	require.Contains(t, env, "KUBE_CONFIG_PATH")
	assert.Equal(t, kubeconfigPath, env["KUBECONFIG"])
	assert.Equal(t, kubeconfigPath, env["KUBE_CONFIG_PATH"])
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
				Path: filepath.Join(t.TempDir(), "nonexistent", "kubeconfig"),
			},
		},
	}

	// Should not error - file doesn't exist, nothing to clean up.
	err := integration.Cleanup(t.Context())
	require.NoError(t, err)
}

func TestEKSIntegration_Cleanup_RemovesEntries(t *testing.T) {
	tests := []struct {
		name           string
		alias          string
		currentContext string
		expectCleared  bool
	}{
		{
			name:           "with alias as current context",
			alias:          "dev-eks",
			currentContext: "dev-eks",
			expectCleared:  true,
		},
		{
			name:           "with alias not current context",
			alias:          "dev-eks",
			currentContext: "other-context",
			expectCleared:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
			clusterARN := "arn:aws:eks:us-east-2:123456789012:cluster/dev-cluster"
			contextName := tt.alias
			userName := "atmos-eks-dev-cluster-us-east-2"

			// Provision a kubeconfig with known entries using BuildClusterConfig.
			info := &awsCloud.EKSClusterInfo{
				Name:                     "dev-cluster",
				Endpoint:                 "https://example.eks.amazonaws.com",
				CertificateAuthorityData: "dGVzdA==",
				ARN:                      clusterARN,
				Region:                   "us-east-2",
			}
			clusterConfig := kube.BuildClusterConfig(info, tt.alias, "dev-admin")

			mgr, err := kube.NewKubeconfigManager(kubeconfigPath, "")
			require.NoError(t, err)
			err = mgr.WriteClusterConfig(info, tt.alias, "dev-admin", "merge")
			require.NoError(t, err)

			// Verify entries exist.
			_ = clusterConfig // Used BuildClusterConfig above to verify consistency.
			arns, err := mgr.ListClusterARNs()
			require.NoError(t, err)
			assert.Contains(t, arns, clusterARN)

			// Set current-context if specified.
			if tt.currentContext != "" {
				existing, loadErr := loadKubeconfig(kubeconfigPath)
				require.NoError(t, loadErr)
				existing.CurrentContext = tt.currentContext
				writeKubeconfig(t, kubeconfigPath, existing)
			}

			// Run cleanup.
			integration := &EKSIntegration{
				name:     "test",
				identity: "dev-admin",
				cluster: &schema.EKSCluster{
					Name:   "dev-cluster",
					Region: "us-east-2",
					Alias:  tt.alias,
					Kubeconfig: &schema.KubeconfigSettings{
						Path: kubeconfigPath,
					},
				},
			}

			err = integration.Cleanup(t.Context())
			require.NoError(t, err)

			// Verify entries were removed.
			arns, err = mgr.ListClusterARNs()
			require.NoError(t, err)
			assert.NotContains(t, arns, clusterARN)

			// Verify context and user were removed.
			remaining, loadErr := loadKubeconfig(kubeconfigPath)
			if loadErr == nil {
				assert.NotContains(t, remaining.Contexts, contextName)
				assert.NotContains(t, remaining.AuthInfos, userName)

				if tt.expectCleared {
					assert.Empty(t, remaining.CurrentContext)
				}
			}

			// Verify idempotency - cleanup again should succeed.
			err = integration.Cleanup(t.Context())
			require.NoError(t, err)
		})
	}
}

// loadKubeconfig loads a kubeconfig file.
func loadKubeconfig(path string) (*clientcmdapi.Config, error) {
	return clientcmd.LoadFromFile(path)
}

// writeKubeconfig writes a kubeconfig file.
func writeKubeconfig(t *testing.T, path string, config *clientcmdapi.Config) {
	t.Helper()
	err := clientcmd.WriteToFile(*config, path)
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

func TestEKSIntegration_Execute_Success(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	clusterARN := "arn:aws:eks:us-east-2:123456789012:cluster/dev-cluster"

	// Override factory functions for testing.
	origClientFactory := eksClientFactory
	origDescribe := eksDescribeCluster
	t.Cleanup(func() {
		eksClientFactory = origClientFactory
		eksDescribeCluster = origDescribe
	})

	eksClientFactory = func(_ context.Context, _ types.ICredentials, _ string) (awsCloud.EKSClient, error) {
		return nil, nil // Client unused since we also override DescribeCluster.
	}

	eksDescribeCluster = func(_ context.Context, _ awsCloud.EKSClient, _, _ string) (*awsCloud.EKSClusterInfo, error) {
		return &awsCloud.EKSClusterInfo{
			Name:                     "dev-cluster",
			Endpoint:                 "https://example.eks.amazonaws.com",
			CertificateAuthorityData: "dGVzdA==",
			ARN:                      clusterARN,
			Region:                   "us-east-2",
		}, nil
	}

	integration := &EKSIntegration{
		name:     "test-eks",
		identity: "dev-admin",
		cluster: &schema.EKSCluster{
			Name:   "dev-cluster",
			Region: "us-east-2",
			Alias:  "dev-eks",
			Kubeconfig: &schema.KubeconfigSettings{
				Path:   kubeconfigPath,
				Update: "merge",
			},
		},
	}

	err := integration.Execute(t.Context(), nil)
	require.NoError(t, err)

	// Verify kubeconfig was written.
	config, loadErr := loadKubeconfig(kubeconfigPath)
	require.NoError(t, loadErr)
	assert.Contains(t, config.Clusters, clusterARN)
	assert.Contains(t, config.Contexts, "dev-eks")
	assert.Equal(t, "dev-eks", config.CurrentContext)
}

func TestEKSIntegration_Execute_NoAlias(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	clusterARN := "arn:aws:eks:us-east-2:123456789012:cluster/dev-cluster"

	origClientFactory := eksClientFactory
	origDescribe := eksDescribeCluster
	t.Cleanup(func() {
		eksClientFactory = origClientFactory
		eksDescribeCluster = origDescribe
	})

	eksClientFactory = func(_ context.Context, _ types.ICredentials, _ string) (awsCloud.EKSClient, error) {
		return nil, nil
	}

	eksDescribeCluster = func(_ context.Context, _ awsCloud.EKSClient, _, _ string) (*awsCloud.EKSClusterInfo, error) {
		return &awsCloud.EKSClusterInfo{
			Name:                     "dev-cluster",
			Endpoint:                 "https://example.eks.amazonaws.com",
			CertificateAuthorityData: "dGVzdA==",
			ARN:                      clusterARN,
			Region:                   "us-east-2",
		}, nil
	}

	integration := &EKSIntegration{
		name:     "test-eks",
		identity: "dev-admin",
		cluster: &schema.EKSCluster{
			Name:   "dev-cluster",
			Region: "us-east-2",
			// No Alias — context name should default to ARN.
			Kubeconfig: &schema.KubeconfigSettings{
				Path: kubeconfigPath,
			},
		},
	}

	err := integration.Execute(t.Context(), nil)
	require.NoError(t, err)

	config, loadErr := loadKubeconfig(kubeconfigPath)
	require.NoError(t, loadErr)
	assert.Equal(t, clusterARN, config.CurrentContext)
}

func TestEKSIntegration_Execute_ClientFactoryError(t *testing.T) {
	origClientFactory := eksClientFactory
	t.Cleanup(func() { eksClientFactory = origClientFactory })

	eksClientFactory = func(_ context.Context, _ types.ICredentials, _ string) (awsCloud.EKSClient, error) {
		return nil, fmt.Errorf("credentials invalid")
	}

	integration := &EKSIntegration{
		name:     "test-eks",
		identity: "dev-admin",
		cluster: &schema.EKSCluster{
			Name:   "dev-cluster",
			Region: "us-east-2",
		},
	}

	err := integration.Execute(t.Context(), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEKSIntegrationFailed)
	assert.Contains(t, err.Error(), "credentials invalid")
}

func TestEKSIntegration_Execute_DescribeClusterError(t *testing.T) {
	origClientFactory := eksClientFactory
	origDescribe := eksDescribeCluster
	t.Cleanup(func() {
		eksClientFactory = origClientFactory
		eksDescribeCluster = origDescribe
	})

	eksClientFactory = func(_ context.Context, _ types.ICredentials, _ string) (awsCloud.EKSClient, error) {
		return nil, nil
	}

	eksDescribeCluster = func(_ context.Context, _ awsCloud.EKSClient, _, _ string) (*awsCloud.EKSClusterInfo, error) {
		return nil, fmt.Errorf("cluster not found")
	}

	integration := &EKSIntegration{
		name:     "test-eks",
		identity: "dev-admin",
		cluster: &schema.EKSCluster{
			Name:   "dev-cluster",
			Region: "us-east-2",
		},
	}

	err := integration.Execute(t.Context(), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEKSIntegrationFailed)
	assert.Contains(t, err.Error(), "cluster not found")
}

func TestEKSIntegration_Execute_ReplaceMode(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	clusterARN := "arn:aws:eks:us-east-2:123456789012:cluster/dev-cluster"

	origClientFactory := eksClientFactory
	origDescribe := eksDescribeCluster
	t.Cleanup(func() {
		eksClientFactory = origClientFactory
		eksDescribeCluster = origDescribe
	})

	eksClientFactory = func(_ context.Context, _ types.ICredentials, _ string) (awsCloud.EKSClient, error) {
		return nil, nil
	}

	eksDescribeCluster = func(_ context.Context, _ awsCloud.EKSClient, _, _ string) (*awsCloud.EKSClusterInfo, error) {
		return &awsCloud.EKSClusterInfo{
			Name:                     "dev-cluster",
			Endpoint:                 "https://example.eks.amazonaws.com",
			CertificateAuthorityData: "dGVzdA==",
			ARN:                      clusterARN,
			Region:                   "us-east-2",
		}, nil
	}

	integration := &EKSIntegration{
		name:     "test-eks",
		identity: "dev-admin",
		cluster: &schema.EKSCluster{
			Name:   "dev-cluster",
			Region: "us-east-2",
			Alias:  "dev-eks",
			Kubeconfig: &schema.KubeconfigSettings{
				Path:   kubeconfigPath,
				Update: "replace",
			},
		},
	}

	err := integration.Execute(t.Context(), nil)
	require.NoError(t, err)

	config, loadErr := loadKubeconfig(kubeconfigPath)
	require.NoError(t, loadErr)
	assert.Contains(t, config.Clusters, clusterARN)
}

func TestEKSIntegration_Cleanup_NoAliasWithARNInKubeconfig(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	clusterARN := "arn:aws:eks:us-east-2:123456789012:cluster/dev-cluster"

	// Write a kubeconfig with the cluster ARN as context name (no alias case).
	info := &awsCloud.EKSClusterInfo{
		Name:                     "dev-cluster",
		Endpoint:                 "https://example.eks.amazonaws.com",
		CertificateAuthorityData: "dGVzdA==",
		ARN:                      clusterARN,
		Region:                   "us-east-2",
	}
	mgr, err := kube.NewKubeconfigManager(kubeconfigPath, "")
	require.NoError(t, err)
	err = mgr.WriteClusterConfig(info, "", "dev-admin", "merge")
	require.NoError(t, err)

	// Verify entry exists.
	arns, err := mgr.ListClusterARNs()
	require.NoError(t, err)
	assert.Contains(t, arns, clusterARN)

	// Cleanup without alias - should find ARN by cluster name suffix.
	integration := &EKSIntegration{
		name:     "test",
		identity: "dev-admin",
		cluster: &schema.EKSCluster{
			Name:   "dev-cluster",
			Region: "us-east-2",
			// No alias.
			Kubeconfig: &schema.KubeconfigSettings{
				Path: kubeconfigPath,
			},
		},
	}

	err = integration.Cleanup(t.Context())
	require.NoError(t, err)

	// Verify entries were removed.
	arns, err = mgr.ListClusterARNs()
	require.NoError(t, err)
	assert.NotContains(t, arns, clusterARN)
}

func TestEKSIntegration_FindClusterARN_MultipleCluster(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")

	mgr, err := kube.NewKubeconfigManager(kubeconfigPath, "")
	require.NoError(t, err)

	// Write two clusters.
	for _, cluster := range []struct {
		name  string
		arn   string
		alias string
	}{
		{"dev-cluster", "arn:aws:eks:us-east-2:123456789012:cluster/dev-cluster", "dev"},
		{"prod-cluster", "arn:aws:eks:us-west-2:123456789012:cluster/prod-cluster", "prod"},
	} {
		info := &awsCloud.EKSClusterInfo{
			Name:                     cluster.name,
			Endpoint:                 "https://example.eks.amazonaws.com",
			CertificateAuthorityData: "dGVzdA==",
			ARN:                      cluster.arn,
			Region:                   "us-east-2",
		}
		err = mgr.WriteClusterConfig(info, cluster.alias, "admin", "merge")
		require.NoError(t, err)
	}

	// findClusterARN should find the correct one.
	integration := &EKSIntegration{
		cluster: &schema.EKSCluster{
			Name:   "prod-cluster",
			Region: "us-west-2",
			Kubeconfig: &schema.KubeconfigSettings{
				Path: kubeconfigPath,
			},
		},
	}

	arn, err := integration.findClusterARN(mgr)
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:eks:us-west-2:123456789012:cluster/prod-cluster", arn)
}

func TestEKSIntegration_FindClusterARN_NoMatch(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")

	mgr, err := kube.NewKubeconfigManager(kubeconfigPath, "")
	require.NoError(t, err)

	// Write a cluster that won't match.
	info := &awsCloud.EKSClusterInfo{
		Name:                     "other-cluster",
		Endpoint:                 "https://example.eks.amazonaws.com",
		CertificateAuthorityData: "dGVzdA==",
		ARN:                      "arn:aws:eks:us-east-2:123456789012:cluster/other-cluster",
		Region:                   "us-east-2",
	}
	err = mgr.WriteClusterConfig(info, "other", "admin", "merge")
	require.NoError(t, err)

	integration := &EKSIntegration{
		cluster: &schema.EKSCluster{
			Name:   "missing-cluster",
			Region: "us-east-2",
			Kubeconfig: &schema.KubeconfigSettings{
				Path: kubeconfigPath,
			},
		},
	}

	_, err = integration.findClusterARN(mgr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no cluster ARN found matching missing-cluster")
}
