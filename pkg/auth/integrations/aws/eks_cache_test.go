package aws

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"

	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// resetDescribeClusterCache wipes the process-local DescribeCluster cache so
// each test runs against a fresh state regardless of order.
func resetDescribeClusterCache(t *testing.T) {
	t.Helper()
	describeClusterCache.Clear()
}

// installDescribeCounter swaps in stub EKS factory + DescribeCluster funcs that
// return a fixed cluster and increment the supplied counter on each call.
// Original funcs are restored on test cleanup.
func installDescribeCounter(t *testing.T, callCount *atomic.Int32) string {
	t.Helper()
	const clusterARN = "arn:aws:eks:us-east-2:123456789012:cluster/dev-cluster"

	origClientFactory := eksClientFactory
	origDescribe := eksDescribeCluster
	t.Cleanup(func() {
		eksClientFactory = origClientFactory
		eksDescribeCluster = origDescribe
	})

	eksClientFactory = func(_ context.Context, _ types.ICredentials, _ string) (awsCloud.EKSClient, error) {
		return nil, nil
	}
	eksDescribeCluster = func(_ context.Context, _ awsCloud.EKSClient, _, _ string) (*awsCloud.EKSClusterInfo, error) {
		callCount.Add(1)
		return &awsCloud.EKSClusterInfo{
			Name:                     "dev-cluster",
			Endpoint:                 "https://example.eks.amazonaws.com",
			CertificateAuthorityData: "dGVzdA==",
			ARN:                      clusterARN,
			Region:                   "us-east-2",
		}, nil
	}
	return clusterARN
}

// TestEKSIntegration_Execute_DescribeClusterCached verifies the cache
// short-circuit: a second Execute call with the same (identity, cluster name,
// region) does NOT call DescribeCluster again, but the kubeconfig is still
// written (reconciliation is intentionally not skipped — see PR discussion).
func TestEKSIntegration_Execute_DescribeClusterCached(t *testing.T) {
	resetDescribeClusterCache(t)

	var calls atomic.Int32
	clusterARN := installDescribeCounter(t, &calls)

	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	integration := &EKSIntegration{
		name:     "test-cache-hit",
		identity: "dev-admin",
		cluster: &schema.EKSCluster{
			Name:   "dev-cluster",
			Region: "us-east-2",
			Alias:  "dev-eks",
			Kubeconfig: &schema.KubeconfigSettings{
				Path:   kubeconfigPath,
				Update: "merge",
			},
		},
	}

	require.NoError(t, integration.Execute(t.Context(), nil))
	require.Equal(t, int32(1), calls.Load(), "first Execute must call DescribeCluster")

	// Second call must skip the API but still produce a valid kubeconfig.
	require.NoError(t, integration.Execute(t.Context(), nil))
	assert.Equal(t, int32(1), calls.Load(), "second Execute must NOT call DescribeCluster")

	// Confirm kubeconfig was reconciled (still has our cluster + context).
	cfg, err := clientcmd.LoadFromFile(kubeconfigPath)
	require.NoError(t, err)
	assert.Contains(t, cfg.Clusters, clusterARN)
	assert.Equal(t, "dev-eks", cfg.CurrentContext)
}

