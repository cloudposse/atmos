package helm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/kube"
	"helm.sh/helm/v4/pkg/registry"
	helmrelease "helm.sh/helm/v4/pkg/release"
	release "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage"
	"helm.sh/helm/v4/pkg/storage/driver"

	errUtils "github.com/cloudposse/atmos/errors"
	authkube "github.com/cloudposse/atmos/pkg/auth/cloud/kube"
	log "github.com/cloudposse/atmos/pkg/logger"
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
	if err := verifyExpectedKubernetesEndpoint(settings); err != nil {
		return nil, err
	}

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

// verifyExpectedKubernetesEndpoint enforces the opt-in GKE endpoint guard.
func verifyExpectedKubernetesEndpoint(settings *cli.EnvSettings) error {
	if os.Getenv(authkube.EndpointGuardEnv) != "true" { //nolint:forbidigo // Internal guard set only for opt-in protected operations.
		return nil
	}
	expected := os.Getenv(authkube.ExpectedServerEnv) //nolint:forbidigo // Set by the selected Atmos Auth cluster integration.
	if expected == "" {
		return nil
	}
	restConfig, err := settings.RESTClientGetter().ToRESTConfig()
	if err != nil {
		return fmt.Errorf("%w: resolve effective kubeconfig: %w", errUtils.ErrKubernetesClientInit, err)
	}
	actual := restConfig.Host
	if strings.TrimRight(actual, "/") != strings.TrimRight(expected, "/") {
		return fmt.Errorf("%w: expected %q, got %q", errUtils.ErrKubernetesEndpointMismatch, expected, actual)
	}
	return nil
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
		reportReleaseProgress(spec, releaseOperationInstall, lifecycle)
		reportResolvedLifecycle(lifecycle)
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
	reportReleaseProgress(spec, releaseOperationUpgrade, lifecycle)
	reportResolvedLifecycle(lifecycle)
	manifest, upgradeErr := upgradeRelease(ctx, actx, spec, dryRun)
	return releaseActionResult{Manifest: manifest, Operation: releaseOperationUpgrade, Lifecycle: lifecycle}, upgradeErr
}

// releaseOperationContext applies the effective lifecycle timeout to every
// cluster-side Helm action. A zero timeout intentionally leaves the outer
// action context unbounded during the migration; Helm then applies the
// selected wait strategy's own zero-timeout behavior.
func releaseOperationContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return parent, func() {}
	}
	return context.WithTimeout(parent, timeout)
}

// releaseWaitOptions builds the Helm wait options bound to the operation
// context. It is a package variable so tests can observe the context wired into
// the waiters (Helm's waitOptions.ctx is unexported and the fake waiter ignores
// it), mirroring the newActionContext seam above.
var releaseWaitOptions = func(ctx context.Context) []kube.WaitOption {
	return []kube.WaitOption{
		kube.WithWaitContext(ctx),
		kube.WithWaitForDeleteMethodContext(ctx),
	}
}

func installRelease(ctx context.Context, actx *actionContext, spec *chartSpec, dryRun bool) (string, error) {
	client := newInstallClient(actx, spec, dryRun)
	manifest, err := runInstall(ctx, client, actx.settings, spec)
	if err != nil {
		// Capture crash-looping pod diagnostics BEFORE the uninstall deletes the
		// failed first-install resources, then perform the uninstall Atmos now owns.
		diagnostics := ""
		if !dryRun {
			diagnostics = diagnoseReleaseFailure(ctx, actx, spec)
			if spec.Lifecycle.Policy.OnFailure == failurePolicyUninstall {
				err = uninstallFailedInstall(actx, spec, err)
			}
		}
		return "", releaseOperationErrorWithDiagnostics("install", spec, err, diagnostics)
	}
	return manifest, nil
}

// newInstallClient builds the Helm Install action for a release install, wiring
// the release name, namespace, namespace-creation policy, and chart version.
// CreateNamespace comes from the component config (default true); setting it to
// false installs into a pre-existing namespace and needs no cluster-level
// permission to create the namespace.
func newInstallClient(actx *actionContext, spec *chartSpec, dryRun bool) *action.Install {
	client := action.NewInstall(actx.cfg)
	client.SetRegistryClient(actx.cfg.RegistryClient)
	client.ReleaseName = spec.ReleaseName
	client.Namespace = spec.Namespace
	client.CreateNamespace = spec.CreateNamespace
	client.Version = spec.Version
	configureInstallLifecycle(client, spec.Lifecycle.Policy)
	if dryRun {
		client.DryRunStrategy = action.DryRunServer
	}
	return client
}

