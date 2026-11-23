package aws

// Tests that the default EKS kube context name is scoped to the auth realm,
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
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/cloud/kube"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// prodUserRepoA and prodUserRepoB are the realm-scoped users of the prod admin integration.
	prodUserRepoA = "atmos-repo-a-eks-prod-us-east-2-111111111111-admin"
	prodUserRepoB = "atmos-repo-b-eks-prod-us-east-2-111111111111-admin"
	// prodUserPreRealm and prodUserLegacy are the users older versions wrote for it.
	prodUserPreRealm = "atmos-eks-prod-us-east-2-111111111111-admin"
	prodUserLegacy   = "atmos-eks-prod-us-east-2"
	// foreignUser is a user Atmos did not write, such as one from the AWS CLI.
	foreignUser = "arn:aws:eks:us-east-2:111111111111:cluster/prod-aws-cli"
)

// putKubeContext adds a context, its cluster and a token user to the kubeconfig
// at path, the way another tool or an older Atmos version would have written them.
func putKubeContext(t *testing.T, path, contextName, clusterID, userName string) {
	t.Helper()

	config := clientcmdapi.NewConfig()
	if _, err := os.Stat(path); err == nil {
		config, err = clientcmd.LoadFromFile(path)
		require.NoError(t, err)
	}
	if _, ok := config.Clusters[clusterID]; !ok {
		config.Clusters[clusterID] = &clientcmdapi.Cluster{Server: "https://example.eks.amazonaws.com"}
	}
	config.AuthInfos[userName] = &clientcmdapi.AuthInfo{Token: "token-" + userName}
	config.Contexts[contextName] = &clientcmdapi.Context{Cluster: clusterID, AuthInfo: userName}
	config.CurrentContext = contextName
	require.NoError(t, clientcmd.WriteToFile(*config, path))
}

// newErrorModeRealmEKSIntegration builds an EKS integration with update: error.
func newErrorModeRealmEKSIntegration(t *testing.T, path, realm string) *EKSIntegration {
	t.Helper()

	integration, ok := newRealmEKSIntegration(t, path, "", realm).(*EKSIntegration)
	require.True(t, ok)
	integration.cluster.Kubeconfig = &schema.KubeconfigSettings{Path: path, Update: "error"}
	return integration
}

