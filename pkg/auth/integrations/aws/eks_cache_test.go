package aws

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"

	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// mockCredentialsForResolveTest is a tiny ICredentials stand-in used to exercise
// defaultResolveAccountID without making real AWS STS calls.
type mockCredentialsForResolveTest struct {
	validate func(ctx context.Context) (*types.ValidationInfo, error)
}

func (m *mockCredentialsForResolveTest) IsExpired() bool                           { return false }
func (m *mockCredentialsForResolveTest) GetExpiration() (*time.Time, error)        { return nil, nil }
func (m *mockCredentialsForResolveTest) BuildWhoamiInfo(_ *types.WhoamiInfo)       {}
func (m *mockCredentialsForResolveTest) Validate(ctx context.Context) (*types.ValidationInfo, error) {
	return m.validate(ctx)
}

// resetDescribeClusterCache wipes the process-local DescribeCluster and
// account-ID caches so each test runs against a fresh state regardless of
// order.
func resetDescribeClusterCache(t *testing.T) {
	t.Helper()
	describeClusterCache.Clear()
	accountIDCache.Clear()
}

// installFixedAccountID overrides accountIDResolver to return a fixed account
// for the duration of the test. Avoids real STS round trips and lets tests
// drive cache-key behavior precisely.
func installFixedAccountID(t *testing.T, account string) {
	t.Helper()
	orig := accountIDResolver
	t.Cleanup(func() { accountIDResolver = orig })
	accountIDResolver = func(_ context.Context, _ types.ICredentials) (string, error) {
		return account, nil
	}
}

// installAccountIDFromCreds overrides accountIDResolver to read the account
// from the AWS credential's session token field (used as a test-only side
// channel to vary account per call without plumbing extra state).
func installAccountIDFromCreds(t *testing.T) {
	t.Helper()
	orig := accountIDResolver
	t.Cleanup(func() { accountIDResolver = orig })
	accountIDResolver = func(_ context.Context, creds types.ICredentials) (string, error) {
		awsCreds, _ := creds.(*types.AWSCredentials)
		if awsCreds == nil {
			return "", errors.New("expected *types.AWSCredentials in test")
		}
		return awsCreds.SessionToken, nil
	}
}

