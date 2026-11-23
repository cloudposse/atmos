package gcp

// Tests that the default GKE kube context name is scoped to the auth realm,
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
)

const (
	// The exampleUserRepoA and exampleUserRepoB constants are the realm-scoped users of the example integration.
	exampleUserRepoA = "atmos-repo-a-gke-example-cluster-example-project-us-central1-gcp-admin"
	exampleUserRepoB = "atmos-repo-b-gke-example-cluster-example-project-us-central1-gcp-admin"
	// The exampleUserPreRealm constant is the user written before realms were part of the name.
	exampleUserPreRealm = legacyExampleUserName + "-gcp-admin"
	// The foreignGKEUser constant is a user Atmos did not write, such as one from gcloud.
	foreignGKEUser = "gke_example-project_us-central1_example-cluster"
)

// putGKEContext adds a context, its cluster and a token user to the kubeconfig
// at path, the way another tool or an older Atmos version would have written them.
func putGKEContext(t *testing.T, path, userName string) {
	t.Helper()

	config := clientcmdapi.NewConfig()
	if _, err := os.Stat(path); err == nil {
		config, err = clientcmd.LoadFromFile(path)
		require.NoError(t, err)
	}
	config.Clusters[exampleClusterID] = &clientcmdapi.Cluster{Server: "https://gke.example.invalid"}
	config.AuthInfos[userName] = &clientcmdapi.AuthInfo{Token: "token"}
	config.Contexts[exampleClusterID] = &clientcmdapi.Context{Cluster: exampleClusterID, AuthInfo: userName}
	config.CurrentContext = exampleClusterID
	require.NoError(t, clientcmd.WriteToFile(*config, path))
}