// TestEKSIntegration_Execute_DistinctDefaultContextPerRealm verifies that two
// realms writing the same cluster without an alias get distinct contexts.
func TestEKSIntegration_Execute_DistinctDefaultContextPerRealm(t *testing.T) {
	stubProdEKSCluster(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")

	require.NoError(t, newRealmEKSIntegration(t, kubeconfigPath, "", realmRepoA).Execute(t.Context(), nil))
	require.NoError(t, newRealmEKSIntegration(t, kubeconfigPath, "", realmRepoB).Execute(t.Context(), nil))

	config, err := loadKubeconfig(kubeconfigPath)
	require.NoError(t, err)
	contextA := realmRepoA + "/" + prodClusterARN
	contextB := realmRepoB + "/" + prodClusterARN
	require.Contains(t, config.Contexts, contextA)
	require.Contains(t, config.Contexts, contextB)
	assert.Equal(t, prodUserRepoA, config.Contexts[contextA].AuthInfo)
	assert.Equal(t, prodUserRepoB, config.Contexts[contextB].AuthInfo)
	assert.Equal(t, prodClusterARN, config.Contexts[contextA].Cluster)
	assert.Equal(t, prodClusterARN, config.Contexts[contextB].Cluster)
	assert.NotContains(t, config.Contexts, prodClusterARN, "a realm must not write the unprefixed context")
	assert.Equal(t, contextB, config.CurrentContext, "the last writer's context is current")
}

// TestEKSIntegration_Cleanup_KeepsOtherRealmDefaultContext verifies that
// cleanup in one realm leaves the other realm's default context working.
func TestEKSIntegration_Cleanup_KeepsOtherRealmDefaultContext(t *testing.T) {
	stubProdEKSCluster(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")

	integrationA := newRealmEKSIntegration(t, kubeconfigPath, "", realmRepoA)
	require.NoError(t, integrationA.Execute(t.Context(), nil))
	require.NoError(t, newRealmEKSIntegration(t, kubeconfigPath, "", realmRepoB).Execute(t.Context(), nil))

	require.NoError(t, integrationA.Cleanup(t.Context()))

	config, err := loadKubeconfig(kubeconfigPath)
	require.NoError(t, err)
	assert.NotContains(t, config.AuthInfos, prodUserRepoA)
	assert.Contains(t, config.AuthInfos, prodUserRepoB)
	assert.Contains(t, config.Clusters, prodClusterARN)
	var kept []string
	for name, kubeContext := range config.Contexts {
		if kubeContext.AuthInfo == prodUserRepoB {
			kept = append(kept, name)
		}
	}
	assert.Equal(t, []string{realmRepoB + "/" + prodClusterARN}, kept, "realm B's context must survive realm A's cleanup")
}

// TestEKSIntegration_Cleanup_KeepsDefaultContextOfOtherUser verifies that a
// context with this integration's default name that points at a user this
// integration did not write is left alone, along with that user and cluster.
func TestEKSIntegration_Cleanup_KeepsDefaultContextOfOtherUser(t *testing.T) {
	for _, realm := range []string{"", realmRepoA} {
		t.Run("realm="+realm, func(t *testing.T) {
			kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
			contextName := prodClusterARN
			if realm != "" {
				contextName = realm + "/" + prodClusterARN
			}
			putKubeContext(t, kubeconfigPath, contextName, prodClusterARN, foreignUser)

			require.NoError(t, newRealmEKSIntegration(t, kubeconfigPath, "", realm).Cleanup(t.Context()))

			config, err := loadKubeconfig(kubeconfigPath)
			require.NoError(t, err)
			require.Contains(t, config.Contexts, contextName)
			assert.Equal(t, foreignUser, config.Contexts[contextName].AuthInfo)
			assert.Contains(t, config.AuthInfos, foreignUser)
			assert.Contains(t, config.Clusters, prodClusterARN)
			assert.Equal(t, contextName, config.CurrentContext)
		})
	}
}

// TestEKSIntegration_Execute_NoRealmOrAliasKeepsContextName verifies that the
// context name is unchanged without a realm, and with an explicit alias.
func TestEKSIntegration_Execute_NoRealmOrAliasKeepsContextName(t *testing.T) {
	for _, tt := range []struct {
		name, alias, realm, wantContext, wantUser string
	}{
		{name: "no realm", wantContext: prodClusterARN, wantUser: prodUserPreRealm},
		{name: "alias without realm", alias: "prod", wantContext: "prod", wantUser: prodUserPreRealm},
		{name: "alias with realm", alias: "prod", realm: realmRepoA, wantContext: "prod", wantUser: prodUserRepoA},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stubProdEKSCluster(t)
			kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")

			require.NoError(t, newRealmEKSIntegration(t, kubeconfigPath, tt.alias, tt.realm).Execute(t.Context(), nil))

			config, err := loadKubeconfig(kubeconfigPath)
			require.NoError(t, err)
			require.Len(t, config.Contexts, 1)
			require.Contains(t, config.Contexts, tt.wantContext)
			assert.Equal(t, tt.wantUser, config.Contexts[tt.wantContext].AuthInfo)
			assert.Equal(t, tt.wantContext, config.CurrentContext)
		})
	}
}

