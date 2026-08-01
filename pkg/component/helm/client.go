package helm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/loader"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/kube"
	"helm.sh/helm/v4/pkg/registry"
	release "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage/driver"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// actionContext bundles an initialized Helm action configuration and settings
// for cluster-side operations (apply/diff/delete).
type actionContext struct {
	cfg      *action.Configuration
	settings *cli.EnvSettings
}

type releaseActionResult struct {
	Manifest  string
	Operation string
	Lifecycle releaseLifecycleResolution
}

const (
	releaseOperationInstall = "install"
	releaseOperationUpgrade = "upgrade"
	releaseOperationDelete  = "delete"
)

// newActionContext initializes a cluster-capable Helm action configuration.
// The RESTClientGetter resolves credentials from the ambient KUBECONFIG, which
// the toolchain/auth environment configures before execution.
var newActionContext = func(namespace string) (*actionContext, error) {
	settings := newSettingsForNamespace(namespace)

	cfg := new(action.Configuration)
	if err := cfg.Init(settings.RESTClientGetter(), namespace, os.Getenv("HELM_DRIVER")); err != nil { //nolint:forbidigo
		return nil, fmt.Errorf("failed to initialize Helm configuration: %w", err)
	}

	registryClient, err := registry.NewClient(
		registry.ClientOptEnableCache(true),
		registry.ClientOptWriter(io.Discard),
		registry.ClientOptCredentialsFile(settings.RegistryConfig),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Helm registry client: %w", err)
	}
	cfg.RegistryClient = registryClient

	return &actionContext{cfg: cfg, settings: settings}, nil
}

// applyRelease installs the release if it does not exist, otherwise upgrades it
// (equivalent to `helm upgrade --install`). When dryRun is true the operation is
// validated server-side without persisting changes and the rendered manifest is
// returned for preview.
func applyRelease(ctx context.Context, spec *chartSpec, dryRun bool) (releaseActionResult, error) {
	defer perf.Track(nil, "helm.applyRelease")()
	if err := ctx.Err(); err != nil {
		return releaseActionResult{}, err
	}

	actx, err := newActionContext(spec.Namespace)
	if err != nil {
		return releaseActionResult{}, err
	}

	histClient := action.NewHistory(actx.cfg)
	histClient.Max = 1
	if _, historyErr := histClient.Run(spec.ReleaseName); errors.Is(historyErr, driver.ErrReleaseNotFound) {
		lifecycle, resolveErr := resolveReleaseLifecycleWithFlags(spec.Release, releaseOperationInstall, spec.LifecycleFlags)
		if resolveErr != nil {
			return releaseActionResult{Operation: releaseOperationInstall}, resolveErr
		}
		spec.Lifecycle = lifecycle
		manifest, installErr := installRelease(ctx, actx, spec, dryRun)
		return releaseActionResult{Manifest: manifest, Operation: releaseOperationInstall, Lifecycle: lifecycle}, installErr
	} else if historyErr != nil {
		return releaseActionResult{}, fmt.Errorf("%w %q: %w", errUtils.ErrHelmReleaseHistory, spec.ReleaseName, historyErr)
	}
	lifecycle, resolveErr := resolveReleaseLifecycleWithFlags(spec.Release, releaseOperationUpgrade, spec.LifecycleFlags)
	if resolveErr != nil {
		return releaseActionResult{Operation: releaseOperationUpgrade}, resolveErr
	}
	spec.Lifecycle = lifecycle
	manifest, upgradeErr := upgradeRelease(ctx, actx, spec, dryRun)
	return releaseActionResult{Manifest: manifest, Operation: releaseOperationUpgrade, Lifecycle: lifecycle}, upgradeErr
}

func installRelease(ctx context.Context, actx *actionContext, spec *chartSpec, dryRun bool) (string, error) {
	client := action.NewInstall(actx.cfg)
	client.SetRegistryClient(actx.cfg.RegistryClient)
	client.ReleaseName = spec.ReleaseName
	client.Namespace = spec.Namespace
	client.CreateNamespace = true
	client.Version = spec.Version
	configureInstallLifecycle(client, spec.Lifecycle.Policy)
	if dryRun {
		client.DryRunStrategy = action.DryRunServer
	}
	return runInstall(ctx, client, actx.settings, spec)
}

