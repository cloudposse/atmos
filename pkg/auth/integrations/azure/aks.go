package azure

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	"github.com/cloudposse/atmos/pkg/auth/cloud/kube"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

func init() {
	integrations.Register(integrations.KindAzureAKS, NewAKSIntegration)
}

// octalBase is the base used to parse a kubeconfig file-mode string (e.g. "0600").
const octalBase = 8

// uint32BitSize is the bit size used to parse a kubeconfig file-mode string
// into a uint32-representable value.
const uint32BitSize = 32

// logKeyCluster is the structured-log key name for the cluster name field.
const logKeyCluster = "cluster"

// aksClientFactory creates an AKS client from credentials. Overridable in tests.
var aksClientFactory = azureCloud.NewAKSClient

// aksDescribeCluster describes an AKS cluster. Overridable in tests.
var aksDescribeCluster = azureCloud.DescribeCluster

// AKSIntegration implements the azure/aks integration type.
type AKSIntegration struct {
	name     string
	identity string
	cluster  *schema.Cluster
}

// NewAKSIntegration creates an AKS integration from config.
func NewAKSIntegration(config *integrations.IntegrationConfig) (integrations.Integration, error) {
	defer perf.Track(nil, "azure.NewAKSIntegration")()

	if config == nil || config.Config == nil {
		return nil, fmt.Errorf("%w: integration config is nil", errUtils.ErrIntegrationNotFound)
	}

	// Extract identity from via.identity.
	identity := ""
	if config.Config.Via != nil {
		identity = config.Config.Via.Identity
	}

	// Extract cluster from spec.cluster - required for azure/aks integrations.
	var cluster *schema.Cluster
	if config.Config.Spec != nil && config.Config.Spec.Cluster != nil {
		cluster = config.Config.Spec.Cluster
	}

	if err := validateAKSCluster(cluster, config.Name); err != nil {
		return nil, err
	}

	return &AKSIntegration{
		name:     config.Name,
		identity: identity,
		cluster:  cluster,
	}, nil
}

// validateAKSCluster validates the required cluster fields and optional
// kubeconfig settings for an azure/aks integration, mirroring aws/eks's
// validation structure (see pkg/auth/integrations/aws/eks.go).
func validateAKSCluster(cluster *schema.Cluster, integrationName string) error {
	if cluster == nil {
		return fmt.Errorf("%w: integration '%s' has no cluster configured (spec.cluster is required for azure/aks)", errUtils.ErrIntegrationFailed, integrationName)
	}

	if cluster.Name == "" {
		return fmt.Errorf("%w: integration '%s' has no cluster name configured", errUtils.ErrIntegrationFailed, integrationName)
	}

	if cluster.ResourceGroup == "" {
		return fmt.Errorf("%w: integration '%s' has no resource_group configured", errUtils.ErrIntegrationFailed, integrationName)
	}

	return validateKubeconfigSettings(cluster, integrationName)
}

// validateKubeconfigSettings validates the optional kubeconfig mode and
// update-mode settings on a cluster. No-op when cluster.Kubeconfig is nil.
func validateKubeconfigSettings(cluster *schema.Cluster, integrationName string) error {
	if cluster.Kubeconfig == nil {
		return nil
	}

	if cluster.Kubeconfig.Mode != "" {
		if _, err := strconv.ParseUint(cluster.Kubeconfig.Mode, octalBase, uint32BitSize); err != nil {
			return fmt.Errorf("%w: integration '%s' has invalid kubeconfig mode %q", errUtils.ErrIntegrationFailed, integrationName, cluster.Kubeconfig.Mode)
		}
	}

	if cluster.Kubeconfig.Update != "" {
		switch cluster.Kubeconfig.Update {
		case "merge", "replace", "error":
			// Valid.
		default:
			return fmt.Errorf("%w: integration '%s' has invalid kubeconfig update mode %q (must be merge, replace, or error)", errUtils.ErrIntegrationFailed, integrationName, cluster.Kubeconfig.Update)
		}
	}

	return nil
}

// Kind returns "azure/aks".
func (a *AKSIntegration) Kind() string {
	return integrations.KindAzureAKS
}