// TestGKEIntegration_Execute_DistinctDefaultContextPerRealm verifies that two
// realms writing the same cluster without an alias get distinct contexts.
func TestGKEIntegration_Execute_DistinctDefaultContextPerRealm(t *testing.T) {
	stubExampleGKECluster(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")

	require.NoError(t, newScopedGKEIntegration(t, kubeconfigPath, "", "gcp-admin", realmRepoA).Execute(t.Context(), exampleGCPCredentials()))
	require.NoError(t, newScopedGKEIntegration(t, kubeconfigPath, "", "gcp-admin", realmRepoB).Execute(t.Context(), exampleGCPCredentials()))

	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	require.NoError(t, err)
	contextA := realmRepoA + "/" + exampleClusterID
	contextB := realmRepoB + "/" + exampleClusterID
	require.Contains(t, config.Contexts, contextA)
	require.Contains(t, config.Contexts, contextB)
	assert.Equal(t, exampleUserRepoA, config.Contexts[contextA].AuthInfo)
	assert.Equal(t, exampleUserRepoB, config.Contexts[contextB].AuthInfo)
	assert.NotContains(t, config.Contexts, exampleClusterID)
	assert.Equal(t, contextB, config.CurrentContext)
}

// TestGKEIntegration_Cleanup_KeepsOtherRealmDefaultContext verifies that
// cleanup in one realm leaves the other realm's default context working.
func TestGKEIntegration_Cleanup_KeepsOtherRealmDefaultContext(t *testing.T) {
	stubExampleGKECluster(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")

	integrationA := newScopedGKEIntegration(t, kubeconfigPath, "", "gcp-admin", realmRepoA)
	require.NoError(t, integrationA.Execute(t.Context(), exampleGCPCredentials()))
	require.NoError(t, newScopedGKEIntegration(t, kubeconfigPath, "", "gcp-admin", realmRepoB).Execute(t.Context(), exampleGCPCredentials()))

	require.NoError(t, integrationA.Cleanup(t.Context()))

	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	require.NoError(t, err)
	assert.NotContains(t, config.Contexts, realmRepoA+"/"+exampleClusterID)
	assert.NotContains(t, config.AuthInfos, exampleUserRepoA)
	require.Contains(t, config.Contexts, realmRepoB+"/"+exampleClusterID)
	assert.Equal(t, exampleUserRepoB, config.Contexts[realmRepoB+"/"+exampleClusterID].AuthInfo)
	assert.Contains(t, config.Clusters, exampleClusterID)
}

// TestGKEIntegration_Cleanup_KeepsDefaultContextOfOtherUser verifies that a
// context with the default name that points at another user is left alone.
func TestGKEIntegration_Cleanup_KeepsDefaultContextOfOtherUser(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	putGKEContext(t, kubeconfigPath, foreignGKEUser)

	require.NoError(t, newScopedGKEIntegration(t, kubeconfigPath, "", "gcp-admin", "").Cleanup(t.Context()))

	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	require.NoError(t, err)
	require.Contains(t, config.Contexts, exampleClusterID)
	assert.Equal(t, foreignGKEUser, config.Contexts[exampleClusterID].AuthInfo)
	assert.Contains(t, config.AuthInfos, foreignGKEUser)
	assert.Contains(t, config.Clusters, exampleClusterID)
}

// TestGKEIntegration_Execute_MigratesUnprefixedOwnContext verifies that writing
// the realm-prefixed context removes the unprefixed context an older version
// wrote for this integration, and the old user once nothing references it.
func TestGKEIntegration_Execute_MigratesUnprefixedOwnContext(t *testing.T) {
	for _, oldUser := range []string{exampleUserPreRealm, legacyExampleUserName} {
		t.Run(oldUser, func(t *testing.T) {
			stubExampleGKECluster(t)
			kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
			putGKEContext(t, kubeconfigPath, oldUser)

			require.NoError(t, newScopedGKEIntegration(t, kubeconfigPath, "", "gcp-admin", realmRepoA).Execute(t.Context(), exampleGCPCredentials()))

			config, err := clientcmd.LoadFromFile(kubeconfigPath)
			require.NoError(t, err)
			newContext := realmRepoA + "/" + exampleClusterID
			assert.NotContains(t, config.Contexts, exampleClusterID)
			require.Contains(t, config.Contexts, newContext)
			assert.Equal(t, exampleUserRepoA, config.Contexts[newContext].AuthInfo)
			assert.Equal(t, newContext, config.CurrentContext)
			assert.NotContains(t, config.AuthInfos, oldUser)
		})
	}
}

// TestGKEIntegration_Execute_KeepsUnprefixedForeignContext verifies that an
// unprefixed context that points at another user is never touched.
func TestGKEIntegration_Execute_KeepsUnprefixedForeignContext(t *testing.T) {
	stubExampleGKECluster(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	putGKEContext(t, kubeconfigPath, exampleUserRepoB)

	require.NoError(t, newScopedGKEIntegration(t, kubeconfigPath, "", "gcp-admin", realmRepoA).Execute(t.Context(), exampleGCPCredentials()))

	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	require.NoError(t, err)
	require.Contains(t, config.Contexts, exampleClusterID)
	assert.Equal(t, exampleUserRepoB, config.Contexts[exampleClusterID].AuthInfo)
	assert.Contains(t, config.AuthInfos, exampleUserRepoB)
	require.Contains(t, config.Contexts, realmRepoA+"/"+exampleClusterID)
	assert.Equal(t, exampleUserRepoA, config.Contexts[realmRepoA+"/"+exampleClusterID].AuthInfo)
}

// TestGKEIntegration_Cleanup_RemovesUnprefixedOwnContext verifies that a realm's
// cleanup also removes the unprefixed context an older version wrote for it.
func TestGKEIntegration_Cleanup_RemovesUnprefixedOwnContext(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	putGKEContext(t, kubeconfigPath, exampleUserPreRealm)

	require.NoError(t, newScopedGKEIntegration(t, kubeconfigPath, "", "gcp-admin", realmRepoA).Cleanup(t.Context()))

	_, err := os.Stat(kubeconfigPath)
	assert.True(t, os.IsNotExist(err), "the only entry was this integration's, so the file is removed")
}
