package kube

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
)

// testCARawPEM is the raw PEM data used in test fixtures.
const testCARawPEM = "-----BEGIN CERTIFICATE-----\ntest-ca-data\n-----END CERTIFICATE-----\n"

func testClusterInfo() *awsCloud.EKSClusterInfo {
	return &awsCloud.EKSClusterInfo{
		Name:                     "dev-cluster",
		Endpoint:                 "https://XXXX.gr7.us-east-2.eks.amazonaws.com",
		CertificateAuthorityData: base64.StdEncoding.EncodeToString([]byte(testCARawPEM)),
		ARN:                      "arn:aws:eks:us-east-2:123456789012:cluster/dev-cluster",
		Region:                   "us-east-2",
	}
}

func TestNewKubeconfigManager_CustomPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom", "kubeconfig")
	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)
	assert.Equal(t, path, mgr.GetPath())
	assert.Equal(t, os.FileMode(defaultFileMode), mgr.mode)
}

func TestNewKubeconfigManager_CustomMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubeconfig")
	mgr, err := NewKubeconfigManager(path, "0644")
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), mgr.mode)
}

func TestNewKubeconfigManager_InvalidMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubeconfig")
	_, err := NewKubeconfigManager(path, "abc")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrKubeconfigWrite)
}

func TestNewKubeconfigManager_EmptyModeDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubeconfig")
	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(defaultFileMode), mgr.mode)
}

func TestBuildClusterConfig_WithAlias(t *testing.T) {
	info := testClusterInfo()
	config := BuildClusterConfig(info, "dev-eks", "dev-admin")

	// Check current context.
	assert.Equal(t, "dev-eks", config.CurrentContext)

	// Check cluster entry.
	cluster, ok := config.Clusters[info.ARN]
	require.True(t, ok)
	assert.Equal(t, info.Endpoint, cluster.Server)
	// CA data should be base64-decoded from the EKS API response.
	assert.Equal(t, []byte(testCARawPEM), cluster.CertificateAuthorityData)

	// Check context entry.
	ctx, ok := config.Contexts["dev-eks"]
	require.True(t, ok)
	assert.Equal(t, info.ARN, ctx.Cluster)
	assert.Equal(t, "atmos-eks-dev-cluster-us-east-2", ctx.AuthInfo)

	// Check user entry.
	user, ok := config.AuthInfos["atmos-eks-dev-cluster-us-east-2"]
	require.True(t, ok)
	require.NotNil(t, user.Exec)
	assert.Equal(t, execAPIVersion, user.Exec.APIVersion)
	assert.Equal(t, atmosCommand, user.Exec.Command)
	assert.Contains(t, user.Exec.Args, "--cluster-name")
	assert.Contains(t, user.Exec.Args, "dev-cluster")
	assert.Contains(t, user.Exec.Args, "--identity=dev-admin")
	assert.Equal(t, clientcmdapi.NeverExecInteractiveMode, user.Exec.InteractiveMode)

	// Check env vars.
	require.Len(t, user.Exec.Env, 1)
	assert.Equal(t, "ATMOS_IDENTITY", user.Exec.Env[0].Name)
	assert.Equal(t, "dev-admin", user.Exec.Env[0].Value)
}

func TestBuildClusterConfig_WithoutAlias(t *testing.T) {
	info := testClusterInfo()
	config := BuildClusterConfig(info, "", "dev-admin")

	// Context name should default to ARN.
	assert.Equal(t, info.ARN, config.CurrentContext)
	_, ok := config.Contexts[info.ARN]
	require.True(t, ok)
}

func TestBuildClusterConfig_WithoutIdentity(t *testing.T) {
	info := testClusterInfo()
	config := BuildClusterConfig(info, "dev", "")

	user := config.AuthInfos["atmos-eks-dev-cluster-us-east-2"]
	require.NotNil(t, user.Exec)
	// Should not contain --identity flag.
	assert.NotContains(t, user.Exec.Args, "--identity")
}

func TestWriteClusterConfig_MergeNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")

	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	info := testClusterInfo()
	changed, err := mgr.WriteClusterConfig(info, "dev-eks", "dev-admin", "merge")
	require.NoError(t, err)
	assert.True(t, changed, "first write to a new file must report changed=true")

	// Verify file exists and is valid kubeconfig.
	loaded, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	assert.Equal(t, "dev-eks", loaded.CurrentContext)
	assert.Contains(t, loaded.Clusters, info.ARN)
	assert.Contains(t, loaded.Contexts, "dev-eks")
	assert.Contains(t, loaded.AuthInfos, "atmos-eks-dev-cluster-us-east-2")
}