// installDescribeCounter swaps in stub EKS factory + DescribeCluster funcs that
// return a fixed cluster and increment the supplied counter on each call.
// Also installs a fixed account-ID resolver so tests don't need to make real
// STS calls. Original funcs are restored on test cleanup.
func installDescribeCounter(t *testing.T, callCount *atomic.Int32) string {
	t.Helper()
	const clusterARN = "arn:aws:eks:us-east-2:123456789012:cluster/dev-cluster"

	installFixedAccountID(t, "123456789012")

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
	installFixedAccountID(t, "123456789012")

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

// TestDescribeClusterCached_SameAccountDifferentIdentityReusesDescribe is the
// real-world payoff test. In typical Atmos auth setups every stack has its
// own identity with its own `aws/eks` integration pointing to a shared EKS
// cluster, but all those identities target the same AWS account. The cache
// is account-scoped (not identity-scoped) so these legitimately share one
// DescribeCluster API call.
//
// See describeClusterCache for the full rationale.
func TestDescribeClusterCached_SameAccountDifferentIdentityReusesDescribe(t *testing.T) {
	resetDescribeClusterCache(t)

	var calls atomic.Int32
	_ = installDescribeCounter(t, &calls)

	mkIntegration := func(identity string) *EKSIntegration {
		return &EKSIntegration{
			name:     "test-cache-identity-share",
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

	// Pass any credential — installDescribeCounter installs a fixed account
	// resolver, so all identities resolve to account "123456789012".
	creds := &types.AWSCredentials{AccessKeyID: "AKIA-A"}
	require.NoError(t, mkIntegration("admin-a").Execute(t.Context(), creds))
	require.Equal(t, int32(1), calls.Load(), "first Execute must call DescribeCluster")

	require.NoError(t, mkIntegration("admin-b").Execute(t.Context(), creds))
	assert.Equal(t, int32(1), calls.Load(), "second identity in the same account must reuse cached describe")

	require.NoError(t, mkIntegration("admin-c").Execute(t.Context(), creds))
	assert.Equal(t, int32(1), calls.Load(), "third identity in the same account must reuse cached describe")
}

// TestDescribeClusterCached_DifferentAccountIsDifferentKey is the regression
// test for the cross-account cache poisoning bug Codex caught in the second
// adversarial review round. Two identities targeting an EKS cluster with the
// same NAME and REGION but in DIFFERENT AWS accounts must NOT share a cache
// entry — they are physically different clusters with different
// endpoints/CA/ARN, and silently giving one identity another account's
// metadata would misroute kubeconfig.
func TestDescribeClusterCached_DifferentAccountIsDifferentKey(t *testing.T) {
	resetDescribeClusterCache(t)
	// Read the account from the credentials' SessionToken field so each
	// integration call resolves to a distinct account.
	installAccountIDFromCreds(t)

	origClientFactory := eksClientFactory
	origDescribe := eksDescribeCluster
	t.Cleanup(func() {
		eksClientFactory = origClientFactory
		eksDescribeCluster = origDescribe
	})

	var calls atomic.Int32
	eksClientFactory = func(_ context.Context, _ types.ICredentials, _ string) (awsCloud.EKSClient, error) {
		return nil, nil
	}
	// Cluster ARN echoes the account so we can assert on it.
	eksDescribeCluster = func(ctx context.Context, _ awsCloud.EKSClient, name, region string) (*awsCloud.EKSClusterInfo, error) {
		calls.Add(1)
		// The cluster ARN's account segment must match whichever identity
		// triggered the describe — accountIDResolver was already called by
		// describeClusterCached, but we can recover the value from the
		// context-bound credentials via the test stub.
		account := callerAccountFromContext(ctx)
		return &awsCloud.EKSClusterInfo{
			Name:                     name,
			Endpoint:                 "https://" + account + ".eks.amazonaws.com",
			CertificateAuthorityData: "dGVzdA==",
			ARN:                      "arn:aws:eks:" + region + ":" + account + ":cluster/" + name,
			Region:                   region,
		}, nil
	}

	mk := func(account string) *EKSIntegration {
		return &EKSIntegration{
			name:     "test-cache-account",
			identity: "admin-in-" + account,
			cluster: &schema.EKSCluster{
				Name:   "shared-cluster-name",
				Region: "us-east-2",
				Alias:  "ctx-" + account,
				Kubeconfig: &schema.KubeconfigSettings{
					Path:   filepath.Join(t.TempDir(), "kubeconfig"),
					Update: "merge",
				},
			},
		}
	}

	// SessionToken doubles as the account ID (installAccountIDFromCreds).
	credsA := &types.AWSCredentials{AccessKeyID: "AKIA-A", SessionToken: "111111111111"}
	credsB := &types.AWSCredentials{AccessKeyID: "AKIA-B", SessionToken: "222222222222"}

	// Provide credentials via context so the describe stub can read them back.
	// describeClusterCached itself doesn't pass creds further; we instead
	// confirm the cache-key separation by counting DescribeCluster calls.
	require.NoError(t, mk("111111111111").Execute(withCallerAccount(t.Context(), "111111111111"), credsA))
	require.NoError(t, mk("222222222222").Execute(withCallerAccount(t.Context(), "222222222222"), credsB))

	assert.Equal(t, int32(2), calls.Load(),
		"distinct AWS accounts with the same cluster name+region MUST trigger two DescribeCluster calls")

	// Re-running each account is a no-op.
	require.NoError(t, mk("111111111111").Execute(withCallerAccount(t.Context(), "111111111111"), credsA))
	require.NoError(t, mk("222222222222").Execute(withCallerAccount(t.Context(), "222222222222"), credsB))
	assert.Equal(t, int32(2), calls.Load(), "re-running each account must reuse its own cached describe")
}

// callerAccountKey is the context key carrying the test-injected account ID,
// only used to round-trip a value into the eksDescribeCluster stub.
type callerAccountKey struct{}

func withCallerAccount(ctx context.Context, account string) context.Context {
	return context.WithValue(ctx, callerAccountKey{}, account)
}

func callerAccountFromContext(ctx context.Context) string {
	v, _ := ctx.Value(callerAccountKey{}).(string)
	return v
}

// TestDescribeClusterCached_DifferentClusterIsDifferentKey verifies the
// inverse: cluster name is part of the key, so two integrations pointing at
// different clusters in the same region make two separate API calls.
func TestDescribeClusterCached_DifferentClusterIsDifferentKey(t *testing.T) {
	resetDescribeClusterCache(t)
	installFixedAccountID(t, "123456789012")

	origClientFactory := eksClientFactory
	origDescribe := eksDescribeCluster
	t.Cleanup(func() {
		eksClientFactory = origClientFactory
		eksDescribeCluster = origDescribe
	})

	var calls atomic.Int32
	eksClientFactory = func(_ context.Context, _ types.ICredentials, _ string) (awsCloud.EKSClient, error) {
		return nil, nil
	}
	eksDescribeCluster = func(_ context.Context, _ awsCloud.EKSClient, name, region string) (*awsCloud.EKSClusterInfo, error) {
		calls.Add(1)
		return &awsCloud.EKSClusterInfo{
			Name:                     name,
			Endpoint:                 "https://" + name + ".eks.amazonaws.com",
			CertificateAuthorityData: "dGVzdA==",
			ARN:                      "arn:aws:eks:" + region + ":123456789012:cluster/" + name,
			Region:                   region,
		}, nil
	}

	mk := func(cluster string) *EKSIntegration {
		return &EKSIntegration{
			name:     "test-cache-cluster",
			identity: "dev-admin",
			cluster: &schema.EKSCluster{
				Name:   cluster,
				Region: "us-east-2",
				Alias:  cluster + "-ctx",
				Kubeconfig: &schema.KubeconfigSettings{
					Path:   filepath.Join(t.TempDir(), "kubeconfig"),
					Update: "merge",
				},
			},
		}
	}

	require.NoError(t, mk("cluster-a").Execute(t.Context(), nil))
	require.NoError(t, mk("cluster-b").Execute(t.Context(), nil))
	assert.Equal(t, int32(2), calls.Load(), "distinct clusters must each call DescribeCluster")

	require.NoError(t, mk("cluster-a").Execute(t.Context(), nil))
	assert.Equal(t, int32(2), calls.Load(), "re-running cluster-a must reuse its cached describe")
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

// TestDefaultResolveAccountID_Success exercises the Validate success path
// with a non-AWS credentials stand-in so no real STS call is made. AccessKeyID
// caching does not apply for non-AWS credentials, so a subsequent call with
// a different mock returning a different account works independently.
func TestDefaultResolveAccountID_Success(t *testing.T) {
	accountIDCache.Clear()

	mock := &mockCredentialsForResolveTest{
		validate: func(_ context.Context) (*types.ValidationInfo, error) {
			return &types.ValidationInfo{Account: "999999999999"}, nil
		},
	}
	got, err := defaultResolveAccountID(t.Context(), mock)
	require.NoError(t, err)
	assert.Equal(t, "999999999999", got)
}

// TestDefaultResolveAccountID_ValidateError verifies that Validate failures
// propagate as wrapped errors (no caching of failure).
func TestDefaultResolveAccountID_ValidateError(t *testing.T) {
	accountIDCache.Clear()

	mock := &mockCredentialsForResolveTest{
		validate: func(_ context.Context) (*types.ValidationInfo, error) {
			return nil, errors.New("STS denied")
		},
	}
	_, err := defaultResolveAccountID(t.Context(), mock)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "STS denied")
}

// TestDefaultResolveAccountID_EmptyAccount verifies that a Validate response
// with a missing Account field is treated as an error rather than being
// silently cached as the empty string.
func TestDefaultResolveAccountID_EmptyAccount(t *testing.T) {
	accountIDCache.Clear()

	mock := &mockCredentialsForResolveTest{
		validate: func(_ context.Context) (*types.ValidationInfo, error) {
			return &types.ValidationInfo{Account: ""}, nil
		},
	}
	_, err := defaultResolveAccountID(t.Context(), mock)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "account ID not present")
}

// TestDefaultResolveAccountID_NilCredentials guards against future callers
// accidentally passing nil; nil must fail loudly rather than cache an empty
// account ID that would later collide across credentials.
func TestDefaultResolveAccountID_NilCredentials(t *testing.T) {
	accountIDCache.Clear()

	_, err := defaultResolveAccountID(t.Context(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil credentials")
}

// TestDefaultResolveAccountID_MemoizesByAccessKeyID verifies the per-session
// memoization: when the cache already contains an entry for an AccessKeyID,
// the function returns it without invoking Validate. This is the critical
// property that prevents one STS call per Execute invocation.
func TestDefaultResolveAccountID_MemoizesByAccessKeyID(t *testing.T) {
	accountIDCache.Clear()
	t.Cleanup(func() { accountIDCache.Clear() })

	// Pre-populate cache as if a previous call had already validated.
	accountIDCache.Store("AKIA-CACHED", "555555555555")

	creds := &types.AWSCredentials{AccessKeyID: "AKIA-CACHED"}
	got, err := defaultResolveAccountID(t.Context(), creds)
	require.NoError(t, err)
	assert.Equal(t, "555555555555", got, "must return cached account without calling Validate")
}
