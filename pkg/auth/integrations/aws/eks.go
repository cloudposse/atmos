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
// of the current process, keyed by (identity, cluster name, region).
//
// Why cache only DescribeCluster, not the whole Execute call:
//
// Auto-provisioned EKS integrations re-run on every identity resolution (each
// template lookup, each !terraform.output evaluation, each workflow step). A
// single `atmos workflow ...` invocation can authenticate the same identity
// hundreds of times. Each unique resolution otherwise costs an EKS
// DescribeCluster API round trip — that is the expensive part.
//
// Caching the whole Execute body is unsafe: WriteClusterConfig sets
// `current-context` on every write, callers depend on `current-context` being
// correct after Execute returns, and the kubeconfig file may be mutated
// between calls (manual `kubectl config use-context`, another integration
// re-pointing `current-context`, `auth logout` removing entries, file
// deletion). Skipping kubeconfig reconciliation in those scenarios would
// cause downstream tools to run against the wrong cluster or a missing
// file.
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
// The cache key includes identity because eks:DescribeCluster IAM permission
// can differ between identities; a denial for one identity must not be
// masked by a success cached from another.
var describeClusterCache sync.Map

// describeCacheKey is the lookup key for describeClusterCache.
type describeCacheKey struct {
	identity    string
	clusterName string
	region      string
}

// describeClusterCached returns the EKS cluster metadata, using the
// process-local cache when available. On a cache miss it creates an EKS
// client via the package-level factory and calls DescribeCluster, caching
// only the successful result.
func describeClusterCached(ctx context.Context, creds types.ICredentials, identity, clusterName, region string) (*awsCloud.EKSClusterInfo, error) {
	defer perf.Track(nil, "aws.describeClusterCached")()

	key := describeCacheKey{identity: identity, clusterName: clusterName, region: region}
	if cached, ok := describeClusterCache.Load(key); ok {
		info, _ := cached.(*awsCloud.EKSClusterInfo)
		log.Debug("EKS DescribeCluster cache hit",
			"cluster", clusterName, "region", region, "identity", identity)
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
	info, err := describeClusterCached(ctx, creds, e.identity, e.cluster.Name, e.cluster.Region)
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
