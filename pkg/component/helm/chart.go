package helm

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/loader"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/registry"
	release "helm.sh/helm/v4/pkg/release/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// chartSpec is the resolved input needed to render or deploy a Helm chart.
type chartSpec struct {
	// Chart is the chart reference: a local path, an OCI ref (oci://...),
	// a "repo/name" reference (resolved via Repositories), or a bare chart name
	// used with RepoURL.
	Chart string
	// RepoURL is an explicit chart repository URL (HTTP). Optional.
	RepoURL string
	// Version is the chart version constraint. Optional.
	Version string
	// ReleaseName is the Helm release name.
	ReleaseName string
	// Namespace is the target Kubernetes namespace.
	Namespace string
	// Values is the merged Helm values map.
	Values map[string]any
	// IncludeCRDs includes CRDs from the chart's crds/ directory in the output.
	IncludeCRDs bool
	// Repositories maps repo name -> URL for "repo/name" chart references.
	Repositories map[string]string
}

// newSettings builds Helm CLI environment settings honoring ambient HELM_* env.
var newSettings = cli.New

// renderManifest renders the chart to a multi-document manifest string without
// contacting a cluster (client-side dry run, equivalent to `helm template`).
func renderManifest(ctx context.Context, spec *chartSpec) (string, error) {
	defer perf.Track(nil, "helm.renderManifest")()

	client, settings, err := newInstallAction(spec)
	if err != nil {
		return "", err
	}
	client.DryRunStrategy = action.DryRunClient
	client.IncludeCRDs = spec.IncludeCRDs

	manifest, err := runInstall(ctx, client, settings, spec)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrHelmRenderFailed, err)
	}
	return manifest, nil
}

// newInstallAction constructs an Install action plus settings, wiring the chart
// path options (repo URL, version) and an OCI-capable registry client.
func newInstallAction(spec *chartSpec) (*action.Install, *cli.EnvSettings, error) {
	settings := newSettings()

	cfg := new(action.Configuration)
	if err := cfg.Init(settings.RESTClientGetter(), spec.Namespace, os.Getenv("HELM_DRIVER")); err != nil { //nolint:forbidigo
		return nil, nil, fmt.Errorf("failed to initialize Helm configuration: %w", err)
	}

	registryClient, err := registry.NewClient(
		registry.ClientOptEnableCache(true),
		registry.ClientOptWriter(io.Discard),
		registry.ClientOptCredentialsFile(settings.RegistryConfig),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Helm registry client: %w", err)
	}
	cfg.RegistryClient = registryClient

	client := action.NewInstall(cfg)
	client.SetRegistryClient(registryClient)
	client.ReleaseName = spec.ReleaseName
	client.Namespace = spec.Namespace
	client.Replace = true
	client.Version = spec.Version

	return client, settings, nil
}

// runInstall resolves the chart reference, loads the chart, runs the action, and
// returns the rendered manifest string.
func runInstall(ctx context.Context, client *action.Install, settings *cli.EnvSettings, spec *chartSpec) (string, error) {
	chartRef := resolveChartRef(client, spec)

	chartPath, err := client.LocateChart(chartRef, settings)
	if err != nil {
		return "", fmt.Errorf("failed to locate Helm chart %q: %w", spec.Chart, err)
	}

	loaded, err := loader.Load(chartPath)
	if err != nil {
		return "", fmt.Errorf("failed to load Helm chart %q: %w", chartPath, err)
	}

	rel, err := client.RunWithContext(ctx, loaded, spec.Values)
	if err != nil {
		return "", err
	}

	rendered, ok := rel.(*release.Release)
	if !ok {
		return "", fmt.Errorf("%w: unexpected release type %T", errUtils.ErrHelmRenderFailed, rel)
	}
	return rendered.Manifest, nil
}

// resolveChartRef maps a "repo/name" chart reference to an explicit RepoURL +
// bare chart name when a matching repository is configured. Local paths and OCI
// refs pass through unchanged.
func resolveChartRef(client *action.Install, spec *chartSpec) string {
	if spec.RepoURL != "" {
		client.RepoURL = spec.RepoURL
		return spec.Chart
	}

	if isLocalOrOCI(spec.Chart) {
		return spec.Chart
	}

	if name, chart, ok := cutRepoRef(spec.Chart); ok {
		if url, found := spec.Repositories[name]; found {
			client.RepoURL = url
			return chart
		}
	}
	return spec.Chart
}

// cutRepoRef splits a "repo/name" chart reference into its repo and chart parts.
func cutRepoRef(chart string) (string, string, bool) {
	return strings.Cut(chart, "/")
}

// isLocalOrOCI reports whether the chart reference is a local path or OCI ref.
func isLocalOrOCI(chart string) bool {
	if strings.HasPrefix(chart, registry.OCIScheme) {
		return true
	}
	if strings.HasPrefix(chart, ".") || strings.HasPrefix(chart, "/") {
		return true
	}
	if _, err := os.Stat(chart); err == nil {
		return true
	}
	return false
}
