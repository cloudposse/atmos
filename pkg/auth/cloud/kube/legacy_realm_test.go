package kube

// Tests for derived (not explicitly configured) realms: they never change kube
// names, and names that unreleased builds wrote under them are cleaned up.

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testAutoRealm is a realm Atmos derives from a hash of the atmos.yaml path.
const testAutoRealm = "462813558259d41c"

// autoRealmClusterInfo returns the fixture as written without an explicit realm
// by a build that knows testAutoRealm was the derived realm.
func autoRealmClusterInfo() *ClusterInfo {
	info := realmClusterInfo("")
	info.Realm, info.LegacyRealms = NameRealm(testAutoRealm, false)
	return info
}

func TestNameRealm(t *testing.T) {
	for _, tt := range []struct {
		name       string
		value      string
		explicit   bool
		wantRealm  string
		wantLegacy []string
	}{
		{name: "no realm"},
		{name: "no realm marked explicit", explicit: true},
		{name: "explicit realm", value: testRealm, explicit: true, wantRealm: testRealm},
		{name: "derived realm", value: testAutoRealm, wantLegacy: []string{testAutoRealm}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gotRealm, gotLegacy := NameRealm(tt.value, tt.explicit)
			assert.Equal(t, tt.wantRealm, gotRealm)
			assert.Equal(t, tt.wantLegacy, gotLegacy)
		})
	}
}

func TestContextNames_LegacyRealms(t *testing.T) {
	const id = "arn:aws:eks:us-east-2:111111111111:cluster/dev"
	assert.Equal(t, []string{id, testAutoRealm + "/" + id}, ContextNames(id, "", "", testAutoRealm))
	assert.Equal(t, []string{"acme/" + id, id, testAutoRealm + "/" + id}, ContextNames(id, "", testRealm, testAutoRealm))
	// A legacy realm equal to the current realm is not listed twice.
	assert.Equal(t, []string{"acme/" + id, id}, ContextNames(id, "", testRealm, testRealm))
	// An alias is the only name, whatever the realms.
	assert.Equal(t, []string{"dev"}, ContextNames(id, "dev", "", testAutoRealm))
}

func TestOwnedUserNames_LegacyRealms(t *testing.T) {
	info := autoRealmClusterInfo()
	assert.Equal(t, []string{
		"atmos-eks-dev-cluster-us-east-2",
		"atmos-" + testAutoRealm + "-eks-dev-cluster-us-east-2",
		testLegacyUser,
	}, OwnedUserNames(info))
}

func TestBuildClusterConfig_DerivedRealmKeepsNames(t *testing.T) {
	info := autoRealmClusterInfo()

	config := BuildClusterConfig(info, "")

	assert.Equal(t, info.ID, config.CurrentContext, "the default context stays the bare cluster ID")
	require.Contains(t, config.Contexts, info.ID)
	user := config.AuthInfos[userNameInRealm(info, "")]
	require.NotNil(t, user, "the user name has no realm")
	for _, env := range user.Exec.Env {
		assert.NotEqual(t, "ATMOS_AUTH_REALM", env.Name, "a derived realm is not pinned in the exec env")
	}
}

// TestWriteClusterConfig_MigratesDerivedRealmNames is the upgrade path from an
// unreleased build that prefixed the context and user with the derived realm:
// the next write restores the bare-ID context and removes the prefixed entries.
func TestWriteClusterConfig_MigratesDerivedRealmNames(t *testing.T) {
	for _, mode := range []string{"merge", "error"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "kubeconfig")
			info := autoRealmClusterInfo()
			prefixedUser := userNameInRealm(info, testAutoRealm)
			mgr, err := NewKubeconfigManager(path, "")
			require.NoError(t, err)
			// What the unreleased build wrote: the derived realm used as a name realm.
			_, err = mgr.WriteClusterConfig(realmClusterInfo(testAutoRealm), "", "merge")
			require.NoError(t, err)
			// Another repository's derived-realm context, and an unrelated entry.
			addTestContext(t, path, "0123456789abcdef/"+info.ID, info.ID, userNameInRealm(info, "0123456789abcdef"))
			addTestContext(t, path, "unrelated", "other-cluster", testOtherUser)

			changed, err := mgr.WriteClusterConfig(info, "", mode)
			require.NoError(t, err)
			assert.True(t, changed)

			config := loadTestKubeconfig(t, path)
			assert.Equal(t, info.ID, config.CurrentContext)
			require.Contains(t, config.Contexts, info.ID)
			assert.Equal(t, UserName(info), config.Contexts[info.ID].AuthInfo)
			assert.NotContains(t, config.Contexts, testAutoRealm+"/"+info.ID)
			assert.NotContains(t, config.AuthInfos, prefixedUser)
			assert.Contains(t, config.Contexts, "0123456789abcdef/"+info.ID, "another realm's context is kept")
			assert.Contains(t, config.AuthInfos, userNameInRealm(info, "0123456789abcdef"))
			assert.Contains(t, config.Contexts, "unrelated")

			changed, err = mgr.WriteClusterConfig(info, "", mode)
			require.NoError(t, err)
			assert.False(t, changed, "the re-run has nothing left to migrate")
		})
	}
}