func upgradeRelease(ctx context.Context, actx *actionContext, spec *chartSpec, dryRun bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	client := action.NewUpgrade(actx.cfg)
	client.SetRegistryClient(actx.cfg.RegistryClient)
	client.Namespace = spec.Namespace
	client.Version = spec.Version
	configureUpgradeLifecycle(client, spec.Lifecycle.Policy)
	if dryRun {
		client.DryRunStrategy = action.DryRunServer
	}
	if err := ensureUpgradeForceConflictsCompatible(actx, spec); err != nil {
		return "", err
	}

	chartRef := resolveUpgradeChartRef(client, spec)
	chartPath, err := client.LocateChart(chartRef, actx.settings)
	if err != nil {
		return "", fmt.Errorf("%w: failed to locate Helm chart %q for upgrade: %w", errUtils.ErrHelmRenderFailed, spec.Chart, err)
	}
	loaded, err := loadChartForAction(ctx, chartPath, actx.settings, actx.cfg.RegistryClient, spec.DependencyUpdate)
	if err != nil {
		return "", err
	}

	operationCtx, cancel := releaseOperationContext(ctx, spec.Lifecycle.Policy.Timeout)
	defer cancel()
	client.WaitOptions = releaseWaitOptions(operationCtx)

	rel, err := client.RunWithContext(operationCtx, spec.ReleaseName, loaded, spec.Values)
	if err != nil {
		// Capture crash-looping pod diagnostics BEFORE the rollback replaces the
		// failing pods, then perform the rollback Atmos now owns.
		diagnostics := ""
		if !dryRun {
			diagnostics = diagnoseReleaseFailure(ctx, actx, spec)
			if spec.Lifecycle.Policy.OnFailure == failurePolicyRollback {
				err = rollbackFailedUpgrade(actx, spec, err)
			}
		}
		if ctxErr := operationCtx.Err(); ctxErr != nil {
			err = errors.Join(ctxErr, err)
		}
		return "", releaseOperationErrorWithDiagnostics("upgrade", spec, err, diagnostics)
	}
	rendered, ok := rel.(*release.Release)
	if !ok {
		return "", fmt.Errorf("%w: unexpected release type %T", errUtils.ErrHelmRenderFailed, rel)
	}
	return renderReleaseManifest(rendered), nil
}

// enforceReleaseHistoryLimit repairs Helm's rollback-on-failure path, which
// does not propagate Upgrade.MaxHistory to the internal Rollback action.
func enforceReleaseHistoryLimit(releases *storage.Storage, name string, maxHistory int) error {
	releases.MaxHistory = maxHistory
	if maxHistory <= 0 {
		return nil
	}

	history, err := releases.History(name)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		return nil
	}
	if err != nil || len(history) <= maxHistory {
		return err
	}

	revisions := make([]int, 0, len(history))
	for _, stored := range history {
		accessor, accessorErr := helmrelease.NewAccessor(stored)
		if accessorErr != nil {
			return accessorErr
		}
		revisions = append(revisions, accessor.Version())
	}
	sort.Ints(revisions)

	deployedVersion := -1
	deployed, err := releases.Deployed(name)
	if err != nil && !errors.Is(err, driver.ErrNoDeployedReleases) {
		return err
	}
	if err == nil {
		accessor, accessorErr := helmrelease.NewAccessor(deployed)
		if accessorErr != nil {
			return accessorErr
		}
		deployedVersion = accessor.Version()
	}

	remaining := len(revisions) - maxHistory
	var deleteErrs []error
	for _, version := range revisions {
		if remaining == 0 {
			break
		}
		if version == deployedVersion {
			continue
		}
		if _, deleteErr := releases.Delete(name, version); deleteErr != nil {
			deleteErrs = append(deleteErrs, deleteErr)
			continue
		}
		remaining--
	}
	return errors.Join(deleteErrs...)
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
	return renderReleaseManifest(deployed), nil
}

// deleteRelease uninstalls the release.
func deleteRelease(ctx context.Context, spec *chartSpec, dryRun bool) error {
	defer perf.Track(nil, "helm.deleteRelease")()
	if err := ctx.Err(); err != nil {
		return err
	}
	operationCtx, cancel := releaseOperationContext(ctx, spec.Lifecycle.Policy.Timeout)
	defer cancel()

	actx, err := newActionContext(spec.Namespace)
	if err != nil {
		return err
	}

	client := action.NewUninstall(actx.cfg)
	configureUninstallLifecycle(client, spec.Lifecycle.Policy, dryRun)
	client.WaitOptions = releaseWaitOptions(operationCtx)
	if _, err := client.Run(spec.ReleaseName); err != nil {
		if errors.Is(err, driver.ErrReleaseNotFound) {
			return nil
		}
		uninstallErr := releaseOperationError("delete", spec, err)
		if ctxErr := operationCtx.Err(); ctxErr != nil {
			return errors.Join(ctxErr, uninstallErr)
		}
		return uninstallErr
	}
	if err := operationCtx.Err(); err != nil {
		return err
	}
	return nil
}

