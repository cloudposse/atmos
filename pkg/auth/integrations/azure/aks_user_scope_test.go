package azure

// Tests that AKS kubeconfig contexts written for different identities get
// their own exec user, and that Cleanup removes only its own entries,
// including the user name older versions wrote (issue #3720, the AKS
// counterpart of EKS finding 2.5).

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"

	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	"github.com/cloudposse/atmos/pkg/auth/cloud/kube"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	devClusterResourceID = "/subscriptions/sub-123/resourceGroups/dev-rg/providers/Microsoft.ContainerService/managedClusters/dev-cluster"
	legacyDevAKSUserName = "atmos-aks-dev-cluster-dev-rg-sub-123"
)

// stubDevAKSCluster makes Execute describe the dev cluster without Azure calls.
func stubDevAKSCluster(t *testing.T) {
	t.Helper()

	origFactory, origDescribe := aksClientFactory, aksDescribeCluster
	t.Cleanup(func() { aksClientFactory, aksDescribeCluster = origFactory, origDescribe })
	aksClientFactory = func(context.Context, types.ICredentials, string) (azureCloud.AKSClient, error) {
		return nil, nil
	}
	aksDescribeCluster = func(_ context.Context, _ azureCloud.AKSClient, subscriptionID, resourceGroup, name string) (*azureCloud.AKSClusterInfo, error) {
		return &azureCloud.AKSClusterInfo{
			Name: name, ResourceGroup: resourceGroup, SubscriptionID: subscriptionID, ID: devClusterResourceID,
			Endpoint: "https://dev-cluster.hcp.eastus.azmk8s.io:443", CertificateAuthorityData: "dGVzdA==",
			ServerID: azureCloud.AKSServerAppID,
		}, nil
	}
}

// newScopedAKSIntegration builds an AKS integration through the factory, as
// the auth manager does, with the given alias, identity and realm.
func newScopedAKSIntegration(t *testing.T, path, alias, identity, realm string) integrations.Integration {
	t.Helper()

	integration, err := NewAKSIntegration(&integrations.IntegrationConfig{
		Name:  "dev",
		Realm: realm,
		Config: &schema.Integration{
			Kind: integrations.KindAzureAKS,
			Via:  &schema.IntegrationVia{Identity: identity},
			Spec: &schema.IntegrationSpec{Cluster: &schema.Cluster{
				Name: "dev-cluster", ResourceGroup: "dev-rg", Alias: alias,
				Kubeconfig: &schema.KubeconfigSettings{Path: path},
			}},
		},
	})
	require.NoError(t, err)
	return integration
}

// identityArg returns the --identity argument of the exec user behind contextName.
func identityArg(t *testing.T, path, contextName string) string {
	t.Helper()

	config, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	kubeContext, ok := config.Contexts[contextName]
	require.True(t, ok, "context %s must exist", contextName)
	user, ok := config.AuthInfos[kubeContext.AuthInfo]
	require.True(t, ok, "user %s of context %s must exist", kubeContext.AuthInfo, contextName)
	require.NotNil(t, user.Exec)
	const prefix = "--identity="
	for _, arg := range user.Exec.Args {
		if len(arg) > len(prefix) && arg[:len(prefix)] == prefix {
			return arg[len(prefix):]
		}
	}
	return ""
}

