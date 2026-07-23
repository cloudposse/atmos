package azure

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"

	errUtils "github.com/cloudposse/atmos/errors"
	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	"github.com/cloudposse/atmos/pkg/auth/cloud/kube"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func validAKSConfig(name string) *integrations.IntegrationConfig {
	return &integrations.IntegrationConfig{
		Name: name,
		Config: &schema.Integration{
			Kind: integrations.KindAzureAKS,
			Via: &schema.IntegrationVia{
				Identity: "azure-dev",
			},
			Spec: &schema.IntegrationSpec{
				Cluster: &schema.Cluster{
					Name:          "dev-cluster",
					ResourceGroup: "dev-rg",
					Alias:         "dev-aks",
				},
			},
		},
	}
}

func TestNewAKSIntegration_Success(t *testing.T) {
	config := validAKSConfig("test-aks")

	integration, err := NewAKSIntegration(config)
	require.NoError(t, err)
	require.NotNil(t, integration)

	aksIntegration, ok := integration.(*AKSIntegration)
	require.True(t, ok)
	assert.Equal(t, "test-aks", aksIntegration.name)
	assert.Equal(t, "azure-dev", aksIntegration.identity)
	assert.Equal(t, "dev-cluster", aksIntegration.cluster.Name)
	assert.Equal(t, "dev-rg", aksIntegration.cluster.ResourceGroup)
	assert.Equal(t, "dev-aks", aksIntegration.cluster.Alias)
}

func TestNewAKSIntegration_NilConfig(t *testing.T) {
	_, err := NewAKSIntegration(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationNotFound)
}

func TestNewAKSIntegration_NilConfigConfig(t *testing.T) {
	config := &integrations.IntegrationConfig{
		Name:   "test-aks",
		Config: nil,
	}

	_, err := NewAKSIntegration(config)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationNotFound)
}

func TestNewAKSIntegration_NoCluster(t *testing.T) {
	config := &integrations.IntegrationConfig{
		Name: "test-aks",
		Config: &schema.Integration{
			Kind: integrations.KindAzureAKS,
			Via: &schema.IntegrationVia{
				Identity: "azure-dev",
			},
			Spec: &schema.IntegrationSpec{
				// No cluster configured.
			},
		},
	}

	_, err := NewAKSIntegration(config)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationFailed)
	assert.Contains(t, err.Error(), "no cluster configured")
}

func TestNewAKSIntegration_NoClusterName(t *testing.T) {
	config := &integrations.IntegrationConfig{
		Name: "test-aks",
		Config: &schema.Integration{
			Kind: integrations.KindAzureAKS,
			Spec: &schema.IntegrationSpec{
				Cluster: &schema.Cluster{
					ResourceGroup: "dev-rg",
					// No Name.
				},
			},
		},
	}

	_, err := NewAKSIntegration(config)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationFailed)
	assert.Contains(t, err.Error(), "no cluster name configured")
}

func TestNewAKSIntegration_NoResourceGroup(t *testing.T) {
	config := &integrations.IntegrationConfig{
		Name: "test-aks",
		Config: &schema.Integration{
			Kind: integrations.KindAzureAKS,
			Spec: &schema.IntegrationSpec{
				Cluster: &schema.Cluster{
					Name: "dev-cluster",
					// No ResourceGroup.
				},
			},
		},
	}

	_, err := NewAKSIntegration(config)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationFailed)
	assert.Contains(t, err.Error(), "no resource_group configured")
}

func TestNewAKSIntegration_NoVia(t *testing.T) {
	config := &integrations.IntegrationConfig{
		Name: "test-aks",
		Config: &schema.Integration{
			Kind: integrations.KindAzureAKS,
			Spec: &schema.IntegrationSpec{
				Cluster: &schema.Cluster{
					Name:          "dev-cluster",
					ResourceGroup: "dev-rg",
				},
			},
		},
	}

	integration, err := NewAKSIntegration(config)
	require.NoError(t, err)
	require.NotNil(t, integration)

	aksIntegration, ok := integration.(*AKSIntegration)
	require.True(t, ok)
	assert.Equal(t, "", aksIntegration.identity)
}

