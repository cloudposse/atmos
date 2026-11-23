package aws

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"

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

// logKeyCluster is the structured log key for the EKS cluster name.
const logKeyCluster = "cluster"

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
	// realm is the explicitly configured auth realm (see kube.NameRealm); a
	// non-empty realm scopes the kubeconfig user and default context names.
	realm string
	// legacyRealms are derived realms whose names unreleased builds wrote;
	// they count as this integration's own (see kube.ClusterInfo.LegacyRealms).
	legacyRealms []string
	cluster      *schema.Cluster
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
	var cluster *schema.Cluster
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

	nameRealm, legacyRealms := kube.NameRealm(config.Realm, config.RealmExplicit())
	return &EKSIntegration{
		name:         config.Name,
		identity:     identity,
		realm:        nameRealm,
		legacyRealms: legacyRealms,
		cluster:      cluster,
	}, nil
}

// Kind returns "aws/eks".
func (e *EKSIntegration) Kind() string {
	return integrations.KindAWSEKS
}

// Execute performs EKS kubeconfig provisioning for the configured cluster.
func (e *EKSIntegration) Execute(ctx context.Context, creds types.ICredentials) error {
	defer perf.Track(nil, "aws.EKSIntegration.Execute")()

	log.Debug("Configuring kubeconfig for EKS cluster", logKeyCluster, e.cluster.Name, "region", e.cluster.Region)

	// Create EKS client.
	client, err := eksClientFactory(ctx, creds, e.cluster.Region)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrEKSIntegrationFailed, err)
	}

	// Describe cluster to get endpoint and certificate data.
	info, err := eksDescribeCluster(ctx, client, e.cluster.Name, e.cluster.Region)
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
	clusterInfo, err := awsCloud.BuildKubeClusterInfo(info, e.identity)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrEKSIntegrationFailed, err)
	}
	clusterInfo.Realm = e.realm
	clusterInfo.LegacyRealms = e.legacyRealms

	changed, err := mgr.WriteClusterConfig(clusterInfo, e.cluster.Alias, updateMode)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrEKSIntegrationFailed, err)
	}

	// The display name for success/debug messages is the context name written.
	displayName := kube.ContextName(info.ARN, e.cluster.Alias, e.realm)

	// Only surface the success line when something actually changed.
	// Auto-provisioned integrations re-run on every identity resolution, so
	// suppressing no-op writes keeps the output readable in long-running
	// commands (workflows, templates, !terraform.output lookups, ...).
	if changed {
		ui.Success(fmt.Sprintf("EKS kubeconfig: %s → %s", displayName, mgr.GetPath()))
		log.Debug("EKS kubeconfig written", logKeyCluster, e.cluster.Name, "context", displayName, "path", mgr.GetPath())
	} else {
		log.Debug("EKS kubeconfig already up to date", logKeyCluster, e.cluster.Name, "context", displayName, "path", mgr.GetPath())
	}

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

	// Compute context names and user names to match BuildClusterConfig output.
	// The default context name is [<realm>/]<ARN>; with a realm, the
	// unprefixed ARN that older versions wrote is checked too, and without one,
	// the <derived-realm>/<ARN> name unreleased builds wrote.
	contextNames := kube.ContextNames(clusterARN, e.cluster.Alias, e.realm, e.legacyRealms...)

	// The user name includes the realm, account ID and identity (see
	// awsCloud.BuildKubeClusterInfo). Also remove the names older versions
	// wrote: without the realm, and the atmos-eks-<name>-<region> legacy name.
	// A context is removed only when it points at one of these users, and a
	// user is kept while another context still references it.
	userNames := e.kubeUserNames(clusterARN)

	if err := mgr.RemoveOwnedClusterConfig(clusterARN, contextNames, userNames, kube.WithIdentity(e.identity)); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrEKSIntegrationFailed, err)
	}

	log.Debug("EKS cleanup: removed kubeconfig entries", logKeyCluster, e.cluster.Name, "context", contextNames[0])

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
func (e *EKSIntegration) GetCluster() *schema.Cluster {
	return e.cluster
}

