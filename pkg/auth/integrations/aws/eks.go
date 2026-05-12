package aws

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/cloud/kube"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// describeClusterCache memoizes EKS DescribeCluster responses for the lifetime
// of the current process, keyed by (AWS account ID, cluster name, region).
//
// Why cache only DescribeCluster, not the whole Execute call:
//
// Auto-provisioned EKS integrations re-run on every identity resolution (each
// template lookup, each !terraform.output evaluation, each workflow step). A
// single `atmos workflow ...` invocation can authenticate identities targeting
// the same EKS cluster many times over. Each unique resolution otherwise costs
// an EKS DescribeCluster API round trip — that is the expensive part.
//
// Caching the whole Execute body is unsafe: WriteClusterConfig sets
// `current-context` on every write, callers depend on `current-context` being
// correct after Execute returns, and the kubeconfig file may be mutated
// between calls (manual `kubectl config use-context`, another integration
// re-pointing `current-context`, `auth logout` removing entries, file
// deletion). Skipping kubeconfig reconciliation in those scenarios would
// cause downstream tools to run against the wrong cluster or a missing file.
//
// Caching only DescribeCluster keeps the expensive part out of the hot path
// while letting WriteClusterConfig run every time. Combined with the no-op
// detection added in PR #2402, the kubeconfig path is fast and idempotent.
//
// Cluster metadata (endpoint, CA, ARN) is immutable in practice — CA
// rotation is a multi-year event and is stored in the kubeconfig anyway. A
// fresh `atmos` invocation always starts with an empty cache, so any rare
// staleness is recovered on the next command.
//
// Why the cache key is identity-independent but ACCOUNT-scoped:
//
// `eks:DescribeCluster` is a *setup-time* permission. At runtime, kubectl
// authenticates to the cluster via the exec credential plugin we install
// (`atmos aws eks token --identity=…`), which uses `sts:GetCallerIdentity`
// — not `eks:DescribeCluster` — to mint EKS bearer tokens. So the cluster
// metadata we cache (endpoint, CA, ARN) is genuinely identity-independent
// *within one AWS account*; the only thing identity controls is whether the
// *initial* DescribeCluster call succeeds.
//
// EKS cluster names are unique within an account+region but NOT across
// accounts. So the cache key must include the AWS account ID — otherwise
// identity A in account 111111111111 and identity B in account 222222222222,
// both with a cluster named "prod" in us-east-2, would share a cache entry
// and the second one would receive the wrong endpoint/CA/ARN. Within a
// single account, all identities legitimately share the same cluster
// metadata regardless of which IAM role describes it.
//
// Empirically this matters: in typical Atmos auth setups, every stack has its
// own identity with its own `aws/eks` integration pointing to a shared cluster.
// An identity-scoped cache key never collapses anything because each
// resolution is a unique (identity, cluster) tuple. A real-world workflow
// trace shows 16 Execute calls across 6 unique clusters → 0 cache hits with
// an identity-scoped key, but 10 hits + 6 misses with an account-scoped key.
//
// The pathological mixed-permission case — identity A has describe but
// identity B does not, both targeting the same cluster *in the same account* —
// would, with this cache, let B receive A's cached cluster metadata and
// write a kubeconfig. B's subsequent kubectl calls would then fail at
// STS-bearer-token time with a clear permission error. That is a slightly
// worse error message in a niche scenario, not a correctness or security bug.
var describeClusterCache sync.Map

// accountIDCache memoizes the result of resolving an AWS account ID for a
// given set of credentials, keyed by AccessKeyID. AccessKeyID is unique per
// credential session, so this is safe and stable. The resolution itself
// requires a one-off STS:GetCallerIdentity round trip; we trade that single
// cheap call for the ability to share DescribeCluster results across all
// identities that authenticate into the same AWS account.
var accountIDCache sync.Map

// accountIDResolver returns the AWS account ID for the given credentials.
// Overridable in tests so unit tests don't have to make real STS calls.
var accountIDResolver = defaultResolveAccountID

// defaultResolveAccountID resolves the AWS account ID for a set of credentials,
// caching by AccessKeyID. On cache miss it calls creds.Validate(ctx), which
// performs an STS:GetCallerIdentity round trip.
func defaultResolveAccountID(ctx context.Context, creds types.ICredentials) (string, error) {
	defer perf.Track(nil, "aws.defaultResolveAccountID")()

	awsCreds, isAWS := creds.(*types.AWSCredentials)
	if isAWS && awsCreds.AccessKeyID != "" {
		if cached, ok := accountIDCache.Load(awsCreds.AccessKeyID); ok {
			id, _ := cached.(string)
			return id, nil
		}
	}

	if creds == nil {
		return "", fmt.Errorf("%w: cannot resolve AWS account ID from nil credentials", errUtils.ErrEKSIntegrationFailed)
	}

	info, err := creds.Validate(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: failed to resolve AWS account ID: %w", errUtils.ErrEKSIntegrationFailed, err)
	}
	if info == nil || info.Account == "" {
		return "", fmt.Errorf("%w: AWS account ID not present in credential validation result", errUtils.ErrEKSIntegrationFailed)
	}

	if isAWS && awsCreds.AccessKeyID != "" {
		accountIDCache.Store(awsCreds.AccessKeyID, info.Account)
	}
	return info.Account, nil
}

