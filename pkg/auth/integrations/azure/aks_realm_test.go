package azure

// Tests that AKS kubeconfig exec users are scoped to the auth realm, so two
// repositories (or worktrees) with different realms never share, and
// overwrite, one kube user in the shared kubeconfig (issue #3720, #3675).

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/cloudposse/atmos/pkg/auth/types"
)

const (
	realmRepoA = "repo-a"
	realmRepoB = "repo-b"
)

// execEnvValue returns the value of the named exec-plugin env var of user, or "".
func execEnvValue(t *testing.T, user *clientcmdapi.AuthInfo, name string) string {
	t.Helper()

	require.NotNil(t, user)
	require.NotNil(t, user.Exec)
	for _, env := range user.Exec.Env {
		if env.Name == name {
			return env.Value
		}
	}
	return ""
}

// resolvedDir returns dir with symlinks resolved, so macOS /var and /private/var compare equal.
func resolvedDir(t *testing.T, dir string) string {
	t.Helper()

	resolved, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	return resolved
}

// TestAKSIntegration_Execute_DistinctUserPerRealm runs the same integration
// from two checkouts with different realms and verifies that each context
// keeps its own exec user, working directory and realm.
func TestAKSIntegration_Execute_DistinctUserPerRealm(t *testing.T) {
	stubDevAKSCluster(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	repoA, repoB := t.TempDir(), t.TempDir()
	creds := &types.AzureCredentials{SubscriptionID: "sub-123"}

	t.Chdir(repoA)
	require.NoError(t, newScopedAKSIntegration(t, kubeconfigPath, "dev-a", "azure-dev", realmRepoA).Execute(t.Context(), creds))
	t.Chdir(repoB)
	require.NoError(t, newScopedAKSIntegration(t, kubeconfigPath, "dev-b", "azure-dev", realmRepoB).Execute(t.Context(), creds))

	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	require.NoError(t, err)
	userA := config.Contexts["dev-a"].AuthInfo
	userB := config.Contexts["dev-b"].AuthInfo
	require.NotEqual(t, userA, userB, "realms must not share a kube user")
	assert.Equal(t, "atmos-repo-a-aks-dev-cluster-dev-rg-sub-123-azure-dev", userA)
	assert.Equal(t, "atmos-repo-b-aks-dev-cluster-dev-rg-sub-123-azure-dev", userB)

	assert.Equal(t, resolvedDir(t, repoA), resolvedDir(t, execEnvValue(t, config.AuthInfos[userA], "ATMOS_CHDIR")))
	assert.Equal(t, resolvedDir(t, repoB), resolvedDir(t, execEnvValue(t, config.AuthInfos[userB], "ATMOS_CHDIR")))
	assert.Equal(t, realmRepoA, execEnvValue(t, config.AuthInfos[userA], "ATMOS_AUTH_REALM"))
	assert.Equal(t, realmRepoB, execEnvValue(t, config.AuthInfos[userB], "ATMOS_AUTH_REALM"))
}

// TestAKSIntegration_Cleanup_KeepsOtherRealmUser verifies that cleaning up one
// realm's integration leaves the other realm's context and user in place.
func TestAKSIntegration_Cleanup_KeepsOtherRealmUser(t *testing.T) {
	stubDevAKSCluster(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	creds := &types.AzureCredentials{SubscriptionID: "sub-123"}

	integrationA := newScopedAKSIntegration(t, kubeconfigPath, "dev-a", "azure-dev", realmRepoA)
	require.NoError(t, integrationA.Execute(t.Context(), creds))
	require.NoError(t, newScopedAKSIntegration(t, kubeconfigPath, "dev-b", "azure-dev", realmRepoB).Execute(t.Context(), creds))

	require.NoError(t, integrationA.Cleanup(t.Context()))

	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	require.NoError(t, err)
	assert.NotContains(t, config.Contexts, "dev-a")
	assert.NotContains(t, config.AuthInfos, "atmos-repo-a-aks-dev-cluster-dev-rg-sub-123-azure-dev")
	assert.Contains(t, config.Contexts, "dev-b")
	assert.Contains(t, config.AuthInfos, "atmos-repo-b-aks-dev-cluster-dev-rg-sub-123-azure-dev")
	assert.Contains(t, config.Clusters, devClusterResourceID, "the cluster is still used by the dev-b context")
}