// TestWriteClusterConfig_MergeUnchanged verifies that re-writing identical
// content reports changed=false. Auto-provisioned EKS integrations re-run on
// every identity resolution, so the integration layer relies on this to avoid
// flooding the user with redundant success messages.
func TestWriteClusterConfig_MergeUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")

	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	info := testClusterInfo()
	changed, err := mgr.WriteClusterConfig(info, "dev-eks", "dev-admin", "merge")
	require.NoError(t, err)
	require.True(t, changed)

	// Capture mtime to confirm the second call does not touch the file.
	statBefore, err := os.Stat(path)
	require.NoError(t, err)

	// Identical inputs → no on-disk change.
	changed, err = mgr.WriteClusterConfig(info, "dev-eks", "dev-admin", "merge")
	require.NoError(t, err)
	assert.False(t, changed, "writing identical content must report changed=false")

	statAfter, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, statBefore.ModTime(), statAfter.ModTime(), "file must not be rewritten when unchanged")
}

// TestWriteClusterConfig_ReplaceUnchanged covers the replace mode no-op path.
func TestWriteClusterConfig_ReplaceUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")

	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	info := testClusterInfo()
	changed, err := mgr.WriteClusterConfig(info, "dev-eks", "dev-admin", "replace")
	require.NoError(t, err)
	require.True(t, changed)

	changed, err = mgr.WriteClusterConfig(info, "dev-eks", "dev-admin", "replace")
	require.NoError(t, err)
	assert.False(t, changed, "replace with identical content must report changed=false")
}

// TestWriteClusterConfig_MergeUnchangedReconcilesMode verifies that when content
// is unchanged but the file mode drifted (e.g., user ran chmod 0644 manually),
// the no-op path still restores the configured mode and reports changed=true.
// Prior to the change-detection refactor, every WriteClusterConfig call called
// os.Chmod unconditionally, so a regression here would silently let the
// kubeconfig persist at a weaker permission than configured.
func TestWriteClusterConfig_MergeUnchangedReconcilesMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission tests are not reliable on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")

	mgr, err := NewKubeconfigManager(path, "0600")
	require.NoError(t, err)

	info := testClusterInfo()
	_, err = mgr.WriteClusterConfig(info, "dev-eks", "dev-admin", "merge")
	require.NoError(t, err)

	// Simulate an out-of-band chmod that weakens the file.
	require.NoError(t, os.Chmod(path, 0o644))

	changed, err := mgr.WriteClusterConfig(info, "dev-eks", "dev-admin", "merge")
	require.NoError(t, err)
	assert.True(t, changed, "mode drift on identical content must report changed=true")

	stat, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), stat.Mode().Perm(), "no-op path must restore configured mode")
}

// TestWriteClusterConfig_MergeChangedFields verifies that altering any visible
// field (endpoint, in this case) flips changed back to true even when the ARN
// key stays the same.
func TestWriteClusterConfig_MergeChangedFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")

	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	info := testClusterInfo()
	_, err = mgr.WriteClusterConfig(info, "dev-eks", "dev-admin", "merge")
	require.NoError(t, err)

	// Rotate the endpoint — same ARN, different cluster data.
	rotated := *info
	rotated.Endpoint = "https://ZZZZ.gr7.us-east-2.eks.amazonaws.com"

	changed, err := mgr.WriteClusterConfig(&rotated, "dev-eks", "dev-admin", "merge")
	require.NoError(t, err)
	assert.True(t, changed, "endpoint change must report changed=true")
}

func TestWriteClusterConfig_MergeExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")

	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	// Write first cluster.
	info1 := testClusterInfo()
	_, err = mgr.WriteClusterConfig(info1, "dev-eks", "dev-admin", "merge")
	require.NoError(t, err)

	// Write second cluster.
	info2 := &awsCloud.EKSClusterInfo{
		Name:                     "staging-cluster",
		Endpoint:                 "https://YYYY.gr7.us-east-1.eks.amazonaws.com",
		CertificateAuthorityData: base64.StdEncoding.EncodeToString([]byte(testCARawPEM)),
		ARN:                      "arn:aws:eks:us-east-1:123456789012:cluster/staging-cluster",
		Region:                   "us-east-1",
	}
	_, err = mgr.WriteClusterConfig(info2, "staging-eks", "dev-admin", "merge")
	require.NoError(t, err)

	// Both clusters should exist.
	loaded, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	assert.Contains(t, loaded.Clusters, info1.ARN)
	assert.Contains(t, loaded.Clusters, info2.ARN)
	assert.Contains(t, loaded.Contexts, "dev-eks")
	assert.Contains(t, loaded.Contexts, "staging-eks")
	// Current context should be the last written.
	assert.Equal(t, "staging-eks", loaded.CurrentContext)
}