// resolveKubeconfigSettings extracts kubeconfig path, mode, and update from cluster config.
func (e *EKSIntegration) resolveKubeconfigSettings() (path, mode, update string) {
	if e.cluster.Kubeconfig != nil {
		return e.cluster.Kubeconfig.Path, e.cluster.Kubeconfig.Mode, e.cluster.Kubeconfig.Update
	}
	return "", "", ""
}

// findClusterARN searches the kubeconfig for the EKS cluster ARN of this
// integration's cluster. Each key is parsed as an ARN and must name the EKS
// service, the integration's region and the "cluster/<name>" resource, so a
// same-named cluster in another region (or a non-EKS entry) is never matched.
// When same-named clusters of several accounts match, the one whose context
// uses this integration's exec user is chosen.
func (e *EKSIntegration) findClusterARN(mgr *kube.KubeconfigManager) (string, error) {
	defer perf.Track(nil, "aws.EKSIntegration.findClusterARN")()

	clusters, err := mgr.ListClusterIDs()
	if err != nil {
		return "", err
	}

	// Sort so the result does not depend on map iteration order.
	sort.Strings(clusters)
	var candidates []string
	for _, id := range clusters {
		if e.matchesClusterARN(id) {
			candidates = append(candidates, id)
		}
	}

	switch len(candidates) {
	case 0:
		return "", fmt.Errorf("%w: no cluster ARN found matching %s in %s", errUtils.ErrEKSClusterNotFound, e.cluster.Name, e.cluster.Region)
	case 1:
		return candidates[0], nil
	default:
		return e.selectOwnedClusterARN(mgr, candidates)
	}
}

// selectOwnedClusterARN picks, from same-named cluster ARNs of different
// accounts, the one whose context uses this integration's exec user. It fails
// rather than guessing when none or more than one qualifies.
func (e *EKSIntegration) selectOwnedClusterARN(mgr *kube.KubeconfigManager, candidates []string) (string, error) {
	var owned []string
	for _, clusterARN := range candidates {
		isOwned, err := e.ownsClusterContext(mgr, clusterARN)
		if err != nil {
			return "", err
		}
		if isOwned {
			owned = append(owned, clusterARN)
		}
	}

	if len(owned) == 1 {
		return owned[0], nil
	}
	return "", fmt.Errorf("%w: %d clusters named %s in %s match and none can be attributed to identity %q",
		errUtils.ErrEKSClusterAmbiguous, len(candidates), e.cluster.Name, e.cluster.Region, e.identity)
}

// ownsClusterContext reports whether one of the context names this integration
// writes for clusterARN (see kube.ContextNames) exists, uses that cluster and
// points at one of this integration's exec users.
func (e *EKSIntegration) ownsClusterContext(mgr *kube.KubeconfigManager, clusterARN string) (bool, error) {
	for _, contextName := range kube.ContextNames(clusterARN, e.cluster.Alias, e.realm, e.legacyRealms...) {
		kubeContext, err := mgr.LookupContext(contextName)
		if err != nil {
			return false, err
		}
		if kubeContext != nil && kubeContext.Cluster == clusterARN &&
			slices.Contains(e.kubeUserNames(clusterARN), kubeContext.AuthInfo) {
			return true, nil
		}
	}
	return false, nil
}

// kubeUserNames returns the exec user name this integration writes for
// clusterARN, followed by the names older versions wrote for it.
func (e *EKSIntegration) kubeUserNames(clusterARN string) []string {
	return awsCloud.EKSKubeUserNames(clusterARN, e.cluster.Name, e.cluster.Region, e.identity, kube.RealmScope{Realm: e.realm, LegacyRealms: e.legacyRealms})
}

// matchesClusterARN reports whether id is the ARN of this integration's EKS cluster.
func (e *EKSIntegration) matchesClusterARN(id string) bool {
	return awsCloud.IsEKSClusterARN(id, e.cluster.Name, e.cluster.Region)
}
