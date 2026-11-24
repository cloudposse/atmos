package kube

// Tests for reference-aware kubeconfig entry removal (remove.go).

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"

	errUtils "github.com/cloudposse/atmos/errors"
)

// sharedClusterInfo returns a fixture for one cluster seen through identity.
func sharedClusterInfo(identity string) *ClusterInfo {
	info := testClusterInfo()
	info.Identity = identity
	info.ExecArgs = []string{"aws", "eks", "token", "--identity=" + identity}
	return info
}

func TestRemoveClusterConfig_KeepsClusterUsedByOtherContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubeconfig")
	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	admin, readonly := sharedClusterInfo("admin"), sharedClusterInfo("readonly")
	_, err = mgr.WriteClusterConfig(admin, "prod-admin", "merge")
	require.NoError(t, err)
	_, err = mgr.WriteClusterConfig(readonly, "prod-readonly", "merge")
	require.NoError(t, err)

	require.NoError(t, mgr.RemoveClusterConfig(readonly.ID, "prod-readonly", UserName(readonly)))

	loaded, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	assert.NotContains(t, loaded.Contexts, "prod-readonly")
	assert.NotContains(t, loaded.AuthInfos, UserName(readonly))
	assert.Contains(t, loaded.Clusters, admin.ID, "the cluster is still used by prod-admin")
	assert.Contains(t, loaded.Contexts, "prod-admin")
	assert.Contains(t, loaded.AuthInfos, UserName(admin))
}

func TestRemoveClusterConfig_ExtraUserNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubeconfig")
	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	// Two contexts share one user, as older versions wrote them.
	legacy := testClusterInfo()
	_, err = mgr.WriteClusterConfig(legacy, "one", "merge")
	require.NoError(t, err)
	_, err = mgr.WriteClusterConfig(legacy, "two", "merge")
	require.NoError(t, err)
	// A user no context references, left behind by an upgrade.
	orphan := sharedClusterInfo("orphan")
	_, err = mgr.WriteClusterConfig(orphan, "orphan", "merge")
	require.NoError(t, err)
	existing, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	delete(existing.Contexts, "orphan")
	require.NoError(t, clientcmd.WriteToFile(*existing, path))

	// Removing "one" keeps the shared user because "two" still uses it, and
	// removes the unreferenced extra user. Empty names are ignored.
	require.NoError(t, mgr.RemoveClusterConfig(legacy.ID, "one", "", UserName(legacy), UserName(orphan)))

	loaded, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	assert.NotContains(t, loaded.Contexts, "one")
	assert.Contains(t, loaded.AuthInfos, UserName(legacy))
	assert.NotContains(t, loaded.AuthInfos, UserName(orphan))

	// Removing "two" leaves nothing, so the file is removed.
	require.NoError(t, mgr.RemoveClusterConfig(legacy.ID, "two", UserName(legacy)))
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err))
}

func TestRemoveClusterConfig_InvalidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubeconfig")
	require.NoError(t, os.WriteFile(path, []byte("{not: [valid"), 0o600))
	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	err = mgr.RemoveClusterConfig("id", "ctx", "user")
	require.ErrorIs(t, err, errUtils.ErrKubeconfigMerge)
}

func TestLookupContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubeconfig")
	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	// A missing file has no contexts.
	got, err := mgr.LookupContext("dev-eks")
	require.NoError(t, err)
	assert.Nil(t, got)

	info := testClusterInfo()
	_, err = mgr.WriteClusterConfig(info, "dev-eks", "merge")
	require.NoError(t, err)

	got, err = mgr.LookupContext("dev-eks")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, info.ID, got.Cluster)
	assert.Equal(t, UserName(info), got.AuthInfo)

	got, err = mgr.LookupContext("missing")
	require.NoError(t, err)
	assert.Nil(t, got)

	require.NoError(t, os.WriteFile(path, []byte("{not: [valid"), 0o600))
	_, err = mgr.LookupContext("dev-eks")
	require.ErrorIs(t, err, errUtils.ErrKubeconfigMerge)
}