func TestWriteClusterConfig_Replace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")

	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	// Write first cluster.
	info1 := testClusterInfo()
	_, err = mgr.WriteClusterConfig(info1, "dev-eks", "dev-admin", "merge")
	require.NoError(t, err)

	// Replace with second cluster.
	info2 := &awsCloud.EKSClusterInfo{
		Name:                     "staging-cluster",
		Endpoint:                 "https://YYYY.gr7.us-east-1.eks.amazonaws.com",
		CertificateAuthorityData: base64.StdEncoding.EncodeToString([]byte(testCARawPEM)),
		ARN:                      "arn:aws:eks:us-east-1:123456789012:cluster/staging-cluster",
		Region:                   "us-east-1",
	}
	_, err = mgr.WriteClusterConfig(info2, "staging-eks", "dev-admin", "replace")
	require.NoError(t, err)

	// Only second cluster should exist.
	loaded, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	assert.NotContains(t, loaded.Clusters, info1.ARN)
	assert.Contains(t, loaded.Clusters, info2.ARN)
}

func TestWriteClusterConfig_ErrorMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")

	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	info := testClusterInfo()

	// First write should succeed.
	_, err = mgr.WriteClusterConfig(info, "dev-eks", "dev-admin", "error")
	require.NoError(t, err)

	// Second write with same cluster should fail.
	_, err = mgr.WriteClusterConfig(info, "dev-eks", "dev-admin", "error")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrKubeconfigMerge)
}

func TestWriteClusterConfig_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission tests are not reliable on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")

	mgr, err := NewKubeconfigManager(path, "0600")
	require.NoError(t, err)

	info := testClusterInfo()
	_, err = mgr.WriteClusterConfig(info, "dev-eks", "dev-admin", "merge")
	require.NoError(t, err)

	stat, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), stat.Mode().Perm())
}

func TestRemoveClusterConfig_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")

	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	info := testClusterInfo()
	_, err = mgr.WriteClusterConfig(info, "dev-eks", "dev-admin", "merge")
	require.NoError(t, err)

	// Remove the cluster.
	err = mgr.RemoveClusterConfig(info.ARN, "dev-eks", "atmos-eks-dev-cluster-us-east-2")
	require.NoError(t, err)

	// File should be removed (was the only cluster).
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err))
}

func TestRemoveClusterConfig_PreservesOthers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")

	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	// Write two clusters.
	info1 := testClusterInfo()
	_, err = mgr.WriteClusterConfig(info1, "dev-eks", "dev-admin", "merge")
	require.NoError(t, err)

	info2 := &awsCloud.EKSClusterInfo{
		Name:                     "staging-cluster",
		Endpoint:                 "https://YYYY.gr7.us-east-1.eks.amazonaws.com",
		CertificateAuthorityData: base64.StdEncoding.EncodeToString([]byte(testCARawPEM)),
		ARN:                      "arn:aws:eks:us-east-1:123456789012:cluster/staging-cluster",
		Region:                   "us-east-1",
	}
	_, err = mgr.WriteClusterConfig(info2, "staging-eks", "dev-admin", "merge")
	require.NoError(t, err)

	// Remove first cluster.
	err = mgr.RemoveClusterConfig(info1.ARN, "dev-eks", "atmos-eks-dev-cluster-us-east-2")
	require.NoError(t, err)

	// Second cluster should still exist.
	loaded, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	assert.NotContains(t, loaded.Clusters, info1.ARN)
	assert.Contains(t, loaded.Clusters, info2.ARN)
}

func TestRemoveClusterConfig_Idempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")

	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	// Remove from nonexistent file should succeed.
	err = mgr.RemoveClusterConfig("arn:aws:eks:us-east-2:123456789012:cluster/missing", "missing", "user-missing")
	require.NoError(t, err)
}

