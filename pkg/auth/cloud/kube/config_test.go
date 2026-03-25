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
	err = mgr.WriteClusterConfig(info, "dev-eks", "dev-admin", "merge")
	require.NoError(t, err)

	// Verify file exists and is valid kubeconfig.
	loaded, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	assert.Equal(t, "dev-eks", loaded.CurrentContext)
	assert.Contains(t, loaded.Clusters, info.ARN)
	assert.Contains(t, loaded.Contexts, "dev-eks")
	assert.Contains(t, loaded.AuthInfos, "atmos-eks-dev-cluster-us-east-2")
}

func TestWriteClusterConfig_MergeExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")

	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	// Write first cluster.
	info1 := testClusterInfo()
	err = mgr.WriteClusterConfig(info1, "dev-eks", "dev-admin", "merge")
	require.NoError(t, err)

	// Write second cluster.
	info2 := &awsCloud.EKSClusterInfo{
		Name:                     "staging-cluster",
		Endpoint:                 "https://YYYY.gr7.us-east-1.eks.amazonaws.com",
		CertificateAuthorityData: base64.StdEncoding.EncodeToString([]byte(testCARawPEM)),
		ARN:                      "arn:aws:eks:us-east-1:123456789012:cluster/staging-cluster",
		Region:                   "us-east-1",
	}
	err = mgr.WriteClusterConfig(info2, "staging-eks", "dev-admin", "merge")
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
	err = mgr.WriteClusterConfig(info1, "dev-eks", "dev-admin", "merge")
	require.NoError(t, err)

	// Replace with second cluster.
	info2 := &awsCloud.EKSClusterInfo{
		Name:                     "staging-cluster",
		Endpoint:                 "https://YYYY.gr7.us-east-1.eks.amazonaws.com",
		CertificateAuthorityData: base64.StdEncoding.EncodeToString([]byte(testCARawPEM)),
		ARN:                      "arn:aws:eks:us-east-1:123456789012:cluster/staging-cluster",
		Region:                   "us-east-1",
	}
	err = mgr.WriteClusterConfig(info2, "staging-eks", "dev-admin", "replace")
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
	err = mgr.WriteClusterConfig(info, "dev-eks", "dev-admin", "error")
	require.NoError(t, err)

	// Second write with same cluster should fail.
	err = mgr.WriteClusterConfig(info, "dev-eks", "dev-admin", "error")
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
	err = mgr.WriteClusterConfig(info, "dev-eks", "dev-admin", "merge")
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
	err = mgr.WriteClusterConfig(info, "dev-eks", "dev-admin", "merge")
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
	err = mgr.WriteClusterConfig(info1, "dev-eks", "dev-admin", "merge")
	require.NoError(t, err)

	info2 := &awsCloud.EKSClusterInfo{
		Name:                     "staging-cluster",
		Endpoint:                 "https://YYYY.gr7.us-east-1.eks.amazonaws.com",
		CertificateAuthorityData: base64.StdEncoding.EncodeToString([]byte(testCARawPEM)),
		ARN:                      "arn:aws:eks:us-east-1:123456789012:cluster/staging-cluster",
		Region:                   "us-east-1",
	}
	err = mgr.WriteClusterConfig(info2, "staging-eks", "dev-admin", "merge")
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
	err = mgr.WriteClusterConfig(info1, "dev-eks", "dev-admin", "merge")
	require.NoError(t, err)

	info2 := &awsCloud.EKSClusterInfo{
		Name:                     "staging-cluster",
		Endpoint:                 "https://YYYY.gr7.us-east-1.eks.amazonaws.com",
		CertificateAuthorityData: base64.StdEncoding.EncodeToString([]byte(testCARawPEM)),
		ARN:                      "arn:aws:eks:us-east-1:123456789012:cluster/staging-cluster",
		Region:                   "us-east-1",
	}
	err = mgr.WriteClusterConfig(info2, "staging-eks", "dev-admin", "merge")
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
	err = mgr.WriteClusterConfig(info, "dev-eks", "dev-admin", "merge")
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
	err = mgr.WriteClusterConfig(info, "dev-eks", "dev-admin", "")
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
	err = mgr.WriteClusterConfig(info1, "dev-eks", "dev-admin", "merge")
	require.NoError(t, err)

	info2 := &awsCloud.EKSClusterInfo{
		Name:                     "staging-cluster",
		Endpoint:                 "https://YYYY.gr7.us-east-1.eks.amazonaws.com",
		CertificateAuthorityData: base64.StdEncoding.EncodeToString([]byte(testCARawPEM)),
		ARN:                      "arn:aws:eks:us-east-1:123456789012:cluster/staging-cluster",
		Region:                   "us-east-1",
	}
	err = mgr.WriteClusterConfig(info2, "staging-eks", "dev-admin", "merge")
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
	err = mgr.WriteClusterConfig(info, "dev-eks", "dev-admin", "invalid")
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
	err = mgr.WriteClusterConfig(info, "dev-eks", "dev-admin", "merge")
	require.NoError(t, err)

	// Write a different cluster but with same alias (context name).
	info2 := &awsCloud.EKSClusterInfo{
		Name:                     "other-cluster",
		Endpoint:                 "https://OTHER.eks.amazonaws.com",
		CertificateAuthorityData: base64.StdEncoding.EncodeToString([]byte(testCARawPEM)),
		ARN:                      "arn:aws:eks:us-east-1:123456789012:cluster/other-cluster",
		Region:                   "us-east-1",
	}
	err = mgr.WriteClusterConfig(info2, "dev-eks", "other-admin", "error")
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
	err = mgr.WriteClusterConfig(info, "", "dev-admin", "error")
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
