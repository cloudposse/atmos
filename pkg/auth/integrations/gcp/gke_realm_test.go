package gcp

// Tests that GKE kubeconfig exec users are scoped to the auth realm, so two
// repositories (or worktrees) with different realms never share, and
// overwrite, one kube user in the shared kubeconfig (issue #3720, #3675).

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
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

// TestGKEIntegration_Execute_DistinctUserPerRealm runs the same integration
// from two checkouts with different realms and verifies that each context
// keeps its own exec user, working directory and realm.
func TestGKEIntegration_Execute_DistinctUserPerRealm(t *testing.T) {
	stubExampleGKECluster(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	repoA, repoB := t.TempDir(), t.TempDir()

	t.Chdir(repoA)
	require.NoError(t, newScopedGKEIntegration(t, kubeconfigPath, "example-a", "gcp-admin", realmRepoA).Execute(t.Context(), exampleGCPCredentials()))
	t.Chdir(repoB)
	require.NoError(t, newScopedGKEIntegration(t, kubeconfigPath, "example-b", "gcp-admin", realmRepoB).Execute(t.Context(), exampleGCPCredentials()))

	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	require.NoError(t, err)
	userA := config.Contexts["example-a"].AuthInfo
	userB := config.Contexts["example-b"].AuthInfo
	require.NotEqual(t, userA, userB, "realms must not share a kube user")
	assert.Equal(t, "atmos-repo-a-gke-example-cluster-example-project-us-central1-gcp-admin", userA)
	assert.Equal(t, "atmos-repo-b-gke-example-cluster-example-project-us-central1-gcp-admin", userB)

	assert.Equal(t, resolvedDir(t, repoA), resolvedDir(t, execEnvValue(t, config.AuthInfos[userA], "ATMOS_CHDIR")))
	assert.Equal(t, resolvedDir(t, repoB), resolvedDir(t, execEnvValue(t, config.AuthInfos[userB], "ATMOS_CHDIR")))
	assert.Equal(t, realmRepoA, execEnvValue(t, config.AuthInfos[userA], "ATMOS_AUTH_REALM"))
	assert.Equal(t, realmRepoB, execEnvValue(t, config.AuthInfos[userB], "ATMOS_AUTH_REALM"))
}

// TestGKEIntegration_Cleanup_KeepsOtherRealmUser verifies that cleaning up one
// realm's integration leaves the other realm's context and user in place.
func TestGKEIntegration_Cleanup_KeepsOtherRealmUser(t *testing.T) {
	stubExampleGKECluster(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")

	integrationA := newScopedGKEIntegration(t, kubeconfigPath, "example-a", "gcp-admin", realmRepoA)
	require.NoError(t, integrationA.Execute(t.Context(), exampleGCPCredentials()))
	require.NoError(t, newScopedGKEIntegration(t, kubeconfigPath, "example-b", "gcp-admin", realmRepoB).Execute(t.Context(), exampleGCPCredentials()))

	require.NoError(t, integrationA.Cleanup(t.Context()))

	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	require.NoError(t, err)
	assert.NotContains(t, config.Contexts, "example-a")
	assert.NotContains(t, config.AuthInfos, "atmos-repo-a-gke-example-cluster-example-project-us-central1-gcp-admin")
	assert.Contains(t, config.Contexts, "example-b")
	assert.Contains(t, config.AuthInfos, "atmos-repo-b-gke-example-cluster-example-project-us-central1-gcp-admin")
	assert.Contains(t, config.Clusters, exampleClusterID, "the cluster is still used by the example-b context")
}

// TestGKEIntegration_Cleanup_RemovesPreRealmUser verifies that a realm's
// cleanup still removes the user written before realms were part of the name.
func TestGKEIntegration_Cleanup_RemovesPreRealmUser(t *testing.T) {
	stubExampleGKECluster(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	writeLegacyGKEEntry(t, kubeconfigPath, "other", "gcp-other")
	require.NoError(t, newScopedGKEIntegration(t, kubeconfigPath, "example-a", "gcp-admin", "").Execute(t.Context(), exampleGCPCredentials()))

	require.NoError(t, newScopedGKEIntegration(t, kubeconfigPath, "example-a", "gcp-admin", realmRepoA).Cleanup(t.Context()))

	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	require.NoError(t, err)
	assert.NotContains(t, config.Contexts, "example-a")
	assert.NotContains(t, config.AuthInfos, legacyExampleUserName+"-gcp-admin")
	assert.Contains(t, config.Contexts, "other")
}