func TestNewAKSIntegration_InvalidMode(t *testing.T) {
	config := &integrations.IntegrationConfig{
		Name: "test-aks",
		Config: &schema.Integration{
			Kind: integrations.KindAzureAKS,
			Spec: &schema.IntegrationSpec{
				Cluster: &schema.Cluster{
					Name:          "dev-cluster",
					ResourceGroup: "dev-rg",
					Kubeconfig: &schema.KubeconfigSettings{
						Mode: "abc",
					},
				},
			},
		},
	}

	_, err := NewAKSIntegration(config)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationFailed)
	assert.Contains(t, err.Error(), "invalid kubeconfig mode")
}

func TestNewAKSIntegration_InvalidUpdateMode(t *testing.T) {
	config := &integrations.IntegrationConfig{
		Name: "test-aks",
		Config: &schema.Integration{
			Kind: integrations.KindAzureAKS,
			Spec: &schema.IntegrationSpec{
				Cluster: &schema.Cluster{
					Name:          "dev-cluster",
					ResourceGroup: "dev-rg",
					Kubeconfig: &schema.KubeconfigSettings{
						Update: "invalid",
					},
				},
			},
		},
	}

	_, err := NewAKSIntegration(config)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIntegrationFailed)
	assert.Contains(t, err.Error(), "invalid kubeconfig update mode")
}

func TestAKSIntegration_Kind(t *testing.T) {
	integration := &AKSIntegration{
		name:     "test",
		identity: "azure-dev",
		cluster:  &schema.Cluster{Name: "dev-cluster", ResourceGroup: "dev-rg"},
	}

	assert.Equal(t, integrations.KindAzureAKS, integration.Kind())
}

func TestAKSIntegration_GetIdentity(t *testing.T) {
	integration := &AKSIntegration{
		name:     "test",
		identity: "azure-dev",
		cluster:  &schema.Cluster{Name: "dev-cluster", ResourceGroup: "dev-rg"},
	}

	assert.Equal(t, "azure-dev", integration.GetIdentity())
}

func TestAKSIntegration_GetCluster(t *testing.T) {
	cluster := &schema.Cluster{Name: "dev-cluster", ResourceGroup: "dev-rg", Alias: "dev-aks"}

	integration := &AKSIntegration{
		name:     "test",
		identity: "azure-dev",
		cluster:  cluster,
	}

	assert.Equal(t, cluster, integration.GetCluster())
}

func TestAKSIntegration_Environment_DefaultPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	integration := &AKSIntegration{
		name:     "test",
		identity: "azure-dev",
		cluster:  &schema.Cluster{Name: "dev-cluster", ResourceGroup: "dev-rg"},
	}

	env, err := integration.Environment()
	require.NoError(t, err)
	require.Contains(t, env, "KUBECONFIG")
	require.Contains(t, env, "KUBE_CONFIG_PATH")
	assert.Equal(t, env["KUBECONFIG"], env["KUBE_CONFIG_PATH"])
}

func TestAKSIntegration_ResolveSubscriptionID_ClusterOverride(t *testing.T) {
	integration := &AKSIntegration{
		cluster: &schema.Cluster{Name: "dev-cluster", ResourceGroup: "dev-rg", SubscriptionID: "cluster-sub"},
	}

	got := integration.resolveSubscriptionID(&types.AzureCredentials{SubscriptionID: "creds-sub"})
	assert.Equal(t, "cluster-sub", got)
}

