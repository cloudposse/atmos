package eks

import (
	"context"
	"fmt"

	"k8s.io/client-go/tools/clientcmd"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/cloud/kube"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// executeEKSUpdateKubeconfigViaIntegrationDryRun previews --integration with
// --dry-run. Executing the integration would write the kubeconfig, so its
// cluster, identity, alias and kubeconfig path are resolved here (with the CLI
// overrides applied) and run through the direct path as a dry run.
func executeEKSUpdateKubeconfigViaIntegrationDryRun(integrationName, kubeconfigPath, alias string) error {
	defer perf.Track(nil, "eks.executeEKSUpdateKubeconfigViaIntegrationDryRun")()

	atmosConfig, mgr, err := loadEKSAuthManager()
	if err != nil {
		return err
	}
	req, err := eksDryRunRequestForIntegration(atmosConfig.Auth.Integrations, integrationName, kubeconfigPath, alias)
	if err != nil {
		return err
	}
	return updateEKSKubeconfigDirect(context.Background(), atmosConfig, mgr, req)
}

// eksDryRunRequestForIntegration builds the dry-run request for the named
// aws/eks integration. Non-empty kubeconfigPath and alias override the
// integration's configured values, as they do when it is executed.
func eksDryRunRequestForIntegration(configured map[string]schema.Integration, name, kubeconfigPath, alias string) (*eksDirectRequest, error) {
	integration, ok := configured[name]
	switch {
	case !ok:
		return nil, fmt.Errorf(errUtils.ErrWrapWithNameFormat, errUtils.ErrIntegrationNotFound, name)
	case integration.Kind != integrations.KindAWSEKS:
		return nil, fmt.Errorf("%w: integration '%s' is %q, not %s", errUtils.ErrIntegrationFailed, name, integration.Kind, integrations.KindAWSEKS)
	case integration.Via == nil || integration.Via.Identity == "":
		return nil, fmt.Errorf("%w: integration '%s' has no identity configured", errUtils.ErrIntegrationFailed, name)
	case integration.Spec == nil || integration.Spec.Cluster == nil ||
		integration.Spec.Cluster.Name == "" || integration.Spec.Cluster.Region == "":
		return nil, fmt.Errorf("%w: integration '%s' needs spec.cluster.name and spec.cluster.region", errUtils.ErrIntegrationFailed, name)
	}

	cluster := integration.Spec.Cluster
	req := &eksDirectRequest{
		clusterName:  cluster.Name,
		region:       cluster.Region,
		alias:        cluster.Alias,
		identityName: integration.Via.Identity,
		dryRun:       true,
	}
	if cluster.Kubeconfig != nil {
		req.kubeconfigPath = cluster.Kubeconfig.Path
	}
	if kubeconfigPath != "" {
		req.kubeconfigPath = kubeconfigPath
	}
	if alias != "" {
		req.alias = alias
	}
	return req, nil
}

// printEKSKubeconfigDryRun prints the cluster, context and user entries a
// write would merge into the kubeconfig at path, and leaves the file alone.
// Only these entries are printed, not the merged file, so other users'
// credentials in the kubeconfig never reach the terminal.
func printEKSKubeconfigDryRun(info *kube.ClusterInfo, alias, contextName, path string) error {
	content, err := clientcmd.Write(*kube.BuildClusterConfig(info, alias))
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrEKSIntegrationFailed, err)
	}
	ui.Info(fmt.Sprintf("Dry run: would write context %s to %s; the kubeconfig was not changed", contextName, path))
	return data.Write(string(content))
}