// TestEKSIntegration_Execute_KubeconfigStillReconciledOnCacheHit is the
// regression test for the Codex-flagged correctness bug an earlier draft of
// this PR had: a cache hit must NOT skip kubeconfig reconciliation. Here we
// have the user steal `current-context` between calls, and the next Execute
// must restore it — even though DescribeCluster is cached.
func TestEKSIntegration_Execute_KubeconfigStillReconciledOnCacheHit(t *testing.T) {
	resetDescribeClusterCache(t)

	var calls atomic.Int32
	clusterARN := installDescribeCounter(t, &calls)

	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	integration := &EKSIntegration{
		name:     "test-reconcile",
		identity: "dev-admin",
		cluster: &schema.EKSCluster{
			Name:   "dev-cluster",
			Region: "us-east-2",
			Alias:  "dev-eks",
			Kubeconfig: &schema.KubeconfigSettings{
				Path:   kubeconfigPath,
				Update: "merge",
			},
		},
	}

	require.NoError(t, integration.Execute(t.Context(), nil))
	require.Equal(t, int32(1), calls.Load())

	// Simulate `kubectl config use-context other`. Atmos must reassert ours.
	cfg, err := clientcmd.LoadFromFile(kubeconfigPath)
	require.NoError(t, err)
	cfg.CurrentContext = "other-context-set-by-user"
	require.NoError(t, clientcmd.WriteToFile(*cfg, kubeconfigPath))

	require.NoError(t, integration.Execute(t.Context(), nil))
	assert.Equal(t, int32(1), calls.Load(), "DescribeCluster must remain cached")

	cfg, err = clientcmd.LoadFromFile(kubeconfigPath)
	require.NoError(t, err)
	assert.Equal(t, "dev-eks", cfg.CurrentContext, "Execute on cache hit must still reassert current-context")
	assert.Contains(t, cfg.Clusters, clusterARN)
}

// TestEKSIntegration_Execute_KubeconfigRestoredAfterFileDeletion verifies the
// other side of the reconciliation guarantee: if something deletes the
// kubeconfig file between Execute calls, a cache hit on DescribeCluster
// must still recreate the file.
func TestEKSIntegration_Execute_KubeconfigRestoredAfterFileDeletion(t *testing.T) {
	resetDescribeClusterCache(t)

	var calls atomic.Int32
	_ = installDescribeCounter(t, &calls)

	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	integration := &EKSIntegration{
		name:     "test-recreate",
		identity: "dev-admin",
		cluster: &schema.EKSCluster{
			Name:   "dev-cluster",
			Region: "us-east-2",
			Alias:  "dev-eks",
			Kubeconfig: &schema.KubeconfigSettings{
				Path:   kubeconfigPath,
				Update: "merge",
			},
		},
	}

	require.NoError(t, integration.Execute(t.Context(), nil))

	// User (or another tool) deletes the kubeconfig.
	require.NoError(t, os.Remove(kubeconfigPath))

	require.NoError(t, integration.Execute(t.Context(), nil))
	assert.Equal(t, int32(1), calls.Load(), "DescribeCluster must remain cached")

	_, err := clientcmd.LoadFromFile(kubeconfigPath)
	require.NoError(t, err, "Execute on cache hit must recreate the kubeconfig file")
}

// TestDescribeClusterCached_FailureNotCached verifies that a failed
// DescribeCluster call is NOT cached, so transient failures (network blip,
// expired credentials, IAM hiccup) are retried on the next call rather than
// silently swallowed forever.
func TestDescribeClusterCached_FailureNotCached(t *testing.T) {
	resetDescribeClusterCache(t)

	origClientFactory := eksClientFactory
	origDescribe := eksDescribeCluster
	t.Cleanup(func() {
		eksClientFactory = origClientFactory
		eksDescribeCluster = origDescribe
	})

	var describeCalls atomic.Int32
	var failNext atomic.Bool
	failNext.Store(true)

	eksClientFactory = func(_ context.Context, _ types.ICredentials, _ string) (awsCloud.EKSClient, error) {
		return nil, nil
	}
	eksDescribeCluster = func(_ context.Context, _ awsCloud.EKSClient, _, _ string) (*awsCloud.EKSClusterInfo, error) {
		describeCalls.Add(1)
		if failNext.Load() {
			return nil, errors.New("simulated transient EKS API failure")
		}
		return &awsCloud.EKSClusterInfo{
			Name:                     "dev-cluster",
			Endpoint:                 "https://example.eks.amazonaws.com",
			CertificateAuthorityData: "dGVzdA==",
			ARN:                      "arn:aws:eks:us-east-2:123456789012:cluster/dev-cluster",
			Region:                   "us-east-2",
		}, nil
	}

	integration := &EKSIntegration{
		name:     "test-cache-failure",
		identity: "dev-admin",
		cluster: &schema.EKSCluster{
			Name:   "dev-cluster",
			Region: "us-east-2",
			Alias:  "dev-eks",
			Kubeconfig: &schema.KubeconfigSettings{
				Path:   filepath.Join(t.TempDir(), "kubeconfig"),
				Update: "merge",
			},
		},
	}

	// Failure must not poison the cache.
	require.Error(t, integration.Execute(t.Context(), nil))
	require.Equal(t, int32(1), describeCalls.Load())

	// Flip to success: retry must actually re-run.
	failNext.Store(false)
	require.NoError(t, integration.Execute(t.Context(), nil))
	assert.Equal(t, int32(2), describeCalls.Load(), "retry after failure must call DescribeCluster")

	// Now success is cached; third Execute reuses it.
	require.NoError(t, integration.Execute(t.Context(), nil))
	assert.Equal(t, int32(2), describeCalls.Load(), "success must be cached")
}

