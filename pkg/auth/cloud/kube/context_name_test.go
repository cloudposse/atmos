package kube

// Tests for realm-prefixed default context names, the migration of an older
// unprefixed context, and owner-aware context removal (issue #3720).

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
	testRealm      = "acme"
	testOtherRealm = "other"
	testOtherUser  = "someone-else"
	testLegacyUser = "atmos-eks-dev-cluster-us-east-2-legacy"
)

// realmClusterInfo returns the default fixture written from realm, with one
// legacy user name.
func realmClusterInfo(realm string) *ClusterInfo {
	info := testClusterInfo()
	info.Realm = realm
	info.LegacyUserNames = []string{testLegacyUser}
	return info
}

// loadTestKubeconfig loads the kubeconfig at path.
func loadTestKubeconfig(t *testing.T, path string) *clientcmdapi.Config {
	t.Helper()

	config, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	return config
}

// addTestContext adds a context, its cluster and a token user to the
// kubeconfig at path, as another tool or an older version would have.
func addTestContext(t *testing.T, path, contextName, clusterID, userName string) {
	t.Helper()

	config := clientcmdapi.NewConfig()
	if _, err := os.Stat(path); err == nil {
		config = loadTestKubeconfig(t, path)
	}
	if _, ok := config.Clusters[clusterID]; !ok {
		config.Clusters[clusterID] = &clientcmdapi.Cluster{Server: "https://example.invalid"}
	}
	config.AuthInfos[userName] = &clientcmdapi.AuthInfo{Token: "token"}
	config.Contexts[contextName] = &clientcmdapi.Context{Cluster: clusterID, AuthInfo: userName}
	config.CurrentContext = contextName
	require.NoError(t, clientcmd.WriteToFile(*config, path))
}

func TestContextName(t *testing.T) {
	const id = "arn:aws:eks:us-east-1:111111111111:cluster/prod"
	for _, tt := range []struct {
		name, alias, realm, want string
		wantAll                  []string
	}{
		{name: "no realm, no alias", want: id, wantAll: []string{id}},
		{name: "realm, no alias", realm: testRealm, want: "acme/" + id, wantAll: []string{"acme/" + id, id}},
		{name: "alias, no realm", alias: "prod", want: "prod", wantAll: []string{"prod"}},
		{name: "alias wins over realm", alias: "prod", realm: testRealm, want: "prod", wantAll: []string{"prod"}},
		{name: "alias equal to cluster ID", alias: id, realm: testRealm, want: id, wantAll: []string{id}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ContextName(id, tt.alias, tt.realm))
			assert.Equal(t, tt.wantAll, ContextNames(id, tt.alias, tt.realm))
		})
	}
}

func TestOwnedUserNames(t *testing.T) {
	t.Run("no realm", func(t *testing.T) {
		info := realmClusterInfo("")
		assert.Equal(t, []string{"atmos-eks-dev-cluster-us-east-2", testLegacyUser}, OwnedUserNames(info))
	})

	t.Run("realm adds the pre-realm name", func(t *testing.T) {
		info := realmClusterInfo(testRealm)
		assert.Equal(t, []string{
			"atmos-acme-eks-dev-cluster-us-east-2-30ea62b8",
			"atmos-acme-eks-dev-cluster-us-east-2",
			"atmos-eks-dev-cluster-us-east-2",
			testLegacyUser,
		}, OwnedUserNames(info))
		assert.Equal(t, testRealm, info.Realm, "the input must not be modified")
	})

	t.Run("duplicate and empty legacy names are skipped", func(t *testing.T) {
		info := testClusterInfo()
		info.LegacyUserNames = []string{"", "atmos-eks-dev-cluster-us-east-2", testLegacyUser, testLegacyUser}
		assert.Equal(t, []string{"atmos-eks-dev-cluster-us-east-2", testLegacyUser}, OwnedUserNames(info))
	})
}