// TestAKSIntegration_Execute_DistinctUserPerIdentity runs two integrations for
// the same cluster with different identities and verifies that each context
// keeps running as its own identity.
func TestAKSIntegration_Execute_DistinctUserPerIdentity(t *testing.T) {
	stubDevAKSCluster(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	creds := &types.AzureCredentials{SubscriptionID: "sub-123"}

	require.NoError(t, newScopedAKSIntegration(t, kubeconfigPath, "dev-admin", "azure-admin", "").Execute(t.Context(), creds))
	require.NoError(t, newScopedAKSIntegration(t, kubeconfigPath, "dev-reader", "azure-reader", "").Execute(t.Context(), creds))

	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	require.NoError(t, err)
	adminUser := config.Contexts["dev-admin"].AuthInfo
	readerUser := config.Contexts["dev-reader"].AuthInfo
	assert.NotEqual(t, adminUser, readerUser, "contexts for different identities must not share an exec user")
	assert.Equal(t, "atmos-aks-dev-cluster-dev-rg-sub-123-azure-admin", adminUser)
	assert.Equal(t, "atmos-aks-dev-cluster-dev-rg-sub-123-azure-reader", readerUser)
	assert.Equal(t, "azure-admin", identityArg(t, kubeconfigPath, "dev-admin"))
	assert.Equal(t, "azure-reader", identityArg(t, kubeconfigPath, "dev-reader"))
}

// TestAKSIntegration_Cleanup_KeepsOtherIdentityContext verifies that cleaning
// up one identity's integration leaves the other identity's context, its user
// and the shared cluster entry in place.
func TestAKSIntegration_Cleanup_KeepsOtherIdentityContext(t *testing.T) {
	stubDevAKSCluster(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	creds := &types.AzureCredentials{SubscriptionID: "sub-123"}

	require.NoError(t, newScopedAKSIntegration(t, kubeconfigPath, "dev-admin", "azure-admin", "").Execute(t.Context(), creds))
	reader := newScopedAKSIntegration(t, kubeconfigPath, "dev-reader", "azure-reader", "")
	require.NoError(t, reader.Execute(t.Context(), creds))

	require.NoError(t, reader.Cleanup(t.Context()))

	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	require.NoError(t, err)
	assert.NotContains(t, config.Contexts, "dev-reader")
	assert.NotContains(t, config.AuthInfos, "atmos-aks-dev-cluster-dev-rg-sub-123-azure-reader")
	assert.Contains(t, config.Clusters, devClusterResourceID, "the cluster is still used by the dev-admin context")
	assert.Equal(t, "azure-admin", identityArg(t, kubeconfigPath, "dev-admin"))
}

// writeLegacyAKSEntry writes an entry in the format older versions used: the
// exec user is named atmos-aks-<name>-<resource-group>-<subscription>, without
// the identity.
func writeLegacyAKSEntry(t *testing.T, path, alias, identity string) {
	t.Helper()

	mgr, err := kube.NewKubeconfigManager(path, "")
	require.NoError(t, err)
	_, err = mgr.WriteClusterConfig(&kube.ClusterInfo{
		Name: "dev-cluster", Endpoint: "https://dev-cluster.hcp.eastus.azmk8s.io:443", ID: devClusterResourceID,
		Region: "dev-rg", UserPrefix: "aks", AccountID: "sub-123",
		ExecArgs: []string{"azure", "aks", "token", "--cluster-name", "dev-cluster", "--resource-group", "dev-rg", "--identity=" + identity},
	}, alias, "merge")
	require.NoError(t, err)

	config, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	require.Contains(t, config.AuthInfos, legacyDevAKSUserName, "the fixture must use the legacy user name")
}

// TestAKSIntegration_Cleanup_RemovesLegacyUser verifies that Cleanup still
// removes an entry written by an older version with the legacy user name.
func TestAKSIntegration_Cleanup_RemovesLegacyUser(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	writeLegacyAKSEntry(t, kubeconfigPath, "dev-admin", "azure-admin")
	// An unrelated cluster keeps the file from being deleted, so the result can be inspected.
	mgr, err := kube.NewKubeconfigManager(kubeconfigPath, "")
	require.NoError(t, err)
	_, err = mgr.WriteClusterConfig(&kube.ClusterInfo{
		Name: "other", Endpoint: "https://other.example.invalid", ID: "other-id", Region: "other-rg", UserPrefix: "aks",
	}, "other", "merge")
	require.NoError(t, err)

	require.NoError(t, newScopedAKSIntegration(t, kubeconfigPath, "dev-admin", "azure-admin", "").Cleanup(t.Context()))

	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	require.NoError(t, err)
	assert.NotContains(t, config.Contexts, "dev-admin")
	assert.NotContains(t, config.Clusters, devClusterResourceID)
	assert.NotContains(t, config.AuthInfos, legacyDevAKSUserName)
	assert.Contains(t, config.Contexts, "other")
}

// TestAKSIntegration_Cleanup_KeepsLegacyUserStillInUse verifies that a legacy
// user shared with another context is not removed with this integration's context.
func TestAKSIntegration_Cleanup_KeepsLegacyUserStillInUse(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	writeLegacyAKSEntry(t, kubeconfigPath, "dev-admin", "azure-admin")
	writeLegacyAKSEntry(t, kubeconfigPath, "dev-reader", "azure-reader")

	require.NoError(t, newScopedAKSIntegration(t, kubeconfigPath, "dev-reader", "azure-reader", "").Cleanup(t.Context()))

	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	require.NoError(t, err)
	assert.NotContains(t, config.Contexts, "dev-reader")
	assert.Contains(t, config.Contexts, "dev-admin")
	assert.Contains(t, config.AuthInfos, legacyDevAKSUserName, "the legacy user is still used by dev-admin")
	assert.Contains(t, config.Clusters, devClusterResourceID)
}