// describeCacheKey is the lookup key for describeClusterCache.
type describeCacheKey struct {
	accountID   string
	clusterName string
	region      string
}

// describeClusterCached returns the EKS cluster metadata, using the
// process-local cache when available. The cache is keyed by AWS account ID,
// cluster name, and region — see describeClusterCache for the rationale.
// On a cache miss it resolves the account ID (one STS call per credential
// session), creates an EKS client via the package-level factory, and calls
// DescribeCluster. Only successful results are cached.
func describeClusterCached(ctx context.Context, creds types.ICredentials, clusterName, region string) (*awsCloud.EKSClusterInfo, error) {
	defer perf.Track(nil, "aws.describeClusterCached")()

	accountID, err := accountIDResolver(ctx, creds)
	if err != nil {
		return nil, err
	}

	key := describeCacheKey{accountID: accountID, clusterName: clusterName, region: region}
	if cached, ok := describeClusterCache.Load(key); ok {
		info, _ := cached.(*awsCloud.EKSClusterInfo)
		log.Debug("EKS DescribeCluster cache hit",
			"account", accountID, "cluster", clusterName, "region", region)
		return info, nil
	}

	client, err := eksClientFactory(ctx, creds, region)
	if err != nil {
		return nil, err
	}
	info, err := eksDescribeCluster(ctx, client, clusterName, region)
	if err != nil {
		return nil, err
	}
	describeClusterCache.Store(key, info)
	return info, nil
}

func init() {
	integrations.Register(integrations.KindAWSEKS, NewEKSIntegration)
}

// eksClientFactory creates an EKS client from credentials. Overridable in tests.
var eksClientFactory = func(ctx context.Context, creds types.ICredentials, region string) (awsCloud.EKSClient, error) {
	return awsCloud.NewEKSClient(ctx, creds, region)
}

// eksDescribeCluster describes an EKS cluster. Overridable in tests.
var eksDescribeCluster = awsCloud.DescribeCluster

// EKSIntegration implements the aws/eks integration type.
type EKSIntegration struct {
	name     string
	identity string
	cluster  *schema.EKSCluster
}

// NewEKSIntegration creates an EKS integration from config.
func NewEKSIntegration(config *integrations.IntegrationConfig) (integrations.Integration, error) {
	defer perf.Track(nil, "aws.NewEKSIntegration")()

	if config == nil || config.Config == nil {
		return nil, fmt.Errorf("%w: integration config is nil", errUtils.ErrIntegrationNotFound)
	}

	// Extract identity from via.identity.
	identity := ""
	if config.Config.Via != nil {
		identity = config.Config.Via.Identity
	}

	// Extract cluster from spec.cluster - required for aws/eks integrations.
	var cluster *schema.EKSCluster
	if config.Config.Spec != nil && config.Config.Spec.Cluster != nil {
		cluster = config.Config.Spec.Cluster
	}

	if cluster == nil {
		return nil, fmt.Errorf("%w: integration '%s' has no cluster configured (spec.cluster is required for aws/eks)", errUtils.ErrIntegrationFailed, config.Name)
	}

	if cluster.Name == "" {
		return nil, fmt.Errorf("%w: integration '%s' has no cluster name configured", errUtils.ErrIntegrationFailed, config.Name)
	}

	if cluster.Region == "" {
		return nil, fmt.Errorf("%w: integration '%s' has no region configured", errUtils.ErrIntegrationFailed, config.Name)
	}

	// Validate optional kubeconfig settings.
	if cluster.Kubeconfig != nil {
		if cluster.Kubeconfig.Mode != "" {
			if _, err := strconv.ParseUint(cluster.Kubeconfig.Mode, 8, 32); err != nil {
				return nil, fmt.Errorf("%w: integration '%s' has invalid kubeconfig mode %q", errUtils.ErrIntegrationFailed, config.Name, cluster.Kubeconfig.Mode)
			}
		}

		if cluster.Kubeconfig.Update != "" {
			switch cluster.Kubeconfig.Update {
			case "merge", "replace", "error":
				// Valid.
			default:
				return nil, fmt.Errorf("%w: integration '%s' has invalid kubeconfig update mode %q (must be merge, replace, or error)", errUtils.ErrIntegrationFailed, config.Name, cluster.Kubeconfig.Update)
			}
		}
	}

	return &EKSIntegration{
		name:     config.Name,
		identity: identity,
		cluster:  cluster,
	}, nil
}

// Kind returns "aws/eks".
func (e *EKSIntegration) Kind() string {
	return integrations.KindAWSEKS
}