func TestAKSIntegration_ResolveSubscriptionID_FromCreds(t *testing.T) {
	integration := &AKSIntegration{
		cluster: &schema.Cluster{Name: "dev-cluster", ResourceGroup: "dev-rg"},
	}

	got := integration.resolveSubscriptionID(&types.AzureCredentials{SubscriptionID: "creds-sub"})
	assert.Equal(t, "creds-sub", got)
}

func TestAKSIntegration_ResolveSubscriptionID_WrongCredType(t *testing.T) {
	integration := &AKSIntegration{
		cluster: &schema.Cluster{Name: "dev-cluster", ResourceGroup: "dev-rg"},
	}

	got := integration.resolveSubscriptionID(&types.AWSCredentials{})
	assert.Equal(t, "", got)
}

func TestAKSIntegration_Execute_Success(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	resourceID := "/subscriptions/sub-123/resourceGroups/dev-rg/providers/Microsoft.ContainerService/managedClusters/dev-cluster"

	origClientFactory := aksClientFactory
	origDescribe := aksDescribeCluster
	t.Cleanup(func() {
		aksClientFactory = origClientFactory
		aksDescribeCluster = origDescribe
	})

	aksClientFactory = func(_ context.Context, _ types.ICredentials, _ string) (azureCloud.AKSClient, error) {
		return nil, nil // Client unused since we also override DescribeCluster.
	}

	aksDescribeCluster = func(_ context.Context, _ azureCloud.AKSClient, _, _, _ string) (*azureCloud.AKSClusterInfo, error) {
		return &azureCloud.AKSClusterInfo{
			Name:                     "dev-cluster",
			ResourceGroup:            "dev-rg",
			ID:                       resourceID,
			Endpoint:                 "https://dev-cluster.hcp.eastus.azmk8s.io:443",
			CertificateAuthorityData: "dGVzdA==",
			ServerID:                 azureCloud.AKSServerAppID,
		}, nil
	}

	integration := &AKSIntegration{
		name:     "test-aks",
		identity: "azure-dev",
		cluster: &schema.Cluster{
			Name:          "dev-cluster",
			ResourceGroup: "dev-rg",
			Alias:         "dev-aks",
			Kubeconfig: &schema.KubeconfigSettings{
				Path:   kubeconfigPath,
				Update: "merge",
			},
		},
	}

	err := integration.Execute(context.Background(), &types.AzureCredentials{SubscriptionID: "sub-123"})
	require.NoError(t, err)

	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	require.NoError(t, err)
	assert.Contains(t, config.Clusters, resourceID)
	assert.Contains(t, config.Contexts, "dev-aks")
	assert.Equal(t, "dev-aks", config.CurrentContext)
}

func TestAKSIntegration_Execute_NonDefaultServerID(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	resourceID := "/subscriptions/sub-123/resourceGroups/dev-rg/providers/Microsoft.ContainerService/managedClusters/dev-cluster"

	origClientFactory := aksClientFactory
	origDescribe := aksDescribeCluster
	t.Cleanup(func() {
		aksClientFactory = origClientFactory
		aksDescribeCluster = origDescribe
	})

	aksClientFactory = func(_ context.Context, _ types.ICredentials, _ string) (azureCloud.AKSClient, error) {
		return nil, nil
	}

	aksDescribeCluster = func(_ context.Context, _ azureCloud.AKSClient, _, _, _ string) (*azureCloud.AKSClusterInfo, error) {
		return &azureCloud.AKSClusterInfo{
			Name:                     "dev-cluster",
			ResourceGroup:            "dev-rg",
			ID:                       resourceID,
			Endpoint:                 "https://dev-cluster.hcp.eastus.azmk8s.io:443",
			CertificateAuthorityData: "dGVzdA==",
			ServerID:                 "custom-legacy-server-app-id",
		}, nil
	}

	integration := &AKSIntegration{
		name:     "test-aks",
		identity: "azure-dev",
		cluster: &schema.Cluster{
			Name:          "dev-cluster",
			ResourceGroup: "dev-rg",
			Kubeconfig:    &schema.KubeconfigSettings{Path: kubeconfigPath},
		},
	}

	// Non-default server ID logs a warning but does not fail Execute.
	err := integration.Execute(context.Background(), &types.AzureCredentials{SubscriptionID: "sub-123"})
	require.NoError(t, err)
}