func TestBuildClusterConfig_RealmPrefixesDefaultContext(t *testing.T) {
	info := realmClusterInfo(testRealm)

	config := BuildClusterConfig(info, "")

	wantContext := "acme/" + info.ID
	assert.Equal(t, wantContext, config.CurrentContext)
	require.Contains(t, config.Contexts, wantContext)
	assert.Equal(t, info.ID, config.Contexts[wantContext].Cluster, "the cluster key stays the bare ID")
	assert.Contains(t, config.Clusters, info.ID)
	assert.Equal(t, UserName(info), config.Contexts[wantContext].AuthInfo)
}

func TestWriteClusterConfig_MigratesUnprefixedOwnContext(t *testing.T) {
	for _, oldUser := range []string{"atmos-acme-eks-dev-cluster-us-east-2", "atmos-eks-dev-cluster-us-east-2", testLegacyUser} {
		t.Run(oldUser, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "kubeconfig")
			info := realmClusterInfo(testRealm)
			addTestContext(t, path, info.ID, info.ID, oldUser)
			// An unrelated entry that the migration must keep.
			addTestContext(t, path, "unrelated", "other-cluster", testOtherUser)
			mgr, err := NewKubeconfigManager(path, "")
			require.NoError(t, err)

			changed, err := mgr.WriteClusterConfig(info, "", "merge")
			require.NoError(t, err)
			assert.True(t, changed)

			config := loadTestKubeconfig(t, path)
			newContext := "acme/" + info.ID
			assert.NotContains(t, config.Contexts, info.ID)
			require.Contains(t, config.Contexts, newContext)
			assert.Equal(t, UserName(info), config.Contexts[newContext].AuthInfo)
			assert.Equal(t, newContext, config.CurrentContext)
			assert.Contains(t, config.AuthInfos, UserName(info))
			if oldUser != UserName(info) {
				assert.NotContains(t, config.AuthInfos, oldUser, "the old user is removed once unreferenced")
			}
			assert.Contains(t, config.Contexts, "unrelated")
			assert.Contains(t, config.AuthInfos, testOtherUser)
			assert.Contains(t, config.Clusters, "other-cluster")

			// The re-run finds nothing to migrate and nothing to merge.
			changed, err = mgr.WriteClusterConfig(info, "", "merge")
			require.NoError(t, err)
			assert.False(t, changed)
		})
	}
}

func TestWriteClusterConfig_MigrationKeepsOldUserStillInUse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubeconfig")
	info := realmClusterInfo(testRealm)
	addTestContext(t, path, info.ID, info.ID, testLegacyUser)
	// Another context written by an older version shares the legacy user.
	addTestContext(t, path, "legacy-alias", info.ID, testLegacyUser)
	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	_, err = mgr.WriteClusterConfig(info, "", "merge")
	require.NoError(t, err)

	config := loadTestKubeconfig(t, path)
	assert.NotContains(t, config.Contexts, info.ID)
	assert.Contains(t, config.Contexts, "legacy-alias")
	assert.Contains(t, config.AuthInfos, testLegacyUser, "the legacy-alias context still uses it")
}

// TestWriteClusterConfig_MigrationOnlyChange verifies that a migration alone
// counts as a change even when the realm-prefixed entries are already current.
func TestWriteClusterConfig_MigrationOnlyChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubeconfig")
	info := realmClusterInfo(testRealm)
	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)
	_, err = mgr.WriteClusterConfig(info, "", "merge")
	require.NoError(t, err)

	// An older version (or a concurrent one) writes the unprefixed context.
	config := loadTestKubeconfig(t, path)
	config.Contexts[info.ID] = &clientcmdapi.Context{Cluster: info.ID, AuthInfo: UserName(info)}
	require.NoError(t, clientcmd.WriteToFile(*config, path))

	changed, err := mgr.WriteClusterConfig(info, "", "merge")
	require.NoError(t, err)
	assert.True(t, changed)
	config = loadTestKubeconfig(t, path)
	assert.NotContains(t, config.Contexts, info.ID)
	assert.Equal(t, "acme/"+info.ID, config.CurrentContext)
}