// Execute performs EKS kubeconfig provisioning for the configured cluster.
//
// The DescribeCluster API call is memoized via describeClusterCache, but the
// kubeconfig write path runs every time so callers can rely on
// `current-context` and file presence being reasserted on every call. See
// the describeClusterCache doc comment for the safety argument.
func (e *EKSIntegration) Execute(ctx context.Context, creds types.ICredentials) error {
	defer perf.Track(nil, "aws.EKSIntegration.Execute")()

	log.Debug("Configuring kubeconfig for EKS cluster", "cluster", e.cluster.Name, "region", e.cluster.Region)

	// Describe cluster (cached) — endpoint, CA data, and ARN.
	info, err := describeClusterCached(ctx, creds, e.cluster.Name, e.cluster.Region)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrEKSIntegrationFailed, err)
	}

	// Resolve kubeconfig settings.
	kubeconfigPath, kubeconfigMode, updateMode := e.resolveKubeconfigSettings()

	// Create kubeconfig manager.
	mgr, err := kube.NewKubeconfigManager(kubeconfigPath, kubeconfigMode)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrEKSIntegrationFailed, err)
	}

	// Write cluster config.
	if err := mgr.WriteClusterConfig(info, e.cluster.Alias, e.identity, updateMode); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrEKSIntegrationFailed, err)
	}

	// Determine display name for success message.
	displayName := e.cluster.Alias
	if displayName == "" {
		displayName = info.ARN
	}

	ui.Success(fmt.Sprintf("EKS kubeconfig: %s → %s", displayName, mgr.GetPath()))
	log.Debug("EKS kubeconfig written", "cluster", e.cluster.Name, "context", displayName, "path", mgr.GetPath())

	return nil
}

// Cleanup removes kubeconfig entries for this integration's cluster.
func (e *EKSIntegration) Cleanup(_ context.Context) error {
	defer perf.Track(nil, "aws.EKSIntegration.Cleanup")()

	kubeconfigPath, kubeconfigMode, _ := e.resolveKubeconfigSettings()

	mgr, err := kube.NewKubeconfigManager(kubeconfigPath, kubeconfigMode)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrEKSIntegrationFailed, err)
	}

	// We need the ARN to remove the cluster entry. Without an API call, we search
	// the kubeconfig for an entry matching the cluster name suffix pattern.
	// This is best-effort since we don't have credentials during cleanup.
	clusterARN, err := e.findClusterARN(mgr)
	if err != nil {
		log.Debug("EKS cleanup: could not determine cluster ARN", "error", err)
		return nil
	}

	// Compute context name and user name to match BuildClusterConfig output.
	contextName := e.cluster.Alias
	if contextName == "" {
		// Use the ARN as context name (same default as BuildClusterConfig).
		contextName = clusterARN
	}

	userName := "atmos-eks-" + e.cluster.Name + "-" + e.cluster.Region

	if err := mgr.RemoveClusterConfig(clusterARN, contextName, userName); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrEKSIntegrationFailed, err)
	}

	log.Debug("EKS cleanup: removed kubeconfig entries", "cluster", e.cluster.Name, "context", contextName)

	return nil
}

// Environment returns environment variables contributed by this EKS integration.
func (e *EKSIntegration) Environment() (map[string]string, error) {
	defer perf.Track(nil, "aws.EKSIntegration.Environment")()

	kubeconfigPath, kubeconfigMode, _ := e.resolveKubeconfigSettings()

	mgr, err := kube.NewKubeconfigManager(kubeconfigPath, kubeconfigMode)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrEKSIntegrationFailed, err)
	}

	path := mgr.GetPath()

	return map[string]string{
		"KUBECONFIG":       path,
		"KUBE_CONFIG_PATH": path,
	}, nil
}

// GetIdentity returns the identity name this integration uses.
func (e *EKSIntegration) GetIdentity() string {
	return e.identity
}

// GetCluster returns the configured cluster.
func (e *EKSIntegration) GetCluster() *schema.EKSCluster {
	return e.cluster
}

// resolveKubeconfigSettings extracts kubeconfig path, mode, and update from cluster config.
func (e *EKSIntegration) resolveKubeconfigSettings() (path, mode, update string) {
	if e.cluster.Kubeconfig != nil {
		return e.cluster.Kubeconfig.Path, e.cluster.Kubeconfig.Mode, e.cluster.Kubeconfig.Update
	}
	return "", "", ""
}

// findClusterARN searches the kubeconfig for a cluster ARN matching this integration's cluster name.
func (e *EKSIntegration) findClusterARN(mgr *kube.KubeconfigManager) (string, error) {
	defer perf.Track(nil, "aws.EKSIntegration.findClusterARN")()

	clusters, err := mgr.ListClusterARNs()
	if err != nil {
		return "", err
	}

	// Look for an ARN ending with "cluster/<name>".
	suffix := "cluster/" + e.cluster.Name
	for _, arn := range clusters {
		if strings.HasSuffix(arn, suffix) {
			return arn, nil
		}
	}

	return "", fmt.Errorf("no cluster ARN found matching %s", e.cluster.Name)
}