func upgradeRelease(ctx context.Context, actx *actionContext, spec *chartSpec, dryRun bool) (string, error) {
	client := action.NewUpgrade(actx.cfg)
	client.SetRegistryClient(actx.cfg.RegistryClient)
	client.Namespace = spec.Namespace
	client.Version = spec.Version
	configureUpgradeLifecycle(client, spec.Lifecycle.Policy)
	if dryRun {
		client.DryRunStrategy = action.DryRunServer
	}

	chartRef := resolveUpgradeChartRef(client, spec)
	chartPath, err := client.LocateChart(chartRef, actx.settings)
	if err != nil {
		return "", fmt.Errorf("failed to locate Helm chart %q: %w", spec.Chart, err)
	}
	loaded, err := loader.Load(chartPath)
	if err != nil {
		return "", fmt.Errorf("failed to load Helm chart %q: %w", chartPath, err)
	}

	rel, err := client.RunWithContext(ctx, spec.ReleaseName, loaded, spec.Values)
	if err != nil {
		return "", err
	}
	rendered, ok := rel.(*release.Release)
	if !ok {
		return "", fmt.Errorf("%w: unexpected release type %T", errUtils.ErrHelmRenderFailed, rel)
	}
	return rendered.Manifest, nil
}

// resolveUpgradeChartRef applies the same repo/name resolution as the install
// path but against an Upgrade action's ChartPathOptions.
func resolveUpgradeChartRef(client *action.Upgrade, spec *chartSpec) string {
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

// getDeployedManifest returns the manifest of the currently deployed release so
// it can serve as the baseline for a live diff. A release that does not exist
// yet yields an empty manifest (every object is reported as added) rather than an
// error. This is the only diff path that requires cluster access.
func getDeployedManifest(releaseName, namespace string) (string, error) {
	defer perf.Track(nil, "helm.getDeployedManifest")()

	actx, err := newActionContext(namespace)
	if err != nil {
		return "", err
	}

	rel, err := action.NewGet(actx.cfg).Run(releaseName)
	if err != nil {
		if errors.Is(err, driver.ErrReleaseNotFound) {
			return "", nil
		}
		return "", fmt.Errorf("failed to get deployed Helm release %q: %w", releaseName, err)
	}
	deployed, ok := rel.(*release.Release)
	if !ok {
		return "", fmt.Errorf("%w: unexpected release type %T", errUtils.ErrHelmRenderFailed, rel)
	}
	return deployed.Manifest, nil
}

// deleteRelease uninstalls the release.
func deleteRelease(ctx context.Context, spec *chartSpec, dryRun bool) error {
	defer perf.Track(nil, "helm.deleteRelease")()
	if err := ctx.Err(); err != nil {
		return err
	}

	actx, err := newActionContext(spec.Namespace)
	if err != nil {
		return err
	}

	client := action.NewUninstall(actx.cfg)
	configureUninstallLifecycle(client, spec.Lifecycle.Policy, dryRun)
	client.WaitOptions = []kube.WaitOption{
		kube.WithWaitContext(ctx),
		kube.WithWaitForDeleteMethodContext(ctx),
	}
	if _, err := client.Run(spec.ReleaseName); err != nil {
		if errors.Is(err, driver.ErrReleaseNotFound) {
			return nil
		}
		return fmt.Errorf("%w %q: %w", errUtils.ErrHelmReleaseUninstall, spec.ReleaseName, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func configureInstallLifecycle(client *action.Install, policy effectiveReleasePolicy) {
	client.RollbackOnFailure = policy.OnFailure == failurePolicyUninstall
	client.WaitStrategy = policy.WaitStrategy
	client.WaitForJobs = policy.WaitForJobs
	client.Timeout = policy.Timeout
	client.DisableHooks = !policy.ChartHooks
	client.SkipCRDs = policy.CRDs == crdPolicySkip
}

func configureUpgradeLifecycle(client *action.Upgrade, policy effectiveReleasePolicy) {
	client.RollbackOnFailure = policy.OnFailure == failurePolicyRollback
	client.WaitStrategy = policy.WaitStrategy
	client.WaitForJobs = policy.WaitForJobs
	client.Timeout = policy.Timeout
	client.CleanupOnFail = policy.CleanupOnFailure
	client.MaxHistory = policy.MaxHistory
	client.DisableHooks = !policy.ChartHooks
}

func configureUninstallLifecycle(client *action.Uninstall, policy effectiveReleasePolicy, dryRun bool) {
	client.WaitStrategy = policy.WaitStrategy
	client.Timeout = policy.Timeout
	client.DisableHooks = !policy.ChartHooks
	client.DryRun = dryRun
}
