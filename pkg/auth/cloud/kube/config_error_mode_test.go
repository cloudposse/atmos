package kube

// Tests that the "error" update mode is idempotent: re-writing entries that
// already match, ignoring only the per-checkout ATMOS_CHDIR exec env value, is
// not a conflict, while a real difference still is (issue #3720, #3675).

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	errUtils "github.com/cloudposse/atmos/errors"
)

const errorModeAlias = "dev-eks"

// errorModeInfo returns the fixture cluster with ATMOS_CHDIR set to dir.
func errorModeInfo(dir string) *ClusterInfo {
	info := testClusterInfo()
	info.ExecEnv = append(info.ExecEnv, clientcmdapi.ExecEnvVar{Name: "ATMOS_CHDIR", Value: dir})
	return info
}

// writeErrorModeFixture writes info in error mode to a new kubeconfig and returns its manager.
func writeErrorModeFixture(t *testing.T, info *ClusterInfo) *KubeconfigManager {
	t.Helper()

	mgr, err := NewKubeconfigManager(filepath.Join(t.TempDir(), "kubeconfig"), "")
	require.NoError(t, err)
	changed, err := mgr.WriteClusterConfig(info, errorModeAlias, "error")
	require.NoError(t, err)
	require.True(t, changed)
	return mgr
}

// TestWriteClusterConfig_ErrorMode_IdenticalRerunIsNoOp verifies that re-running
// the same integration in error mode succeeds without rewriting the file.
func TestWriteClusterConfig_ErrorMode_IdenticalRerunIsNoOp(t *testing.T) {
	dir := t.TempDir()
	mgr := writeErrorModeFixture(t, errorModeInfo(dir))

	changed, err := mgr.WriteClusterConfig(errorModeInfo(dir), errorModeAlias, "error")
	require.NoError(t, err)
	assert.False(t, changed)
}

// TestWriteClusterConfig_ErrorMode_OnlyChdirDiffers verifies that an entry that
// differs only in ATMOS_CHDIR (another checkout of the same repository, or a
// moved one) is not a conflict, and that the directory is refreshed.
func TestWriteClusterConfig_ErrorMode_OnlyChdirDiffers(t *testing.T) {
	oldDir, newDir := t.TempDir(), t.TempDir()
	mgr := writeErrorModeFixture(t, errorModeInfo(oldDir))

	changed, err := mgr.WriteClusterConfig(errorModeInfo(newDir), errorModeAlias, "error")
	require.NoError(t, err)
	assert.True(t, changed)

	config, err := clientcmd.LoadFromFile(mgr.GetPath())
	require.NoError(t, err)
	user := config.AuthInfos[UserName(testClusterInfo())]
	require.NotNil(t, user)
	assert.Contains(t, user.Exec.Env, clientcmdapi.ExecEnvVar{Name: "ATMOS_CHDIR", Value: newDir})
}

// TestWriteClusterConfig_ErrorMode_RealConflicts verifies that every real
// difference in the cluster, context or user still fails in error mode and
// leaves the file unchanged.
func TestWriteClusterConfig_ErrorMode_RealConflicts(t *testing.T) {
	tests := []struct {
		name   string
		alias  string
		mutate func(info *ClusterInfo)
	}{
		{name: "different server", alias: errorModeAlias, mutate: func(info *ClusterInfo) { info.Endpoint = "https://other.example.com" }},
		{name: "different CA", alias: errorModeAlias, mutate: func(info *ClusterInfo) { info.CertificateAuthorityData = "b3RoZXI=" }},
		{name: "different args", alias: errorModeAlias, mutate: func(info *ClusterInfo) { info.ExecArgs = append(info.ExecArgs, "--extra") }},
		{name: "different other env", alias: errorModeAlias, mutate: func(info *ClusterInfo) { info.ExecEnv[0].Value = "other-admin" }},
		{name: "extra env", alias: errorModeAlias, mutate: func(info *ClusterInfo) {
			info.ExecEnv = append(info.ExecEnv, clientcmdapi.ExecEnvVar{Name: "AWS_PROFILE", Value: "x"})
		}},
		{name: "different user under a new context", alias: "dev-eks-2", mutate: func(info *ClusterInfo) { info.ExecArgs = info.ExecArgs[:1] }},
		{name: "context pointing at another user", alias: errorModeAlias, mutate: func(info *ClusterInfo) { info.Identity = "other" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			mgr := writeErrorModeFixture(t, errorModeInfo(dir))
			before, err := clientcmd.LoadFromFile(mgr.GetPath())
			require.NoError(t, err)

			info := errorModeInfo(dir)
			tt.mutate(info)
			changed, err := mgr.WriteClusterConfig(info, tt.alias, "error")
			require.ErrorIs(t, err, errUtils.ErrKubeconfigMerge)
			assert.False(t, changed)

			after, err := clientcmd.LoadFromFile(mgr.GetPath())
			require.NoError(t, err)
			assert.True(t, configContentEqual(before, after), "a conflict must leave the kubeconfig unchanged")
		})
	}
}

// TestAuthInfosEqualIgnoringChdir covers the comparison used by error mode.
func TestAuthInfosEqualIgnoringChdir(t *testing.T) {
	withEnv := func(env ...clientcmdapi.ExecEnvVar) *clientcmdapi.AuthInfo {
		return &clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{Command: atmosCommand, Env: env}}
	}
	chdirA := clientcmdapi.ExecEnvVar{Name: "ATMOS_CHDIR", Value: "/a"}
	chdirB := clientcmdapi.ExecEnvVar{Name: "ATMOS_CHDIR", Value: "/b"}
	identity := clientcmdapi.ExecEnvVar{Name: "ATMOS_IDENTITY", Value: "admin"}

	assert.True(t, authInfosEqualIgnoringChdir(withEnv(identity, chdirA), withEnv(identity, chdirB)))
	assert.True(t, authInfosEqualIgnoringChdir(withEnv(identity), withEnv(identity, chdirB)), "a missing ATMOS_CHDIR is ignored too")
	assert.False(t, authInfosEqualIgnoringChdir(withEnv(identity, chdirA), withEnv(chdirA)))
	assert.False(t, authInfosEqualIgnoringChdir(withEnv(identity), &clientcmdapi.AuthInfo{Token: "x", Exec: withEnv(identity).Exec}))
	assert.True(t, authInfosEqualIgnoringChdir(&clientcmdapi.AuthInfo{Token: "x"}, &clientcmdapi.AuthInfo{Token: "x"}), "no exec config")
	assert.True(t, authInfosEqualIgnoringChdir(nil, nil))
	assert.False(t, authInfosEqualIgnoringChdir(nil, withEnv()))
}

// TestWriteClusterConfig_ErrorMode_UnreadableFile verifies that an unparseable
// existing kubeconfig is reported by the merge as a load failure and is left
// untouched, rather than being treated as a collision or overwritten.
func TestWriteClusterConfig_ErrorMode_UnreadableFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubeconfig")
	garbage := []byte("clusters: [this is: not, valid")
	require.NoError(t, os.WriteFile(path, garbage, 0o600))
	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	changed, err := mgr.WriteClusterConfig(errorModeInfo(t.TempDir()), errorModeAlias, "error")
	require.ErrorIs(t, err, errUtils.ErrKubeconfigMerge)
	assert.False(t, changed)

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, garbage, after)
}