// TestEKSIntegration_Execute_MigratesUnprefixedOwnContext verifies that writing
// the realm-prefixed context removes the unprefixed context an older version
// wrote for this integration, whichever of its user names that context used,
// along with the old user once nothing references it.
func TestEKSIntegration_Execute_MigratesUnprefixedOwnContext(t *testing.T) {
	for _, oldUser := range []string{prodUserRepoA, prodUserPreRealm, prodUserLegacy} {
		t.Run(oldUser, func(t *testing.T) {
			stubProdEKSCluster(t)
			kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
			putKubeContext(t, kubeconfigPath, prodClusterARN, prodClusterARN, oldUser)

			require.NoError(t, newRealmEKSIntegration(t, kubeconfigPath, "", realmRepoA).Execute(t.Context(), nil))

			config, err := loadKubeconfig(kubeconfigPath)
			require.NoError(t, err)
			newContext := realmRepoA + "/" + prodClusterARN
			assert.NotContains(t, config.Contexts, prodClusterARN, "the old unprefixed context must be migrated")
			require.Contains(t, config.Contexts, newContext)
			assert.Equal(t, prodUserRepoA, config.Contexts[newContext].AuthInfo)
			assert.Equal(t, newContext, config.CurrentContext)
			assert.Len(t, config.Contexts, 1)
			assert.Equal(t, []string{prodUserRepoA}, mapKeys(config.AuthInfos), "the old user is removed once unreferenced")
		})
	}
}