// TestDescribeClusterCached_DifferentIdentityIsDifferentKey verifies that
// two identities targeting the same cluster get separate cache entries —
// IAM permission for eks:DescribeCluster can differ per identity, and a
// denial for one must not be masked by a cached success from another.
func TestDescribeClusterCached_DifferentIdentityIsDifferentKey(t *testing.T) {
	resetDescribeClusterCache(t)

	var calls atomic.Int32
	_ = installDescribeCounter(t, &calls)

	mkIntegration := func(identity string) *EKSIntegration {
		return &EKSIntegration{
			name:     "test-cache-identity",
			identity: identity,
			cluster: &schema.EKSCluster{
				Name:   "dev-cluster",
				Region: "us-east-2",
				Alias:  "dev-eks-" + identity,
				Kubeconfig: &schema.KubeconfigSettings{
					Path:   filepath.Join(t.TempDir(), "kubeconfig"),
					Update: "merge",
				},
			},
		}
	}

	require.NoError(t, mkIntegration("admin-a").Execute(t.Context(), nil))
	require.Equal(t, int32(1), calls.Load())

	require.NoError(t, mkIntegration("admin-b").Execute(t.Context(), nil))
	assert.Equal(t, int32(2), calls.Load(), "different identity must produce a fresh DescribeCluster call")

	require.NoError(t, mkIntegration("admin-a").Execute(t.Context(), nil))
	assert.Equal(t, int32(2), calls.Load(), "re-running an identity must reuse its cached describe")
}

// TestDescribeClusterCached_SameClusterDifferentPathReusesDescribe verifies
// the inverse: kubeconfig destination has no effect on the DescribeCluster
// cache key, so writing the same cluster to two different kubeconfig files
// only costs one API call.
func TestDescribeClusterCached_SameClusterDifferentPathReusesDescribe(t *testing.T) {
	resetDescribeClusterCache(t)

	var calls atomic.Int32
	_ = installDescribeCounter(t, &calls)

	mkIntegration := func(path string) *EKSIntegration {
		return &EKSIntegration{
			name:     "test-cache-path",
			identity: "dev-admin",
			cluster: &schema.EKSCluster{
				Name:   "dev-cluster",
				Region: "us-east-2",
				Alias:  "dev-eks",
				Kubeconfig: &schema.KubeconfigSettings{
					Path:   path,
					Update: "merge",
				},
			},
		}
	}

	pathA := filepath.Join(t.TempDir(), "kubeconfig-a")
	pathB := filepath.Join(t.TempDir(), "kubeconfig-b")

	require.NoError(t, mkIntegration(pathA).Execute(t.Context(), nil))
	require.NoError(t, mkIntegration(pathB).Execute(t.Context(), nil))
	assert.Equal(t, int32(1), calls.Load(), "same cluster+identity must reuse describe regardless of kubeconfig path")

	// Both files must be valid kubeconfigs.
	for _, p := range []string{pathA, pathB} {
		_, err := clientcmd.LoadFromFile(p)
		require.NoError(t, err, "both kubeconfig files must be written")
	}
}
