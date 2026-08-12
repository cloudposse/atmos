package gcp

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	gcpCloud "github.com/cloudposse/atmos/pkg/auth/cloud/gcp"
	"github.com/cloudposse/atmos/pkg/auth/cloud/kube"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	kubeconfigModeBase    = 8
	kubeconfigModeBitSize = 32
)

var (
	gkeClientFactory   = gcpCloud.NewGKEClient
	gkeDescribeCluster = gcpCloud.DescribeCluster
	gkeExpectedServers sync.Map
)

// init registers the GKE integration factory.
func init() {
	integrations.Register(integrations.KindGCPGKE, NewGKEIntegration)
}

// GKEIntegration implements the gcp/gke integration type.
type GKEIntegration struct {
	name     string
	identity string
	cluster  *schema.Cluster
}

// NewGKEIntegration creates a GKE integration from config.
func NewGKEIntegration(config *integrations.IntegrationConfig) (integrations.Integration, error) {
	defer perf.Track(nil, "gcp.NewGKEIntegration")()

	if config == nil || config.Config == nil {
		return nil, fmt.Errorf("%w: integration config is nil", errUtils.ErrIntegrationNotFound)
	}

	identity := ""
	if config.Config.Via != nil {
		identity = config.Config.Via.Identity
	}

	var cluster *schema.Cluster
	if config.Config.Spec != nil {
		cluster = config.Config.Spec.Cluster
	}
	if err := validateGKECluster(cluster, config.Name); err != nil {
		return nil, err
	}

	return &GKEIntegration{name: config.Name, identity: identity, cluster: cluster}, nil
}

// validateGKECluster validates the cluster and kubeconfig portions of an integration.
func validateGKECluster(cluster *schema.Cluster, integrationName string) error {
	if cluster == nil {
		return fmt.Errorf("%w: integration '%s' has no cluster configured (spec.cluster is required for gcp/gke)", errUtils.ErrIntegrationFailed, integrationName)
	}
	if err := validateRequiredGKEClusterFields(cluster, integrationName); err != nil {
		return err
	}
	return validateGKEKubeconfig(cluster.Kubeconfig, integrationName)
}

// validateRequiredGKEClusterFields verifies the GKE resource address is complete.
func validateRequiredGKEClusterFields(cluster *schema.Cluster, integrationName string) error {
	if cluster.Name == "" {
		return fmt.Errorf("%w: integration '%s' has no cluster name configured", errUtils.ErrIntegrationFailed, integrationName)
	}
	if cluster.ProjectID == "" {
		return fmt.Errorf("%w: integration '%s' has no project_id configured", errUtils.ErrIntegrationFailed, integrationName)
	}
	if cluster.Location == "" {
		return fmt.Errorf("%w: integration '%s' has no location configured", errUtils.ErrIntegrationFailed, integrationName)
	}
	return nil
}

// validateGKEKubeconfig verifies supported file modes and update behavior.
func validateGKEKubeconfig(kubeconfig *schema.KubeconfigSettings, integrationName string) error {
	if kubeconfig == nil {
		return nil
	}
	if kubeconfig.Mode != "" {
		if _, err := strconv.ParseUint(kubeconfig.Mode, kubeconfigModeBase, kubeconfigModeBitSize); err != nil {
			return fmt.Errorf("%w: integration '%s' has invalid kubeconfig mode %q", errUtils.ErrIntegrationFailed, integrationName, kubeconfig.Mode)
		}
	}
	if update := kubeconfig.Update; update != "" && update != "merge" && update != "replace" && update != "error" {
		return fmt.Errorf("%w: integration '%s' has invalid kubeconfig update mode %q (must be merge, replace, or error)", errUtils.ErrIntegrationFailed, integrationName, update)
	}
	return nil
}

// Kind returns gcp/gke.
func (g *GKEIntegration) Kind() string { return integrations.KindGCPGKE }