// Execute performs AKS kubeconfig provisioning for the configured cluster.
func (a *AKSIntegration) Execute(ctx context.Context, creds types.ICredentials) error {
	defer perf.Track(nil, "azure.AKSIntegration.Execute")()

	log.Debug("Configuring kubeconfig for AKS cluster", logKeyCluster, a.cluster.Name, "resource_group", a.cluster.ResourceGroup)

	subscriptionID := a.resolveSubscriptionID(creds)

	// Create AKS client.
	client, err := aksClientFactory(ctx, creds, subscriptionID)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrAKSIntegrationFailed, err)
	}

	// Describe cluster to get endpoint and certificate data.
	info, err := aksDescribeCluster(ctx, client, subscriptionID, a.cluster.ResourceGroup, a.cluster.Name)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrAKSIntegrationFailed, err)
	}

	// The login-time AKS-scoped token was requested against the well-known
	// AKSServerAppID (see pkg/auth/providers/azure). If this cluster expects
	// a different AAD server application, that token won't be accepted —
	// warn rather than fail silently, since we can't detect this any earlier.
	if info.ServerID != "" && info.ServerID != azureCloud.AKSServerAppID {
		log.Warn("AKS cluster expects a non-default AAD server application; the identity's AKS token may not be accepted",
			logKeyCluster, a.cluster.Name, "expected_server_id", info.ServerID, "default_server_id", azureCloud.AKSServerAppID)
	}

	// Resolve kubeconfig settings.
	kubeconfigPath, kubeconfigMode, updateMode := a.resolveKubeconfigSettings()

	// Create kubeconfig manager.
	mgr, err := kube.NewKubeconfigManager(kubeconfigPath, kubeconfigMode)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrAKSIntegrationFailed, err)
	}

	// Write cluster config.
	changed, err := mgr.WriteClusterConfig(azureCloud.BuildKubeClusterInfo(info, a.identity), a.cluster.Alias, updateMode)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrAKSIntegrationFailed, err)
	}

	// Determine display name for success/debug message.
	displayName := a.cluster.Alias
	if displayName == "" {
		displayName = info.ID
	}

	// Only surface the success line when something actually changed.
	// Auto-provisioned integrations re-run on every identity resolution, so
	// suppressing no-op writes keeps the output readable in long-running
	// commands (workflows, templates, !terraform.output lookups, ...).
	if changed {
		ui.Success(fmt.Sprintf("AKS kubeconfig: %s → %s", displayName, mgr.GetPath()))
		log.Debug("AKS kubeconfig written", logKeyCluster, a.cluster.Name, "context", displayName, "path", mgr.GetPath())
	} else {
		log.Debug("AKS kubeconfig already up to date", logKeyCluster, a.cluster.Name, "context", displayName, "path", mgr.GetPath())
	}

	return nil
}

// Cleanup removes kubeconfig entries for this integration's cluster.
func (a *AKSIntegration) Cleanup(_ context.Context) error {
	defer perf.Track(nil, "azure.AKSIntegration.Cleanup")()

	kubeconfigPath, kubeconfigMode, _ := a.resolveKubeconfigSettings()

	mgr, err := kube.NewKubeconfigManager(kubeconfigPath, kubeconfigMode)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrAKSIntegrationFailed, err)
	}

	// We need the ARM resource ID to remove the cluster entry. Without an API
	// call, we search the kubeconfig for an entry matching the cluster name
	// suffix pattern. This is best-effort since we don't have credentials
	// during cleanup.
	clusterID, err := a.findClusterID(mgr)
	if err != nil {
		log.Debug("AKS cleanup: could not determine cluster resource ID", "error", err)
		return nil
	}

	// Compute context name and user name to match BuildKubeClusterInfo output.
	contextName := a.cluster.Alias
	if contextName == "" {
		contextName = clusterID
	}

	userName := "atmos-aks-" + a.cluster.Name + "-" + a.cluster.ResourceGroup

	if err := mgr.RemoveClusterConfig(clusterID, contextName, userName); err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrAKSIntegrationFailed, err)
	}

	log.Debug("AKS cleanup: removed kubeconfig entries", logKeyCluster, a.cluster.Name, "context", contextName)

	return nil
}

// Environment returns environment variables contributed by this AKS integration.
func (a *AKSIntegration) Environment() (map[string]string, error) {
	defer perf.Track(nil, "azure.AKSIntegration.Environment")()

	kubeconfigPath, kubeconfigMode, _ := a.resolveKubeconfigSettings()

	mgr, err := kube.NewKubeconfigManager(kubeconfigPath, kubeconfigMode)
	if err != nil {
		return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrAKSIntegrationFailed, err)
	}

	path := mgr.GetPath()

	return map[string]string{
		"KUBECONFIG":       path,
		"KUBE_CONFIG_PATH": path,
	}, nil
}

// GetIdentity returns the identity name this integration uses.
func (a *AKSIntegration) GetIdentity() string {
	return a.identity
}

// GetCluster returns the configured cluster.
func (a *AKSIntegration) GetCluster() *schema.Cluster {
	return a.cluster
}

// resolveSubscriptionID returns the cluster's subscription_id override, or
// falls back to the authenticated identity's subscription.
func (a *AKSIntegration) resolveSubscriptionID(creds types.ICredentials) string {
	if a.cluster.SubscriptionID != "" {
		return a.cluster.SubscriptionID
	}
	if azureCreds, ok := creds.(*types.AzureCredentials); ok {
		return azureCreds.SubscriptionID
	}
	return ""
}

// resolveKubeconfigSettings extracts kubeconfig path, mode, and update from cluster config.
func (a *AKSIntegration) resolveKubeconfigSettings() (path, mode, update string) {
	if a.cluster.Kubeconfig != nil {
		return a.cluster.Kubeconfig.Path, a.cluster.Kubeconfig.Mode, a.cluster.Kubeconfig.Update
	}
	return "", "", ""
}

// findClusterID searches the kubeconfig for an ARM resource ID matching this integration's cluster name.
func (a *AKSIntegration) findClusterID(mgr *kube.KubeconfigManager) (string, error) {
	defer perf.Track(nil, "azure.AKSIntegration.findClusterID")()

	clusters, err := mgr.ListClusterIDs()
	if err != nil {
		return "", err
	}

	// Include the resource group so same-named clusters cannot collide.
	suffix := "/resourceGroups/" + a.cluster.ResourceGroup + "/providers/Microsoft.ContainerService/managedClusters/" + a.cluster.Name
	for _, id := range clusters {
		if strings.HasSuffix(strings.ToLower(id), strings.ToLower(suffix)) {
			return id, nil
		}
	}

	return "", fmt.Errorf("%w: no cluster resource ID found matching %s", errUtils.ErrAKSClusterNotFound, a.cluster.Name)
}