func TestWriteClusterConfig_NoMigration(t *testing.T) {
	for _, tt := range []struct {
		name, alias, realm, oldUser string
	}{
		{name: "unprefixed context of another user", realm: testRealm, oldUser: testOtherUser},
		{name: "unprefixed context of another realm", realm: testRealm, oldUser: "atmos-other-eks-dev-cluster-us-east-2"},
		{name: "alias keeps the unprefixed context", alias: "dev", realm: testRealm, oldUser: "atmos-eks-dev-cluster-us-east-2"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "kubeconfig")
			info := realmClusterInfo(tt.realm)
			addTestContext(t, path, info.ID, info.ID, tt.oldUser)
			mgr, err := NewKubeconfigManager(path, "")
			require.NoError(t, err)

			_, err = mgr.WriteClusterConfig(info, tt.alias, "merge")
			require.NoError(t, err)

			config := loadTestKubeconfig(t, path)
			require.Contains(t, config.Contexts, info.ID, "the unprefixed context must be left alone")
			assert.Equal(t, tt.oldUser, config.Contexts[info.ID].AuthInfo)
			assert.Contains(t, config.AuthInfos, tt.oldUser)
			assert.Contains(t, config.Contexts, ContextName(info.ID, tt.alias, tt.realm))
		})
	}
}

func TestWriteClusterConfig_ErrorModeRealmContext(t *testing.T) {
	t.Run("identical re-run is a no-op", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "kubeconfig")
		mgr, err := NewKubeconfigManager(path, "")
		require.NoError(t, err)
		changed, err := mgr.WriteClusterConfig(realmClusterInfo(testRealm), "", "error")
		require.NoError(t, err)
		assert.True(t, changed)

		changed, err = mgr.WriteClusterConfig(realmClusterInfo(testRealm), "", "error")
		require.NoError(t, err)
		assert.False(t, changed)
	})

	t.Run("two realms do not conflict", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "kubeconfig")
		mgr, err := NewKubeconfigManager(path, "")
		require.NoError(t, err)
		_, err = mgr.WriteClusterConfig(realmClusterInfo(testRealm), "", "error")
		require.NoError(t, err)

		_, err = mgr.WriteClusterConfig(realmClusterInfo(testOtherRealm), "", "error")
		require.NoError(t, err)

		config := loadTestKubeconfig(t, path)
		assert.Contains(t, config.Contexts, "acme/"+testClusterInfo().ID)
		assert.Contains(t, config.Contexts, "other/"+testClusterInfo().ID)
	})

	t.Run("migrates the unprefixed own context", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "kubeconfig")
		mgr, err := NewKubeconfigManager(path, "")
		require.NoError(t, err)
		// Written by an older version: same cluster, pre-realm user, unprefixed context.
		_, err = mgr.WriteClusterConfig(realmClusterInfo(""), "", "merge")
		require.NoError(t, err)

		changed, err := mgr.WriteClusterConfig(realmClusterInfo(testRealm), "", "error")
		require.NoError(t, err)
		assert.True(t, changed)

		config := loadTestKubeconfig(t, path)
		assert.NotContains(t, config.Contexts, testClusterInfo().ID)
		assert.Contains(t, config.Contexts, "acme/"+testClusterInfo().ID)
		assert.NotContains(t, config.AuthInfos, "atmos-eks-dev-cluster-us-east-2")
	})
}

