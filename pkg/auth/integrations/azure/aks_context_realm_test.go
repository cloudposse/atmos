package azure

// Tests that the default AKS kube context name is scoped to the auth realm,
// that Cleanup removes only contexts that point at this integration's own
// users, and that an unprefixed context written by an older version is
// migrated (issue #3720, "Default kube context names are shared across realms").

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// The devUserRepoA and devUserRepoB constants are the realm-scoped users of the dev integration.
	devUserRepoA = "atmos-repo-a-aks-dev-cluster-dev-rg-sub-123-azure-dev"
	devUserRepoB = "atmos-repo-b-aks-dev-cluster-dev-rg-sub-123-azure-dev"
	// The devUserPreRealm constant is the user written before realms were part of the name.
	devUserPreRealm = "atmos-aks-dev-cluster-dev-rg-sub-123-azure-dev"
	// The foreignAKSUser constant is a user Atmos did not write, such as one from the Azure CLI.
	foreignAKSUser = "clusterUser_dev-rg_dev-cluster"
)

// devCreds returns credentials for the dev cluster's subscription.
func devCreds() *types.AzureCredentials {
	return &types.AzureCredentials{SubscriptionID: "sub-123"}
}

// putAKSContext adds a context, its cluster and a token user to the kubeconfig
// at path, the way another tool or an older Atmos version would have written them.
func putAKSContext(t *testing.T, path, userName string) {
	t.Helper()

	config := clientcmdapi.NewConfig()
	if _, err := os.Stat(path); err == nil {
		config, err = clientcmd.LoadFromFile(path)
		require.NoError(t, err)
	}
	config.Clusters[devClusterResourceID] = &clientcmdapi.Cluster{Server: "https://dev-cluster.hcp.eastus.azmk8s.io:443"}
	config.AuthInfos[userName] = &clientcmdapi.AuthInfo{Token: "token"}
	config.Contexts[devClusterResourceID] = &clientcmdapi.Context{Cluster: devClusterResourceID, AuthInfo: userName}
	config.CurrentContext = devClusterResourceID
	require.NoError(t, clientcmd.WriteToFile(*config, path))
}