func releaseOperationError(operation string, spec *chartSpec, cause error) error {
	return releaseOperationErrorWithDiagnostics(operation, spec, cause, "")
}

// releaseOperationErrorWithDiagnostics builds the #2849 failure error and, when
// diagnostics is non-empty, folds the crash-looping pod summary into the
// explanation so the cause is legible from the Atmos output alone
// (cloudposse/atmos#3271).
func releaseOperationErrorWithDiagnostics(operation string, spec *chartSpec, cause error, diagnostics string) error {
	policy := spec.Lifecycle.Policy
	builder := errUtils.Build(errUtils.ErrHelmReleaseOperation).
		WithCause(cause).
		WithContext("operation", operation).
		WithContext("release", spec.ReleaseName).
		WithContext("namespace", spec.Namespace).
		WithContext("wait_strategy", policy.WaitStrategy).
		WithContext("timeout", policy.Timeout).
		WithContext("timeout_field", spec.Lifecycle.TimeoutField)
	if diagnostics != "" {
		builder = builder.WithExplanation("workload diagnostics:\n" + diagnostics)
	}
	return builder.Err()
}

// releaseDiagnosticsTimeout bounds the best-effort pod-diagnostics collection so a
// slow or unreachable cluster never delays surfacing the original failure.
const releaseDiagnosticsTimeout = 30 * time.Second

// releaseErrWrapFormat wraps a sentinel, the release name, and a cause into one
// error ("sentinel "release": cause").
const releaseErrWrapFormat = "%w %q: %w"

// diagnoseReleaseFailure collects crash-looping pod diagnostics for a failed
// release operation before any rollback/uninstall deletes the evidence. The log
// tail and events are gated behind debug/trace so normal output is unchanged. It
// derives a short, independently-bounded context so a cluster that is slow to
// answer cannot stall the error, and so a readiness timeout on the operation
// context does not pre-cancel the queries.
func diagnoseReleaseFailure(ctx context.Context, actx *actionContext, spec *chartSpec) string {
	diagCtx, cancel := context.WithTimeout(ctx, releaseDiagnosticsTimeout)
	defer cancel()
	verbose := log.GetLevel() <= log.DebugLevel
	return collectReleaseFailureDiagnostics(diagCtx, actx, spec, verbose)
}

// rollbackFailedUpgrade performs the rollback Atmos owns (Helm's inline
// RollbackOnFailure is disabled so diagnostics can run first), joining any
// rollback or history-trim error onto the upgrade error. MaxHistory and
// CleanupOnFail mirror the configured policy; enforceReleaseHistoryLimit remains
// as the authoritative history trim.
func rollbackFailedUpgrade(actx *actionContext, spec *chartSpec, upgradeErr error) error {
	policy := spec.Lifecycle.Policy
	rollback := action.NewRollback(actx.cfg)
	rollback.MaxHistory = policy.MaxHistory
	rollback.CleanupOnFail = policy.CleanupOnFailure
	rollback.WaitStrategy = policy.WaitStrategy
	rollback.WaitForJobs = policy.WaitForJobs
	rollback.Timeout = policy.Timeout
	if rbErr := rollback.Run(spec.ReleaseName); rbErr != nil {
		upgradeErr = errors.Join(upgradeErr, fmt.Errorf(releaseErrWrapFormat, errUtils.ErrHelmReleaseRollback, spec.ReleaseName, rbErr))
	}
	if historyErr := enforceReleaseHistoryLimit(actx.cfg.Releases, spec.ReleaseName, policy.MaxHistory); historyErr != nil {
		upgradeErr = errors.Join(upgradeErr, fmt.Errorf(releaseErrWrapFormat, errUtils.ErrHelmReleaseHistory, spec.ReleaseName, historyErr))
	}
	return upgradeErr
}

// uninstallFailedInstall removes a failed first install (Atmos owns the
// uninstall-on-failure so diagnostics can run first), joining any uninstall error
// onto the install error. A release that is already gone is not an error.
func uninstallFailedInstall(actx *actionContext, spec *chartSpec, installErr error) error {
	policy := spec.Lifecycle.Policy
	uninstall := action.NewUninstall(actx.cfg)
	uninstall.WaitStrategy = policy.WaitStrategy
	uninstall.Timeout = policy.Timeout
	if _, rbErr := uninstall.Run(spec.ReleaseName); rbErr != nil && !errors.Is(rbErr, driver.ErrReleaseNotFound) {
		installErr = errors.Join(installErr, fmt.Errorf(releaseErrWrapFormat, errUtils.ErrHelmReleaseUninstall, spec.ReleaseName, rbErr))
	}
	return installErr
}