func TestRemoveOwnedClusterConfig(t *testing.T) {
	info := realmClusterInfo(testRealm)
	owned := OwnedUserNames(info)

	t.Run("removes owned contexts under every name", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "kubeconfig")
		addTestContext(t, path, info.ID, info.ID, "atmos-eks-dev-cluster-us-east-2")
		addTestContext(t, path, "acme/"+info.ID, info.ID, UserName(info))
		mgr, err := NewKubeconfigManager(path, "")
		require.NoError(t, err)

		require.NoError(t, mgr.RemoveOwnedClusterConfig(info.ID, ContextNames(info.ID, "", testRealm), owned))

		_, err = os.Stat(path)
		assert.True(t, os.IsNotExist(err), "nothing is left, so the file is removed")
	})

	t.Run("keeps contexts of other users", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "kubeconfig")
		addTestContext(t, path, info.ID, info.ID, testOtherUser)
		addTestContext(t, path, "acme/"+info.ID, info.ID, "atmos-other-eks-dev-cluster-us-east-2")
		mgr, err := NewKubeconfigManager(path, "")
		require.NoError(t, err)

		require.NoError(t, mgr.RemoveOwnedClusterConfig(info.ID, ContextNames(info.ID, "", testRealm), owned))

		config := loadTestKubeconfig(t, path)
		assert.Equal(t, testOtherUser, config.Contexts[info.ID].AuthInfo)
		assert.Equal(t, "atmos-other-eks-dev-cluster-us-east-2", config.Contexts["acme/"+info.ID].AuthInfo)
		assert.Contains(t, config.Clusters, info.ID)
		assert.Contains(t, config.AuthInfos, testOtherUser)
		assert.Equal(t, "acme/"+info.ID, config.CurrentContext, "a kept context stays current")
	})

	t.Run("clears current-context only for a removed context", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "kubeconfig")
		addTestContext(t, path, "keep", "other-cluster", testOtherUser)
		addTestContext(t, path, "acme/"+info.ID, info.ID, UserName(info))
		mgr, err := NewKubeconfigManager(path, "")
		require.NoError(t, err)

		require.NoError(t, mgr.RemoveOwnedClusterConfig(info.ID, ContextNames(info.ID, "", testRealm), owned))

		config := loadTestKubeconfig(t, path)
		assert.Empty(t, config.CurrentContext)
		assert.NotContains(t, config.Contexts, "acme/"+info.ID)
		assert.NotContains(t, config.Clusters, info.ID)
		assert.NotContains(t, config.AuthInfos, UserName(info))
		assert.Contains(t, config.Contexts, "keep")
	})

	t.Run("missing file is a no-op", func(t *testing.T) {
		mgr, err := NewKubeconfigManager(filepath.Join(t.TempDir(), "kubeconfig"), "")
		require.NoError(t, err)
		require.NoError(t, mgr.RemoveOwnedClusterConfig(info.ID, []string{info.ID}, owned))
	})
}

func TestRemoveClusterConfig_KeepsContextOfOtherUser(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubeconfig")
	info := testClusterInfo()
	addTestContext(t, path, "dev-eks", info.ID, testOtherUser)
	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	require.NoError(t, mgr.RemoveClusterConfig(info.ID, "dev-eks", UserName(info)))

	config := loadTestKubeconfig(t, path)
	require.Contains(t, config.Contexts, "dev-eks")
	assert.Equal(t, testOtherUser, config.Contexts["dev-eks"].AuthInfo)
	assert.Contains(t, config.Clusters, info.ID)
	assert.Equal(t, "dev-eks", config.CurrentContext)
}

// TestWriteClusterConfig_MigratesUnhashedUserNames verifies that the unhashed
// realm- and identity-scoped user names earlier builds wrote are still treated
// as owned: the context is migrated and the old user removed, in both modes.
func TestWriteClusterConfig_MigratesUnhashedUserNames(t *testing.T) {
	for _, mode := range []string{"merge", "error"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "kubeconfig")
			info := realmClusterInfo(testRealm)
			info.Identity = "dev-admin"
			unhashed := UnhashedUserName(info)
			require.NotEqual(t, unhashed, UserName(info))
			addTestContext(t, path, info.ID, info.ID, unhashed)
			// Error mode only migrates when the existing cluster is identical.
			seeded := loadTestKubeconfig(t, path)
			seeded.Clusters[info.ID] = BuildClusterConfig(info, "").Clusters[info.ID]
			require.NoError(t, clientcmd.WriteToFile(*seeded, path))
			mgr, err := NewKubeconfigManager(path, "")
			require.NoError(t, err)

			changed, err := mgr.WriteClusterConfig(info, "", mode)
			require.NoError(t, err)
			assert.True(t, changed)

			config := loadTestKubeconfig(t, path)
			newContext := "acme/" + info.ID
			assert.NotContains(t, config.Contexts, info.ID)
			assert.Equal(t, UserName(info), config.Contexts[newContext].AuthInfo)
			assert.NotContains(t, config.AuthInfos, unhashed)
			assert.Contains(t, config.AuthInfos, UserName(info))
		})
	}
}