func TestRemoveClusterConfig_ClearsCurrentContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")

	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	// Write two clusters.
	info1 := testClusterInfo()
	_, err = mgr.WriteClusterConfig(info1, "dev-eks", "dev-admin", "merge")
	require.NoError(t, err)

	info2 := &awsCloud.EKSClusterInfo{
		Name:                     "staging-cluster",
		Endpoint:                 "https://YYYY.gr7.us-east-1.eks.amazonaws.com",
		CertificateAuthorityData: base64.StdEncoding.EncodeToString([]byte(testCARawPEM)),
		ARN:                      "arn:aws:eks:us-east-1:123456789012:cluster/staging-cluster",
		Region:                   "us-east-1",
	}
	_, err = mgr.WriteClusterConfig(info2, "staging-eks", "dev-admin", "merge")
	require.NoError(t, err)

	// Current context should be staging-eks (last written).
	loaded, _ := clientcmd.LoadFromFile(path)
	assert.Equal(t, "staging-eks", loaded.CurrentContext)

	// Remove staging (current context).
	err = mgr.RemoveClusterConfig(info2.ARN, "staging-eks", "atmos-eks-staging-cluster-us-east-1")
	require.NoError(t, err)

	// Current context should be cleared.
	loaded, _ = clientcmd.LoadFromFile(path)
	assert.Equal(t, "", loaded.CurrentContext)
}

func TestWriteClusterConfig_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "kubeconfig")

	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	info := testClusterInfo()
	_, err = mgr.WriteClusterConfig(info, "dev-eks", "dev-admin", "merge")
	require.NoError(t, err)

	// File should exist.
	_, err = os.Stat(path)
	require.NoError(t, err)
}

func TestWriteClusterConfig_DefaultUpdateMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")

	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	info := testClusterInfo()

	// Empty update mode should default to merge.
	_, err = mgr.WriteClusterConfig(info, "dev-eks", "dev-admin", "")
	require.NoError(t, err)

	loaded, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	assert.Contains(t, loaded.Clusters, info.ARN)
}

func TestListClusterARNs_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")

	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	// Write two clusters.
	info1 := testClusterInfo()
	_, err = mgr.WriteClusterConfig(info1, "dev-eks", "dev-admin", "merge")
	require.NoError(t, err)

	info2 := &awsCloud.EKSClusterInfo{
		Name:                     "staging-cluster",
		Endpoint:                 "https://YYYY.gr7.us-east-1.eks.amazonaws.com",
		CertificateAuthorityData: base64.StdEncoding.EncodeToString([]byte(testCARawPEM)),
		ARN:                      "arn:aws:eks:us-east-1:123456789012:cluster/staging-cluster",
		Region:                   "us-east-1",
	}
	_, err = mgr.WriteClusterConfig(info2, "staging-eks", "dev-admin", "merge")
	require.NoError(t, err)

	arns, err := mgr.ListClusterARNs()
	require.NoError(t, err)
	assert.Len(t, arns, 2)
	assert.Contains(t, arns, info1.ARN)
	assert.Contains(t, arns, info2.ARN)
}

func TestListClusterARNs_NonexistentFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent", "kubeconfig")

	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	arns, err := mgr.ListClusterARNs()
	require.NoError(t, err)
	assert.Nil(t, arns)
}

func TestListClusterARNs_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")

	// Write empty kubeconfig.
	config := clientcmdapi.NewConfig()
	err := clientcmd.WriteToFile(*config, path)
	require.NoError(t, err)

	mgr, mgrErr := NewKubeconfigManager(path, "")
	require.NoError(t, mgrErr)

	arns, err := mgr.ListClusterARNs()
	require.NoError(t, err)
	assert.Empty(t, arns)
}

func TestDefaultKubeconfigPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	path, err := DefaultKubeconfigPath()
	require.NoError(t, err)
	assert.Contains(t, path, "kube")
	assert.Contains(t, path, "config")
}

func TestNewKubeconfigManager_DefaultPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	mgr, err := NewKubeconfigManager("", "")
	require.NoError(t, err)
	assert.Contains(t, mgr.GetPath(), "kube")
	assert.Contains(t, mgr.GetPath(), "config")
}

func TestWriteClusterConfig_InvalidUpdateMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")

	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	info := testClusterInfo()
	_, err = mgr.WriteClusterConfig(info, "dev-eks", "dev-admin", "invalid")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrKubeconfigMerge)
	assert.Contains(t, err.Error(), "invalid update mode")
}

func TestWriteClusterConfig_ErrorMode_ContextCollision(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")

	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	info := testClusterInfo()
	_, err = mgr.WriteClusterConfig(info, "dev-eks", "dev-admin", "merge")
	require.NoError(t, err)

	// Write a different cluster but with same alias (context name).
	info2 := &awsCloud.EKSClusterInfo{
		Name:                     "other-cluster",
		Endpoint:                 "https://OTHER.eks.amazonaws.com",
		CertificateAuthorityData: base64.StdEncoding.EncodeToString([]byte(testCARawPEM)),
		ARN:                      "arn:aws:eks:us-east-1:123456789012:cluster/other-cluster",
		Region:                   "us-east-1",
	}
	_, err = mgr.WriteClusterConfig(info2, "dev-eks", "other-admin", "error")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrKubeconfigMerge)
	assert.Contains(t, err.Error(), "context dev-eks already exists")
}