// TestEKSIntegration_Execute_KeepsUnprefixedForeignContext verifies that an
// unprefixed context that points at another user is never touched.
func TestEKSIntegration_Execute_KeepsUnprefixedForeignContext(t *testing.T) {
	stubProdEKSCluster(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	putKubeContext(t, kubeconfigPath, prodClusterARN, prodClusterARN, prodUserRepoB)

	require.NoError(t, newRealmEKSIntegration(t, kubeconfigPath, "", realmRepoA).Execute(t.Context(), nil))

	config, err := loadKubeconfig(kubeconfigPath)
	require.NoError(t, err)
	require.Contains(t, config.Contexts, prodClusterARN)
	assert.Equal(t, prodUserRepoB, config.Contexts[prodClusterARN].AuthInfo)
	assert.Contains(t, config.AuthInfos, prodUserRepoB)
	assert.Equal(t, prodUserRepoA, config.Contexts[realmRepoA+"/"+prodClusterARN].AuthInfo)
}

// TestEKSIntegration_Cleanup_RemovesUnprefixedOwnContext verifies that a realm's
// cleanup also removes the unprefixed context an older version wrote for it.
func TestEKSIntegration_Cleanup_RemovesUnprefixedOwnContext(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	putKubeContext(t, kubeconfigPath, prodClusterARN, prodClusterARN, prodUserPreRealm)
	putKubeContext(t, kubeconfigPath, "other", "other-cluster", "other-user")

	require.NoError(t, newRealmEKSIntegration(t, kubeconfigPath, "", realmRepoA).Cleanup(t.Context()))

	config, err := loadKubeconfig(kubeconfigPath)
	require.NoError(t, err)
	assert.NotContains(t, config.Contexts, prodClusterARN)
	assert.NotContains(t, config.AuthInfos, prodUserPreRealm)
	assert.NotContains(t, config.Clusters, prodClusterARN)
	assert.Contains(t, config.Contexts, "other")
}

// TestEKSIntegration_Execute_ErrorModeRealmContext verifies that update: error
// stays idempotent with the realm-prefixed context name, migrates the
// unprefixed context of this integration, and still fails on a real conflict.
func TestEKSIntegration_Execute_ErrorModeRealmContext(t *testing.T) {
	t.Run("identical re-run is a no-op", func(t *testing.T) {
		stubProdEKSCluster(t)
		kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
		require.NoError(t, newErrorModeRealmEKSIntegration(t, kubeconfigPath, realmRepoA).Execute(t.Context(), nil))
		before, err := os.ReadFile(kubeconfigPath)
		require.NoError(t, err)

		require.NoError(t, newErrorModeRealmEKSIntegration(t, kubeconfigPath, realmRepoA).Execute(t.Context(), nil))

		after, err := os.ReadFile(kubeconfigPath)
		require.NoError(t, err)
		assert.Equal(t, string(before), string(after))
	})

	t.Run("migrates the unprefixed own context", func(t *testing.T) {
		stubProdEKSCluster(t)
		kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
		writeEKSEntry(t, kubeconfigPath, prodClusterARN, "prod", "us-east-2", "", "admin")

		require.NoError(t, newErrorModeRealmEKSIntegration(t, kubeconfigPath, realmRepoA).Execute(t.Context(), nil))

		config, err := loadKubeconfig(kubeconfigPath)
		require.NoError(t, err)
		assert.NotContains(t, config.Contexts, prodClusterARN)
		assert.Equal(t, prodUserRepoA, config.Contexts[realmRepoA+"/"+prodClusterARN].AuthInfo)
	})

	t.Run("conflicting realm context errors", func(t *testing.T) {
		stubProdEKSCluster(t)
		kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
		require.NoError(t, newErrorModeRealmEKSIntegration(t, kubeconfigPath, realmRepoA).Execute(t.Context(), nil))
		// Repoint the realm context at another user; cluster and user stay identical.
		config, err := loadKubeconfig(kubeconfigPath)
		require.NoError(t, err)
		contextName := realmRepoA + "/" + prodClusterARN
		require.Contains(t, config.Contexts, contextName)
		config.Contexts[contextName].AuthInfo = foreignUser
		require.NoError(t, clientcmd.WriteToFile(*config, kubeconfigPath))

		err = newErrorModeRealmEKSIntegration(t, kubeconfigPath, realmRepoA).Execute(t.Context(), nil)
		require.ErrorIs(t, err, errUtils.ErrKubeconfigMerge)
		assert.Contains(t, err.Error(), "context "+contextName)
	})
}

// TestEKSIntegration_Cleanup_PicksOwnAccountByRealmContext verifies that, with
// same-named clusters of two accounts written under realm-prefixed default
// context names, cleanup attributes the cluster by its realm context and
// removes only that one.
func TestEKSIntegration_Cleanup_PicksOwnAccountByRealmContext(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	for _, entry := range []struct{ arn, identity string }{
		{prodClusterARN, "prod-admin"},
		{prodClusterARNOther, "other-admin"},
	} {
		mgr, err := kube.NewKubeconfigManager(kubeconfigPath, "")
		require.NoError(t, err)
		clusterInfo, err := awsCloud.BuildKubeClusterInfo(&awsCloud.EKSClusterInfo{
			Name: "prod", Endpoint: "https://example.eks.amazonaws.com", CertificateAuthorityData: "dGVzdA==",
			ARN: entry.arn, Region: "us-east-2",
		}, entry.identity)
		require.NoError(t, err)
		clusterInfo.Realm = realmRepoA
		_, err = mgr.WriteClusterConfig(clusterInfo, "", "merge")
		require.NoError(t, err)
	}

	integration := newCleanupIntegration(kubeconfigPath, "prod", "us-east-2", "", "other-admin")
	integration.realm = realmRepoA
	require.NoError(t, integration.Cleanup(t.Context()))

	config, err := loadKubeconfig(kubeconfigPath)
	require.NoError(t, err)
	assert.NotContains(t, config.Clusters, prodClusterARNOther)
	assert.NotContains(t, config.Contexts, realmRepoA+"/"+prodClusterARNOther)
	assert.Contains(t, config.Clusters, prodClusterARN)
	assert.Equal(t, "prod-admin", contextIdentity(t, config, realmRepoA+"/"+prodClusterARN))
}

// TestEKSIntegration_SelectOwnedClusterARN_UnreadableKubeconfig verifies that a
// kubeconfig that cannot be read while attributing same-named clusters fails
// the selection instead of guessing.
func TestEKSIntegration_SelectOwnedClusterARN_UnreadableKubeconfig(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	require.NoError(t, os.WriteFile(kubeconfigPath, []byte("{not: [valid"), 0o600))
	mgr, err := kube.NewKubeconfigManager(kubeconfigPath, "")
	require.NoError(t, err)
	integration := newCleanupIntegration(kubeconfigPath, "prod", "us-east-2", "", "admin")
	integration.realm = realmRepoA

	_, err = integration.selectOwnedClusterARN(mgr, []string{prodClusterARN, prodClusterARNOther})
	require.ErrorIs(t, err, errUtils.ErrKubeconfigMerge)
}

// mapKeys returns the keys of m.
func mapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}
