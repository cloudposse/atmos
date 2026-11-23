package kube

// Tests that migrating an older user name checks which identity the older
// user's exec plugin authenticates as, so another identity's entry is never
// treated as this entry's own.

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	errUtils "github.com/cloudposse/atmos/errors"
)

// addLegacyContextForIdentity adds a bare-ID context that points at testLegacyUser,
// whose exec plugin names legacyIdentity (no ATMOS_IDENTITY env when empty).
func addLegacyContextForIdentity(t *testing.T, path string, info *ClusterInfo, legacyIdentity string) {
	t.Helper()

	addTestContext(t, path, info.ID, info.ID, testLegacyUser)
	config := loadTestKubeconfig(t, path)
	// Use the cluster the write produces, so only the user and context can conflict.
	config.Clusters[info.ID] = BuildClusterConfig(info, "").Clusters[info.ID]
	exec := &clientcmdapi.ExecConfig{Command: "atmos"}
	if legacyIdentity != "" {
		exec.Env = []clientcmdapi.ExecEnvVar{{Name: identityExecEnv, Value: legacyIdentity}}
	}
	config.AuthInfos[testLegacyUser] = &clientcmdapi.AuthInfo{Exec: exec}
	require.NoError(t, clientcmd.WriteToFile(*config, path))
}

func TestWriteClusterConfig_LegacyUserOfAnotherIdentity(t *testing.T) {
	info := realmClusterInfo("")
	info.Identity = "admin"

	t.Run("error mode reports a conflict", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "kubeconfig")
		addLegacyContextForIdentity(t, path, info, "other-admin")
		mgr, err := NewKubeconfigManager(path, "")
		require.NoError(t, err)

		_, err = mgr.WriteClusterConfig(info, "", "error")
		require.ErrorIs(t, err, errUtils.ErrKubeconfigMerge)
	})

	t.Run("merge mode keeps the other identity's user", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "kubeconfig")
		addLegacyContextForIdentity(t, path, info, "other-admin")
		mgr, err := NewKubeconfigManager(path, "")
		require.NoError(t, err)

		_, err = mgr.WriteClusterConfig(info, "", "merge")
		require.NoError(t, err)

		config := loadTestKubeconfig(t, path)
		assert.Contains(t, config.AuthInfos, testLegacyUser, "another identity's user is not removed")
	})
}

func TestWriteClusterConfig_LegacyUserOfSameOrNoIdentityMigrates(t *testing.T) {
	info := realmClusterInfo("")
	info.Identity = "admin"

	for _, legacyIdentity := range []string{"admin", ""} {
		t.Run("legacy identity "+legacyIdentity, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "kubeconfig")
			addLegacyContextForIdentity(t, path, info, legacyIdentity)
			mgr, err := NewKubeconfigManager(path, "")
			require.NoError(t, err)

			_, err = mgr.WriteClusterConfig(info, "", "error")
			require.NoError(t, err)

			config := loadTestKubeconfig(t, path)
			assert.Equal(t, UserName(info), config.Contexts[info.ID].AuthInfo)
			assert.NotContains(t, config.AuthInfos, testLegacyUser)
		})
	}
}
