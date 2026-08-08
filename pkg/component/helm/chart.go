package helm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"helm.sh/helm/v4/pkg/action"
	helmchart "helm.sh/helm/v4/pkg/chart"
	"helm.sh/helm/v4/pkg/chart/loader"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/downloader"
	"helm.sh/helm/v4/pkg/getter"
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
	// DependencyUpdate fetches missing dependencies before loading the chart.
	DependencyUpdate bool
	// Repositories lists declarative chart repositories for "repo/name" refs.
	Repositories []chartRepository
	// Release is the presence-aware policy tree before an action is selected.
	Release releasePolicyInput
	// LifecycleFlags contains only explicitly supplied invocation overrides.
	LifecycleFlags map[string]any
	// Lifecycle is populated with the effective selected-operation policy.
	Lifecycle releaseLifecycleResolution
}

// newSettings builds Helm CLI environment settings honoring ambient HELM_* env.
var newSettings = cli.New

func newSettingsForNamespace(namespace string) *cli.EnvSettings {
	settings := newSettings()
	settings.SetNamespace(namespace)
	return settings
}

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
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		if errors.Is(err, errUtils.ErrHelmRenderFailed) {
			return "", err
		}
		return "", fmt.Errorf("%w: %w", errUtils.ErrHelmRenderFailed, err)
	}
	return manifest, nil
}

// newInstallAction constructs an Install action plus settings, wiring the chart
// path options (repo URL, version) and an OCI-capable registry client.
func newInstallAction(spec *chartSpec) (*action.Install, *cli.EnvSettings, error) {
	settings := newSettingsForNamespace(spec.Namespace)

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
	if err := ctx.Err(); err != nil {
		return "", err
	}
	chartRef := resolveChartRef(client, spec)

	chartPath, err := client.LocateChart(chartRef, settings)
	if err != nil {
		return "", fmt.Errorf("%w: failed to locate Helm chart %q: %w", errUtils.ErrHelmRenderFailed, spec.Chart, err)
	}

	loaded, err := loadChartForAction(ctx, chartPath, settings, client.GetRegistryClient(), spec.DependencyUpdate)
	if err != nil {
		return "", err
	}

	rel, err := client.RunWithContext(ctx, loaded, spec.Values)
	if err != nil {
		return "", err
	}

	rendered, ok := rel.(*release.Release)
	if !ok {
		return "", fmt.Errorf("%w: unexpected release type %T", errUtils.ErrHelmRenderFailed, rel)
	}
	return renderReleaseManifest(rendered), nil
}

// loadChart loads a chart and verifies that every declared dependency is present.
// The Helm action package assumes its CLI caller performed this check; without it,
// a missing library chart fails later with an unrelated template-definition error.
func loadChart(chartPath string) (helmchart.Charter, error) {
	return loadChartForAction(context.Background(), chartPath, nil, nil, false)
}

func loadChartForAction(
	ctx context.Context,
	chartPath string,
	settings *cli.EnvSettings,
	registryClient *registry.Client,
	dependencyUpdate bool,
) (helmchart.Charter, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	loaded, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to load Helm chart %q: %w", errUtils.ErrHelmRenderFailed, chartPath, err)
	}

	accessor, err := helmchart.NewAccessor(loaded)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to inspect Helm chart %q: %w", errUtils.ErrHelmRenderFailed, chartPath, err)
	}
	if dependencies := accessor.MetaDependencies(); len(dependencies) > 0 {
		if err := action.CheckDependencies(loaded, dependencies); err != nil {
			if !dependencyUpdate {
				return nil, fmt.Errorf("%w: an error occurred while checking Helm chart dependencies; run 'helm dependency build %s' or pass '--dependency-update' to fetch missing dependencies: %w", errUtils.ErrHelmRenderFailed, chartPath, err)
			}
			if settings == nil {
				return nil, fmt.Errorf("%w: cannot update dependencies for Helm chart %q without Helm environment settings", errUtils.ErrHelmRenderFailed, chartPath)
			}
			manager := &downloader.Manager{
				Out:              io.Discard,
				ChartPath:        chartPath,
				Getters:          getter.All(settings),
				RepositoryConfig: settings.RepositoryConfig,
				RepositoryCache:  settings.RepositoryCache,
				ContentCache:     settings.ContentCache,
				RegistryClient:   registryClient,
				Debug:            settings.Debug,
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if updateErr := manager.Update(); updateErr != nil {
				return nil, fmt.Errorf("%w: failed to update dependencies for Helm chart %q: %w", errUtils.ErrHelmRenderFailed, chartPath, updateErr)
			}
			loaded, err = loader.Load(chartPath)
			if err != nil {
				return nil, fmt.Errorf("%w: failed to reload Helm chart %q after dependency update: %w", errUtils.ErrHelmRenderFailed, chartPath, err)
			}
			if depErr := action.CheckDependencies(loaded, dependencies); depErr != nil {
				return nil, fmt.Errorf("%w: Helm chart %q is still missing dependencies after 'helm dependency update': %w", errUtils.ErrHelmRenderFailed, chartPath, depErr)
			}
		}
	}

	return loaded, nil
}

// renderReleaseManifest returns the same manifest surface as `helm template`:
// regular resources followed by hook resources with their source annotations.
// Helm keeps hooks outside Release.Manifest, so returning that field alone makes
// template, plan, and diff silently omit resources such as migration Jobs.
func renderReleaseManifest(rendered *release.Release) string {
	var manifests bytes.Buffer
	if manifest := strings.TrimSpace(rendered.Manifest); manifest != "" {
		fmt.Fprintln(&manifests, manifest)
	}
	for _, hook := range rendered.Hooks {
		if hook == nil || strings.TrimSpace(hook.Manifest) == "" {
			continue
		}
		fmt.Fprintf(&manifests, "---\n# Source: %s\n%s\n", hook.Path, strings.TrimSpace(hook.Manifest))
	}
	return manifests.String()
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
		if repo, found := findRepository(spec.Repositories, name); found {
			client.RepoURL = repo.URL
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