func TestWriteClusterConfig_ErrorMode_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")

	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	// Error mode on new file should succeed.
	info := testClusterInfo()
	_, err = mgr.WriteClusterConfig(info, "", "dev-admin", "error")
	require.NoError(t, err)

	loaded, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	assert.Contains(t, loaded.Clusters, info.ARN)
}

func TestBuildClusterConfig_RawPEMCertificate(t *testing.T) {
	// If certificate data is already raw PEM (not base64), it should be used as-is.
	info := &awsCloud.EKSClusterInfo{
		Name:                     "dev-cluster",
		Endpoint:                 "https://example.eks.amazonaws.com",
		CertificateAuthorityData: "not-valid-base64!@#$",
		ARN:                      "arn:aws:eks:us-east-2:123456789012:cluster/dev-cluster",
		Region:                   "us-east-2",
	}

	config := BuildClusterConfig(info, "dev", "admin")
	cluster := config.Clusters[info.ARN]
	assert.Equal(t, []byte("not-valid-base64!@#$"), cluster.CertificateAuthorityData)
}

// TestMergeWouldChange_StructuralComparison locks in the contract for the
// no-op detection helper. This test does NOT touch the filesystem and so
// is immune to platform-specific YAML serialization quirks (Windows line
// endings, clientcmd LocationOfOrigin paths populated during LoadFromFile,
// …) that previously caused TestWriteClusterConfig_MergeUnchanged and
// friends to fail on Windows.
func TestMergeWouldChange_StructuralComparison(t *testing.T) {
	info := testClusterInfo()
	base := BuildClusterConfig(info, "dev-eks", "dev-admin")

	t.Run("identical configs return false", func(t *testing.T) {
		other := BuildClusterConfig(info, "dev-eks", "dev-admin")
		assert.False(t, mergeWouldChange(base, other),
			"merging an identical config must be a no-op")
	})

	t.Run("different current-context returns true", func(t *testing.T) {
		other := BuildClusterConfig(info, "dev-eks", "dev-admin")
		other.CurrentContext = "different-context"
		assert.True(t, mergeWouldChange(base, other),
			"a different current-context is a meaningful change")
	})

	t.Run("empty current-context in newConfig is treated as no-op for that field", func(t *testing.T) {
		other := BuildClusterConfig(info, "dev-eks", "dev-admin")
		other.CurrentContext = ""
		// All other fields match, and an empty newConfig.CurrentContext doesn't
		// overwrite, so this must report no change.
		assert.False(t, mergeWouldChange(base, other),
			"empty CurrentContext in newConfig must not flag the merge as a change")
	})

	t.Run("missing cluster in existing returns true", func(t *testing.T) {
		existing := BuildClusterConfig(info, "dev-eks", "dev-admin")
		// Wipe the cluster the newConfig adds.
		delete(existing.Clusters, info.ARN)
		assert.True(t, mergeWouldChange(existing, base),
			"adding a cluster that isn't in existing must flag a change")
	})

	t.Run("different cluster endpoint returns true", func(t *testing.T) {
		other := BuildClusterConfig(info, "dev-eks", "dev-admin")
		other.Clusters[info.ARN].Server = "https://different.eks.amazonaws.com"
		assert.True(t, mergeWouldChange(base, other),
			"changed cluster Server must flag a change")
	})

	t.Run("different exec plugin identity returns true", func(t *testing.T) {
		other := BuildClusterConfig(info, "dev-eks", "different-admin")
		assert.True(t, mergeWouldChange(base, other),
			"different identity changes the exec args, must flag a change")
	})

	t.Run("loaded entry with LocationOfOrigin matches freshly built", func(t *testing.T) {
		// Simulate the post-load state: cluster has LocationOfOrigin set
		// (which clientcmd populates from the source file path). The fresh
		// BuildClusterConfig output has LocationOfOrigin empty. Without the
		// structural comparison, byte equality would diverge here.
		existing := BuildClusterConfig(info, "dev-eks", "dev-admin")
		existing.Clusters[info.ARN].LocationOfOrigin = "/some/path/kubeconfig"
		existing.Contexts["dev-eks"].LocationOfOrigin = "/some/path/kubeconfig"
		existing.AuthInfos["atmos-eks-dev-cluster-us-east-2"].LocationOfOrigin = "/some/path/kubeconfig"
		assert.False(t, mergeWouldChange(existing, base),
			"LocationOfOrigin is load-time metadata and must not affect no-op detection")
	})
}
