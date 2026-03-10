package aws

import (
	"context"
	"fmt"
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

func init() {
	integrations.Register(integrations.KindAWSEKS, NewEKSIntegration)
}

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
func (e *EKSIntegration) Execute(ctx context.Context, creds types.ICredentials) error {
	defer perf.Track(nil, "aws.EKSIntegration.Execute")()

	log.Debug("Configuring kubeconfig for EKS cluster", "cluster", e.cluster.Name, "region", e.cluster.Region)

	// Create EKS client.
	client, err := awsCloud.NewEKSClient(ctx, creds, e.cluster.Region)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrEKSIntegrationFailed, err)
	}

	// Describe cluster to get endpoint and certificate data.
	info, err := awsCloud.DescribeCluster(ctx, client, e.cluster.Name, e.cluster.Region)
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

	// Compute context name and user name as BuildClusterConfig would.
	contextName := e.cluster.Alias
	if contextName == "" {
		// Without the actual ARN, we construct it from the cluster name and region.
		// This matches the format: arn:aws:eks:<region>:<account>:cluster/<name>.
		// Since we don't have the account ID during cleanup, we use a wildcard approach:
		// search for any cluster entry that contains the cluster name.
		// However, the simpler approach is to require the alias for cleanup reliability.
		// For now, we'll skip removal if we can't determine the context name.
		log.Debug("EKS cleanup: no alias configured, skipping kubeconfig removal (ARN unknown without API call)")
		return nil
	}

	userName := "user-" + e.cluster.Name

	// We need the ARN to remove the cluster entry. Without an API call, we construct
	// a best-effort match. The cluster ARN key in kubeconfig is set by Execute().
	// During cleanup we don't have credentials, so we search the kubeconfig for
	// the cluster name pattern.
	clusterARN, err := e.findClusterARN(mgr)
	if err != nil {
		log.Debug("EKS cleanup: could not determine cluster ARN", "error", err)
		return nil
	}

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

	// Look for an ARN containing the cluster name.
	suffix := "cluster/" + e.cluster.Name
	for _, arn := range clusters {
		if len(arn) >= len(suffix) && arn[len(arn)-len(suffix):] == suffix {
			return arn, nil
		}
	}

	return "", fmt.Errorf("no cluster ARN found matching %s", e.cluster.Name)
}