func TestAKSIntegration_Execute_ClientFactoryError(t *testing.T) {
	origClientFactory := aksClientFactory
	t.Cleanup(func() { aksClientFactory = origClientFactory })

	aksClientFactory = func(_ context.Context, _ types.ICredentials, _ string) (azureCloud.AKSClient, error) {
		return nil, fmt.Errorf("credentials invalid")
	}

	integration := &AKSIntegration{
		name:     "test-aks",
		identity: "azure-dev",
		cluster:  &schema.Cluster{Name: "dev-cluster", ResourceGroup: "dev-rg"},
	}

	err := integration.Execute(context.Background(), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAKSIntegrationFailed)
	assert.Contains(t, err.Error(), "credentials invalid")
}

func TestAKSIntegration_Execute_DescribeClusterError(t *testing.T) {
	origClientFactory := aksClientFactory
	origDescribe := aksDescribeCluster
	t.Cleanup(func() {
		aksClientFactory = origClientFactory
		aksDescribeCluster = origDescribe
	})

	aksClientFactory = func(_ context.Context, _ types.ICredentials, _ string) (azureCloud.AKSClient, error) {
		return nil, nil
	}

	aksDescribeCluster = func(_ context.Context, _ azureCloud.AKSClient, _, _, _ string) (*azureCloud.AKSClusterInfo, error) {
		return nil, fmt.Errorf("cluster not found")
	}

	integration := &AKSIntegration{
		name:     "test-aks",
		identity: "azure-dev",
		cluster:  &schema.Cluster{Name: "dev-cluster", ResourceGroup: "dev-rg"},
	}

	err := integration.Execute(context.Background(), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAKSIntegrationFailed)
	assert.Contains(t, err.Error(), "cluster not found")
}

func TestAKSIntegration_Cleanup_NonexistentFile(t *testing.T) {
	integration := &AKSIntegration{
		name:     "test",
		identity: "azure-dev",
		cluster: &schema.Cluster{
			Name:          "dev-cluster",
			ResourceGroup: "dev-rg",
			Alias:         "dev-aks",
			Kubeconfig: &schema.KubeconfigSettings{
				Path: filepath.Join(t.TempDir(), "nonexistent", "kubeconfig"),
			},
		},
	}

	err := integration.Cleanup(t.Context())
	require.NoError(t, err)
}

func TestAKSIntegration_Cleanup_RemovesEntries(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	resourceID := "/subscriptions/sub-123/resourceGroups/dev-rg/providers/Microsoft.ContainerService/managedClusters/dev-cluster"

	info := &kube.ClusterInfo{
		Name:                     "dev-cluster",
		Endpoint:                 "https://example.hcp.eastus.azmk8s.io:443",
		CertificateAuthorityData: "dGVzdA==",
		ID:                       resourceID,
		Region:                   "dev-rg",
		UserPrefix:               "aks",
		ExecArgs:                 []string{"azure", "aks", "token"},
	}

	mgr, err := kube.NewKubeconfigManager(kubeconfigPath, "")
	require.NoError(t, err)
	_, err = mgr.WriteClusterConfig(info, "dev-aks", "merge")
	require.NoError(t, err)

	ids, err := mgr.ListClusterIDs()
	require.NoError(t, err)
	assert.Contains(t, ids, resourceID)

	integration := &AKSIntegration{
		name:     "test",
		identity: "azure-dev",
		cluster: &schema.Cluster{
			Name:          "dev-cluster",
			ResourceGroup: "dev-rg",
			Alias:         "dev-aks",
			Kubeconfig:    &schema.KubeconfigSettings{Path: kubeconfigPath},
		},
	}

	err = integration.Cleanup(t.Context())
	require.NoError(t, err)

	ids, err = mgr.ListClusterIDs()
	require.NoError(t, err)
	assert.NotContains(t, ids, resourceID)
}