// Execute describes the GKE cluster and provisions its kubeconfig entry.
func (g *GKEIntegration) Execute(ctx context.Context, creds types.ICredentials) error {
	defer perf.Track(nil, "gcp.GKEIntegration.Execute")()

	path, mode, update := g.resolveKubeconfigSettings()
	mgr, err := kube.NewKubeconfigManager(path, mode)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrGKEIntegrationFailed, err)
	}
	expectedServerKey := g.expectedServerKey(mgr.GetPath())
	gkeExpectedServers.Delete(expectedServerKey)

	if _, ok := creds.(*types.GCPCredentials); !ok {
		return fmt.Errorf("%w: expected GCP credentials", errUtils.ErrGKEIntegrationFailed)
	}

	log.Debug("Configuring kubeconfig for GKE cluster", "cluster", g.cluster.Name, "project_id", g.cluster.ProjectID, "location", g.cluster.Location)
	client, err := gkeClientFactory(ctx, creds)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrGKEIntegrationFailed, err)
	}
	info, err := gkeDescribeCluster(ctx, client, g.cluster.ProjectID, g.cluster.Location, g.cluster.Name)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrGKEIntegrationFailed, err)
	}

	changed, err := mgr.WriteClusterConfig(gcpCloud.BuildKubeClusterInfo(info, g.identity), g.cluster.Alias, update)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrGKEIntegrationFailed, err)
	}
	gkeExpectedServers.Store(expectedServerKey, info.Endpoint)

	displayName := g.cluster.Alias
	if displayName == "" {
		displayName = info.ID
	}
	if changed {
		ui.Success(fmt.Sprintf("GKE kubeconfig: %s → %s", displayName, mgr.GetPath()))
		log.Debug("GKE kubeconfig written", "cluster", g.cluster.Name, "context", displayName, "path", mgr.GetPath())
	} else {
		log.Debug("GKE kubeconfig already up to date", "cluster", g.cluster.Name, "context", displayName, "path", mgr.GetPath())
	}
	return nil
}

// Cleanup removes this integration's kubeconfig entries.
func (g *GKEIntegration) Cleanup(_ context.Context) error {
	defer perf.Track(nil, "gcp.GKEIntegration.Cleanup")()

	path, mode, _ := g.resolveKubeconfigSettings()
	mgr, err := kube.NewKubeconfigManager(path, mode)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrGKEIntegrationFailed, err)
	}
	clusterID := gcpCloud.ClusterResourceName(g.cluster.ProjectID, g.cluster.Location, g.cluster.Name)
	contextName := g.cluster.Alias
	if contextName == "" {
		contextName = clusterID
	}
	userName := "atmos-gke-" + g.cluster.Name + "-" + g.cluster.ProjectID + "-" + g.cluster.Location
	if err := mgr.RemoveClusterConfig(clusterID, contextName, userName); err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrGKEIntegrationFailed, err)
	}
	gkeExpectedServers.Delete(g.expectedServerKey(mgr.GetPath()))
	return nil
}

// Environment returns the kubeconfig variables contributed by this integration.
func (g *GKEIntegration) Environment() (map[string]string, error) {
	defer perf.Track(nil, "gcp.GKEIntegration.Environment")()

	path, mode, _ := g.resolveKubeconfigSettings()
	mgr, err := kube.NewKubeconfigManager(path, mode)
	if err != nil {
		return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrGKEIntegrationFailed, err)
	}
	env := map[string]string{"KUBECONFIG": mgr.GetPath(), "KUBE_CONFIG_PATH": mgr.GetPath()}
	if server, ok := gkeExpectedServers.Load(g.expectedServerKey(mgr.GetPath())); ok {
		env[kube.ExpectedServerEnv] = server.(string)
	}
	return env, nil
}

// expectedServerKey scopes endpoint state to one kubeconfig and GKE resource.
func (g *GKEIntegration) expectedServerKey(path string) string {
	return path + "\x00" + gcpCloud.ClusterResourceName(g.cluster.ProjectID, g.cluster.Location, g.cluster.Name)
}

// resolveKubeconfigSettings returns configured output settings or their zero values.
func (g *GKEIntegration) resolveKubeconfigSettings() (path, mode, update string) {
	if g.cluster.Kubeconfig != nil {
		return g.cluster.Kubeconfig.Path, g.cluster.Kubeconfig.Mode, g.cluster.Kubeconfig.Update
	}
	return "", "", ""
}
