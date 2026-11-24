package kube

// Tests that cleanup removes only the entries of the cluster and identity it
// was asked to remove, and that "error" update mode migrates legacy users.

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	errUtils "github.com/cloudposse/atmos/errors"
)

// setUserIdentity gives the user at path an exec plugin that authenticates as identity.
func setUserIdentity(t *testing.T, path, userName, identity string) {
	t.Helper()

	config := loadTestKubeconfig(t, path)
	exec := &clientcmdapi.ExecConfig{APIVersion: execAPIVersion, Command: "atmos"}
	if identity != "" {
		exec.Env = []clientcmdapi.ExecEnvVar{{Name: identityExecEnv, Value: identity}}
	}
	config.AuthInfos[userName] = &clientcmdapi.AuthInfo{Exec: exec}
	require.NoError(t, clientcmd.WriteToFile(*config, path))
}

func TestRemoveOwnedClusterConfig_KeepsContextOfAnotherCluster(t *testing.T) {
	info := testClusterInfo()
	owned := OwnedUserNames(info)
	otherCluster := "arn:aws:eks:us-east-2:123456789012:cluster/other"

	t.Run("same alias and shared user but another cluster", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "kubeconfig")
		addTestContext(t, path, "dev-eks", otherCluster, UserName(info))
		mgr, err := NewKubeconfigManager(path, "")
		require.NoError(t, err)

		require.NoError(t, mgr.RemoveOwnedClusterConfig(info.ID, []string{"dev-eks"}, owned))

		config := loadTestKubeconfig(t, path)
		require.Contains(t, config.Contexts, "dev-eks")
		assert.Equal(t, otherCluster, config.Contexts["dev-eks"].Cluster)
		assert.Contains(t, config.AuthInfos, UserName(info), "the user is still referenced")
		assert.Contains(t, config.Clusters, otherCluster)
	})

	t.Run("the same cluster is removed", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "kubeconfig")
		addTestContext(t, path, "dev-eks", info.ID, UserName(info))
		mgr, err := NewKubeconfigManager(path, "")
		require.NoError(t, err)

		require.NoError(t, mgr.RemoveOwnedClusterConfig(info.ID, []string{"dev-eks"}, owned))

		assert.NoFileExists(t, path)
	})
}

func TestRemoveOwnedClusterConfig_WithIdentity(t *testing.T) {
	info := testClusterInfo()
	legacyUser := "atmos-eks-dev-cluster-us-east-2"
	owned := append(OwnedUserNames(info), legacyUser)

	for _, tt := range []struct {
		name        string
		userIdent   string
		wantRemoved bool
	}{
		{name: "legacy entry of another identity is kept", userIdent: "other-admin", wantRemoved: false},
		{name: "entry of the same identity is removed", userIdent: "dev-admin", wantRemoved: true},
		{name: "entry naming no identity is removed", userIdent: "", wantRemoved: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "kubeconfig")
			addTestContext(t, path, info.ID, info.ID, legacyUser)
			setUserIdentity(t, path, legacyUser, tt.userIdent)
			mgr, err := NewKubeconfigManager(path, "")
			require.NoError(t, err)

			require.NoError(t, mgr.RemoveOwnedClusterConfig(info.ID, []string{info.ID}, owned, WithIdentity("dev-admin")))

			if tt.wantRemoved {
				assert.NoFileExists(t, path)
				return
			}
			config := loadTestKubeconfig(t, path)
			assert.Contains(t, config.Contexts, info.ID)
			assert.Contains(t, config.AuthInfos, legacyUser)
		})
	}

	t.Run("without WithIdentity any identity is removed", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "kubeconfig")
		addTestContext(t, path, info.ID, info.ID, legacyUser)
		setUserIdentity(t, path, legacyUser, "other-admin")
		mgr, err := NewKubeconfigManager(path, "")
		require.NoError(t, err)

		require.NoError(t, mgr.RemoveOwnedClusterConfig(info.ID, []string{info.ID}, owned))

		assert.NoFileExists(t, path)
	})
}

// writeLegacyUserContext writes info under alias, then repoints the context at
// a copy of its user named legacyUser and removes the current user, as an
// older version that named the user differently would have left it.
func writeLegacyUserContext(t *testing.T, path string, info *ClusterInfo, alias, legacyUser string) {
	t.Helper()

	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)
	_, err = mgr.WriteClusterConfig(info, alias, "merge")
	require.NoError(t, err)
	config := loadTestKubeconfig(t, path)
	config.AuthInfos[legacyUser] = config.AuthInfos[UserName(info)]
	delete(config.AuthInfos, UserName(info))
	config.Contexts[alias].AuthInfo = legacyUser
	require.NoError(t, clientcmd.WriteToFile(*config, path))
}

func TestWriteClusterConfig_ErrorModeMigratesLegacyUser(t *testing.T) {
	info := testClusterInfo()
	legacyUser := testLegacyUser
	info.LegacyUserNames = []string{legacyUser}

	t.Run("owned context on a legacy user is migrated", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "kubeconfig")
		writeLegacyUserContext(t, path, info, errorModeAlias, legacyUser)
		mgr, err := NewKubeconfigManager(path, "")
		require.NoError(t, err)

		changed, err := mgr.WriteClusterConfig(info, errorModeAlias, "error")

		require.NoError(t, err)
		assert.True(t, changed)
		config := loadTestKubeconfig(t, path)
		assert.Equal(t, UserName(info), config.Contexts[errorModeAlias].AuthInfo)
		assert.NotContains(t, config.AuthInfos, legacyUser, "the legacy user is removed once unreferenced")
	})

	t.Run("context on an unrelated user still conflicts", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "kubeconfig")
		writeLegacyUserContext(t, path, info, errorModeAlias, testOtherUser)
		mgr, err := NewKubeconfigManager(path, "")
		require.NoError(t, err)

		_, err = mgr.WriteClusterConfig(info, errorModeAlias, "error")

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrKubeconfigMerge)
	})

	t.Run("legacy user on another cluster still conflicts", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "kubeconfig")
		writeLegacyUserContext(t, path, info, errorModeAlias, legacyUser)
		config := loadTestKubeconfig(t, path)
		config.Contexts[errorModeAlias].Cluster = "arn:aws:eks:us-east-2:123456789012:cluster/other"
		require.NoError(t, clientcmd.WriteToFile(*config, path))
		mgr, err := NewKubeconfigManager(path, "")
		require.NoError(t, err)

		_, err = mgr.WriteClusterConfig(info, errorModeAlias, "error")

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrKubeconfigMerge)
	})
}