func TestAKSIntegration_FindClusterID_NoMatch(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")

	mgr, err := kube.NewKubeconfigManager(kubeconfigPath, "")
	require.NoError(t, err)

	info := &kube.ClusterInfo{
		Name:       "other-cluster",
		Endpoint:   "https://example.hcp.eastus.azmk8s.io:443",
		ID:         "/subscriptions/sub-123/resourceGroups/dev-rg/providers/Microsoft.ContainerService/managedClusters/other-cluster",
		Region:     "dev-rg",
		UserPrefix: "aks",
		ExecArgs:   []string{"azure", "aks", "token"},
	}
	_, err = mgr.WriteClusterConfig(info, "other", "merge")
	require.NoError(t, err)

	integration := &AKSIntegration{
		cluster: &schema.Cluster{
			Name:          "missing-cluster",
			ResourceGroup: "dev-rg",
			Kubeconfig:    &schema.KubeconfigSettings{Path: kubeconfigPath},
		},
	}

	_, err = integration.findClusterID(mgr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no cluster resource ID found matching missing-cluster")
}

func TestAKSIntegration_FindClusterID_DisambiguatesResourceGroup(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	mgr, err := kube.NewKubeconfigManager(kubeconfigPath, "")
	require.NoError(t, err)

	const clusterName = "shared-cluster"
	wrongID := "/subscriptions/sub-123/resourceGroups/other-rg/providers/Microsoft.ContainerService/managedClusters/" + clusterName
	wantID := "/subscriptions/sub-123/resourceGroups/target-rg/providers/Microsoft.ContainerService/managedClusters/" + clusterName
	for _, tt := range []struct {
		alias  string
		id     string
		region string
	}{
		{alias: "other", id: wrongID, region: "other-rg"},
		{alias: "target", id: wantID, region: "target-rg"},
	} {
		_, err = mgr.WriteClusterConfig(&kube.ClusterInfo{
			Name:       clusterName,
			Endpoint:   "https://example.hcp.eastus.azmk8s.io:443",
			ID:         tt.id,
			Region:     tt.region,
			UserPrefix: "aks",
			ExecArgs:   []string{"azure", "aks", "token"},
		}, tt.alias, "merge")
		require.NoError(t, err)
	}

	integration := &AKSIntegration{cluster: &schema.Cluster{Name: clusterName, ResourceGroup: "TARGET-RG"}}
	got, err := integration.findClusterID(mgr)
	require.NoError(t, err)
	assert.Equal(t, wantID, got)
}

func TestAKSIntegrationRegistration(t *testing.T) {
	assert.True(t, integrations.IsRegistered(integrations.KindAzureAKS))
}

func TestAKSIntegrationRegistration_ViaRegistry(t *testing.T) {
	config := validAKSConfig("test-via-registry")

	integration, err := integrations.Create(config)
	require.NoError(t, err)
	require.NotNil(t, integration)
	assert.Equal(t, integrations.KindAzureAKS, integration.Kind())
}

func TestAKSIntegration_ResolveKubeconfigSettings_NilKubeconfig(t *testing.T) {
	integration := &AKSIntegration{
		cluster: &schema.Cluster{Name: "dev-cluster", ResourceGroup: "dev-rg"},
	}

	path, mode, update := integration.resolveKubeconfigSettings()
	assert.Equal(t, "", path)
	assert.Equal(t, "", mode)
	assert.Equal(t, "", update)
}

func TestAKSIntegration_ResolveKubeconfigSettings_WithSettings(t *testing.T) {
	integration := &AKSIntegration{
		cluster: &schema.Cluster{
			Name:          "dev-cluster",
			ResourceGroup: "dev-rg",
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