func configureInstallLifecycle(client *action.Install, policy effectiveReleasePolicy) {
	// Atmos owns the uninstall-on-failure so it can capture crash-looping pod
	// diagnostics before the failed resources are deleted (cloudposse/atmos#3271);
	// see installRelease. Helm's own RollbackOnFailure is therefore left off.
	client.RollbackOnFailure = false
	client.WaitStrategy = policy.WaitStrategy
	client.WaitForJobs = policy.WaitForJobs
	client.Timeout = policy.Timeout
	client.DisableHooks = !policy.ChartHooks
	client.SkipCRDs = policy.CRDs == crdPolicySkip
	// Install exposes ServerSideApply as a bool; leave the Helm default (true)
	// untouched when unset so omitting the setting is byte-for-byte unchanged.
	switch policy.ServerSideApply {
	case serverSideApplyFalse:
		client.ServerSideApply = false
	case serverSideApplyAuto, serverSideApplyTrue:
		client.ServerSideApply = true
	case serverSideApplyUnset:
	}
	client.ForceConflicts = policy.ForceConflicts
}

func configureUpgradeLifecycle(client *action.Upgrade, policy effectiveReleasePolicy) {
	// Atmos owns the rollback-on-failure so it can capture crash-looping pod
	// diagnostics before the rollback replaces the failing pods
	// (cloudposse/atmos#3271); see upgradeRelease. Helm's own RollbackOnFailure is
	// therefore left off.
	client.RollbackOnFailure = false
	client.WaitStrategy = policy.WaitStrategy
	client.WaitForJobs = policy.WaitForJobs
	client.Timeout = policy.Timeout
	client.CleanupOnFail = policy.CleanupOnFailure
	client.MaxHistory = policy.MaxHistory
	client.DisableHooks = !policy.ChartHooks
	// Upgrade exposes ServerSideApply as a string (auto|true|false); leave the
	// Helm default ("auto") untouched when unset.
	if policy.ServerSideApply != serverSideApplyUnset {
		client.ServerSideApply = string(policy.ServerSideApply)
	}
	client.ForceConflicts = policy.ForceConflicts
}

// ensureUpgradeForceConflictsCompatible rejects force_conflicts on an upgrade
// whose resolved apply method is client-side, which Helm refuses only after the
// chart is loaded and a revision is persisted ("forceConflicts enabled when
// serverSideApply disabled"). An explicit server_side_apply: false is already
// rejected at resolution; here we resolve the unset/auto method against the
// previous release's recorded apply method so the failure surfaces before any
// chart download or cluster mutation, and so an automatic rollback does not
// inherit the same doomed combination.
func ensureUpgradeForceConflictsCompatible(actx *actionContext, spec *chartSpec) error {
	policy := spec.Lifecycle.Policy
	if !policy.ForceConflicts || policy.ServerSideApply == serverSideApplyTrue {
		return nil
	}
	if policy.ServerSideApply == serverSideApplyFalse {
		return errUtils.ErrHelmForceConflictsRequiresSSA
	}
	// Unset or auto: Helm enables server-side apply only when the previous
	// release used it, so inspect the recorded apply method.
	lastReleaser, err := actx.cfg.Releases.Last(spec.ReleaseName)
	if err != nil {
		// No readable release history (e.g. a storage error or missing release):
		// this preflight has no basis to reject, so defer to Helm's own handling
		// rather than blocking the upgrade. Absence of history is not a validation
		// failure.
		return nil //nolint:nilerr // intentional: unreadable history means "cannot preflight", not "invalid".
	}
	if lastReleaser == nil {
		return nil
	}
	last, ok := lastReleaser.(*release.Release)
	if !ok || last.ApplyMethod == string(release.ApplyMethodServerSideApply) {
		return nil
	}
	return fmt.Errorf("%w: the current release of %q uses client-side apply, so upgrade 'auto' leaves it disabled; set server_side_apply: true to force conflicts",
		errUtils.ErrHelmForceConflictsRequiresSSA, spec.ReleaseName)
}

func configureUninstallLifecycle(client *action.Uninstall, policy effectiveReleasePolicy, dryRun bool) {
	client.WaitStrategy = policy.WaitStrategy
	client.Timeout = policy.Timeout
	client.DisableHooks = !policy.ChartHooks
	client.DryRun = dryRun
}