// TestAKSIntegration_Execute_DistinctDefaultContextPerRealm verifies that two
// realms writing the same cluster without an alias get distinct contexts.
func TestAKSIntegration_Execute_DistinctDefaultContextPerRealm(t *testing.T) {
	stubDevAKSCluster(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")

	require.NoError(t, newScopedAKSIntegration(t, kubeconfigPath, "", "azure-dev", realmRepoA).Execute(t.Context(), devCreds()))
	require.NoError(t, newScopedAKSIntegration(t, kubeconfigPath, "", "azure-dev", realmRepoB).Execute(t.Context(), devCreds()))

	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	require.NoError(t, err)
	contextA := realmRepoA + "/" + devClusterResourceID
	contextB := realmRepoB + "/" + devClusterResourceID
	require.Contains(t, config.Contexts, contextA)
	require.Contains(t, config.Contexts, contextB)
	assert.Equal(t, devUserRepoA, config.Contexts[contextA].AuthInfo)
	assert.Equal(t, devUserRepoB, config.Contexts[contextB].AuthInfo)
	assert.NotContains(t, config.Contexts, devClusterResourceID)
	assert.Equal(t, contextB, config.CurrentContext)
}

// TestAKSIntegration_Cleanup_KeepsOtherRealmDefaultContext verifies that
// cleanup in one realm leaves the other realm's default context working.
func TestAKSIntegration_Cleanup_KeepsOtherRealmDefaultContext(t *testing.T) {
	stubDevAKSCluster(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")

	integrationA := newScopedAKSIntegration(t, kubeconfigPath, "", "azure-dev", realmRepoA)
	require.NoError(t, integrationA.Execute(t.Context(), devCreds()))
	require.NoError(t, newScopedAKSIntegration(t, kubeconfigPath, "", "azure-dev", realmRepoB).Execute(t.Context(), devCreds()))

	require.NoError(t, integrationA.Cleanup(t.Context()))

	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	require.NoError(t, err)
	assert.NotContains(t, config.Contexts, realmRepoA+"/"+devClusterResourceID)
	assert.NotContains(t, config.AuthInfos, devUserRepoA)
	require.Contains(t, config.Contexts, realmRepoB+"/"+devClusterResourceID)
	assert.Equal(t, devUserRepoB, config.Contexts[realmRepoB+"/"+devClusterResourceID].AuthInfo)
	assert.Contains(t, config.Clusters, devClusterResourceID)
}

// TestAKSIntegration_Cleanup_KeepsDefaultContextOfOtherUser verifies that a
// context with the default name that points at another user is left alone.
func TestAKSIntegration_Cleanup_KeepsDefaultContextOfOtherUser(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	putAKSContext(t, kubeconfigPath, foreignAKSUser)

	require.NoError(t, newScopedAKSIntegration(t, kubeconfigPath, "", "azure-dev", "").Cleanup(t.Context()))

	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	require.NoError(t, err)
	require.Contains(t, config.Contexts, devClusterResourceID)
	assert.Equal(t, foreignAKSUser, config.Contexts[devClusterResourceID].AuthInfo)
	assert.Contains(t, config.AuthInfos, foreignAKSUser)
	assert.Contains(t, config.Clusters, devClusterResourceID)
}

// TestAKSIntegration_Execute_MigratesUnprefixedOwnContext verifies that writing
// the realm-prefixed context removes the unprefixed context an older version
// wrote for this integration, and the old user once nothing references it.
func TestAKSIntegration_Execute_MigratesUnprefixedOwnContext(t *testing.T) {
	for _, oldUser := range []string{devUserPreRealm, legacyDevAKSUserName} {
		t.Run(oldUser, func(t *testing.T) {
			stubDevAKSCluster(t)
			kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
			putAKSContext(t, kubeconfigPath, oldUser)

			require.NoError(t, newScopedAKSIntegration(t, kubeconfigPath, "", "azure-dev", realmRepoA).Execute(t.Context(), devCreds()))

			config, err := clientcmd.LoadFromFile(kubeconfigPath)
			require.NoError(t, err)
			newContext := realmRepoA + "/" + devClusterResourceID
			assert.NotContains(t, config.Contexts, devClusterResourceID)
			require.Contains(t, config.Contexts, newContext)
			assert.Equal(t, devUserRepoA, config.Contexts[newContext].AuthInfo)
			assert.Equal(t, newContext, config.CurrentContext)
			assert.NotContains(t, config.AuthInfos, oldUser)
		})
	}
}

// TestAKSIntegration_Execute_KeepsUnprefixedForeignContext verifies that an
// unprefixed context that points at another user is never touched.
func TestAKSIntegration_Execute_KeepsUnprefixedForeignContext(t *testing.T) {
	stubDevAKSCluster(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	putAKSContext(t, kubeconfigPath, devUserRepoB)

	require.NoError(t, newScopedAKSIntegration(t, kubeconfigPath, "", "azure-dev", realmRepoA).Execute(t.Context(), devCreds()))

	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	require.NoError(t, err)
	require.Contains(t, config.Contexts, devClusterResourceID)
	assert.Equal(t, devUserRepoB, config.Contexts[devClusterResourceID].AuthInfo)
	assert.Contains(t, config.AuthInfos, devUserRepoB)
	require.Contains(t, config.Contexts, realmRepoA+"/"+devClusterResourceID)
	assert.Equal(t, devUserRepoA, config.Contexts[realmRepoA+"/"+devClusterResourceID].AuthInfo)
}

// TestAKSIntegration_Cleanup_RemovesUnprefixedOwnContext verifies that a realm's
// cleanup also removes the unprefixed context an older version wrote for it.
func TestAKSIntegration_Cleanup_RemovesUnprefixedOwnContext(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	putAKSContext(t, kubeconfigPath, devUserPreRealm)

	require.NoError(t, newScopedAKSIntegration(t, kubeconfigPath, "", "azure-dev", realmRepoA).Cleanup(t.Context()))

	_, err := os.Stat(kubeconfigPath)
	assert.True(t, os.IsNotExist(err), "the only entry was this integration's, so the file is removed")
}

// TestAKSIntegration_Execute_ErrorModeRealmContext verifies that update: error
// stays idempotent with the realm-prefixed context name and still fails when
// that context points at another user.
func TestAKSIntegration_Execute_ErrorModeRealmContext(t *testing.T) {
	stubDevAKSCluster(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	integration, ok := newScopedAKSIntegration(t, kubeconfigPath, "", "azure-dev", realmRepoA).(*AKSIntegration)
	require.True(t, ok)
	integration.cluster.Kubeconfig = &schema.KubeconfigSettings{Path: kubeconfigPath, Update: "error"}

	require.NoError(t, integration.Execute(t.Context(), devCreds()))
	before, err := os.ReadFile(kubeconfigPath)
	require.NoError(t, err)
	require.NoError(t, integration.Execute(t.Context(), devCreds()), "an identical re-run is not a conflict")
	after, err := os.ReadFile(kubeconfigPath)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))

	// Repoint the realm context at another user; cluster and user stay identical.
	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	require.NoError(t, err)
	contextName := realmRepoA + "/" + devClusterResourceID
	require.Contains(t, config.Contexts, contextName)
	config.Contexts[contextName].AuthInfo = foreignAKSUser
	require.NoError(t, clientcmd.WriteToFile(*config, kubeconfigPath))

	err = integration.Execute(t.Context(), devCreds())
	require.ErrorIs(t, err, errUtils.ErrAKSIntegrationFailed)
	require.ErrorIs(t, err, errUtils.ErrKubeconfigMerge)
}
