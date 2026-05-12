package kube

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"

	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
)

// TestMultiCluster_SecondRewriteSameClusterNoOp simulates the user's reported scenario:
// kubeconfig has two atmos-managed clusters, we re-write the *second* one with identical inputs.
// current-context should remain on cluster B, and changed should be false.
func TestMultiCluster_SecondRewriteSameClusterNoOp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")
	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	infoA := &awsCloud.EKSClusterInfo{
		Name: "cluster-a", Endpoint: "https://a.eks.amazonaws.com",
		CertificateAuthorityData: base64.StdEncoding.EncodeToString([]byte(testCARawPEM)),
		ARN:                      "arn:aws:eks:us-east-2:111111111111:cluster/cluster-a", Region: "us-east-2",
	}
	infoB := &awsCloud.EKSClusterInfo{
		Name: "cluster-b", Endpoint: "https://b.eks.amazonaws.com",
		CertificateAuthorityData: base64.StdEncoding.EncodeToString([]byte(testCARawPEM)),
		ARN:                      "arn:aws:eks:us-west-2:222222222222:cluster/cluster-b", Region: "us-west-2",
	}

	_, err = mgr.WriteClusterConfig(infoA, "ctx-a", "admin", "merge")
	require.NoError(t, err)
	_, err = mgr.WriteClusterConfig(infoB, "ctx-b", "admin", "merge")
	require.NoError(t, err)

	loaded, _ := clientcmd.LoadFromFile(path)
	require.Contains(t, loaded.Clusters, infoA.ARN)
	require.Contains(t, loaded.Clusters, infoB.ARN)
	require.Equal(t, "ctx-b", loaded.CurrentContext)

	// Re-write B with identical inputs — should be a no-op.
	changed, err := mgr.WriteClusterConfig(infoB, "ctx-b", "admin", "merge")
	require.NoError(t, err)
	assert.False(t, changed, "re-writing cluster B with current-context already on B must be no-op")

	// Re-write A — current-context flips from B to A, must report changed=true.
	changed, err = mgr.WriteClusterConfig(infoA, "ctx-a", "admin", "merge")
	require.NoError(t, err)
	assert.True(t, changed, "re-writing A while current-context is B must flip context → changed=true")

	loaded, _ = clientcmd.LoadFromFile(path)
	assert.Equal(t, "ctx-a", loaded.CurrentContext)
	// Both clusters still present.
	assert.Contains(t, loaded.Clusters, infoA.ARN)
	assert.Contains(t, loaded.Clusters, infoB.ARN)
}

// TestMultiCluster_UnknownThirdPartyClusterPreserved: kubeconfig has an entry atmos
// didn't write (e.g., user added one manually). Re-running for our cluster must
// preserve the third-party entry and report changed=false.
func TestMultiCluster_UnknownThirdPartyClusterPreserved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")
	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)

	ours := testClusterInfo()
	_, err = mgr.WriteClusterConfig(ours, "ours", "admin", "merge")
	require.NoError(t, err)

	// User adds a third-party cluster manually (simulating `kubectl config set-cluster`).
	loaded, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	loaded.Clusters["arn:aws:eks:eu-west-1:999999999999:cluster/third-party"] = loaded.Clusters[ours.ARN]
	require.NoError(t, clientcmd.WriteToFile(*loaded, path))

	// Capture state.
	before, _ := os.ReadFile(path)

	// Re-run our integration — should be a no-op even though kubeconfig diverged.
	changed, err := mgr.WriteClusterConfig(ours, "ours", "admin", "merge")
	require.NoError(t, err)
	assert.False(t, changed)

	after, _ := os.ReadFile(path)
	assert.Equal(t, before, after, "third-party cluster entry must be preserved byte-for-byte")
}