// TestWriteClusterConfig_RepointedContextDropsOldUser covers an alias (or a
// bare-ID context) that an older version pointed at one of this entry's older
// users: the write repoints it, and the older user is removed once nothing
// else uses it.
func TestWriteClusterConfig_RepointedContextDropsOldUser(t *testing.T) {
	for _, tt := range []struct {
		name, alias, oldUser string
	}{
		{name: "alias with derived-realm user", alias: "dev", oldUser: userNameInRealm(autoRealmClusterInfo(), testAutoRealm)},
		{name: "bare ID with legacy user", oldUser: testLegacyUser},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "kubeconfig")
			info := autoRealmClusterInfo()
			contextName := ContextName(info.ID, tt.alias, "")
			addTestContext(t, path, contextName, info.ID, tt.oldUser)
			mgr, err := NewKubeconfigManager(path, "")
			require.NoError(t, err)

			_, err = mgr.WriteClusterConfig(info, tt.alias, "merge")
			require.NoError(t, err)

			config := loadTestKubeconfig(t, path)
			require.Contains(t, config.Contexts, contextName)
			assert.Equal(t, UserName(info), config.Contexts[contextName].AuthInfo)
			assert.NotContains(t, config.AuthInfos, tt.oldUser)
		})
	}
}

// TestWriteClusterConfig_RepointKeepsSharedOldUser is the negative path: an
// older user that another context still uses is kept.
func TestWriteClusterConfig_RepointKeepsSharedOldUser(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubeconfig")
	info := autoRealmClusterInfo()
	addTestContext(t, path, info.ID, info.ID, testLegacyUser)
	addTestContext(t, path, "legacy-alias", info.ID, testLegacyUser)
	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	_, err = mgr.WriteClusterConfig(info, "", "merge")
	require.NoError(t, err)

	config := loadTestKubeconfig(t, path)
	assert.Equal(t, UserName(info), config.Contexts[info.ID].AuthInfo)
	assert.Equal(t, testLegacyUser, config.Contexts["legacy-alias"].AuthInfo)
	assert.Contains(t, config.AuthInfos, testLegacyUser)
}

func TestRemoveOwnedClusterConfig_DerivedRealmNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubeconfig")
	info := autoRealmClusterInfo()
	prefixedUser := userNameInRealm(info, testAutoRealm)
	addTestContext(t, path, testAutoRealm+"/"+info.ID, info.ID, prefixedUser)
	addTestContext(t, path, info.ID, info.ID, UserName(info))
	addTestContext(t, path, "0123456789abcdef/"+info.ID, info.ID, testOtherUser)
	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	require.NoError(t, mgr.RemoveOwnedClusterConfig(info.ID, ContextNames(info.ID, "", info.Realm, info.LegacyRealms...), OwnedUserNames(info)))

	config := loadTestKubeconfig(t, path)
	assert.NotContains(t, config.Contexts, testAutoRealm+"/"+info.ID)
	assert.NotContains(t, config.Contexts, info.ID)
	assert.NotContains(t, config.AuthInfos, prefixedUser)
	assert.NotContains(t, config.AuthInfos, UserName(info))
	assert.Contains(t, config.Contexts, "0123456789abcdef/"+info.ID, "a context of another user is kept")
	assert.Contains(t, config.Clusters, info.ID, "the kept context still uses the cluster")
}
