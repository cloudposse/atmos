package helm

import (
	"fmt"
	"time"

	"helm.sh/helm/v4/pkg/kube"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
)

const helmTimeoutMigrationActive = true

type lifecycleWarningCode string

const (
	warningWaitBoolean      lifecycleWarningCode = "wait_boolean_deprecated"
	warningWaitDerived      lifecycleWarningCode = "wait_strategy_derived_from_recovery"
	warningTimeoutMigration lifecycleWarningCode = "timeout_default_migration"
)

type lifecycleWarning struct {
	Code    lifecycleWarningCode
	Field   string
	Message string
}

var lifecycleFlagKeys = []string{
	cfg.HelmOnFailureSectionName,
	cfg.HelmCleanupOnFailureSectionName,
	cfg.HelmWaitStrategySectionName,
	cfg.HelmWaitJobsSectionName,
	cfg.HelmTimeoutSectionName,
	cfg.HelmHistoryMaxSectionName,
	cfg.HelmChartHooksSectionName,
	cfg.HelmCRDsSectionName,
}

type failurePolicy string

const (
	failurePolicyKeep      failurePolicy = "keep"
	failurePolicyUninstall failurePolicy = "uninstall"
	failurePolicyRollback  failurePolicy = "rollback"
)

type crdPolicy string

const (
	crdPolicyCreate crdPolicy = "create"
	crdPolicySkip   crdPolicy = "skip"
)

type waitPolicyInput struct {
	Strategy *string
	Jobs     *bool
}

type historyPolicyInput struct {
	Max *int
}

type installPolicyInput struct {
	Timeout    *string
	ChartHooks *bool
	Wait       waitPolicyInput
	CRDs       *string
	OnFailure  *string
}

type upgradePolicyInput struct {
	Timeout          *string
	ChartHooks       *bool
	Wait             waitPolicyInput
	OnFailure        *string
	CleanupOnFailure *bool
}

type deletePolicyInput struct {
	Timeout    *string
	ChartHooks *bool
	Wait       waitPolicyInput
}

// releasePolicyInput is the presence-aware release tree after normal Atmos
// stack merging but before an install, upgrade, or delete action is selected.
type releasePolicyInput struct {
	Timeout    *string
	ChartHooks *bool
	Wait       waitPolicyInput
	History    historyPolicyInput
	Install    installPolicyInput
	Upgrade    upgradePolicyInput
	Delete     deletePolicyInput
}

// effectiveReleasePolicy is the flat policy for one selected Helm action.
type effectiveReleasePolicy struct {
	Operation        string
	Timeout          time.Duration
	ChartHooks       bool
	WaitStrategy     kube.WaitStrategy
	WaitForJobs      bool
	MaxHistory       int
	OnFailure        failurePolicy
	CleanupOnFailure bool
	CRDs             crdPolicy
}

// releaseLifecycleResolution retains presence-derived metadata that must not
// be sent to Helm actions but is needed for migration warnings and summaries.
type releaseLifecycleResolution struct {
	Policy          effectiveReleasePolicy
	TimeoutExplicit bool
	TimeoutField    string
	Warnings        []lifecycleWarning
}

func defaultReleasePolicy(operation string) effectiveReleasePolicy {
	return effectiveReleasePolicy{
		Operation:    operation,
		ChartHooks:   true,
		WaitStrategy: kube.HookOnlyStrategy,
		MaxHistory:   cfg.HelmDefaultMaxHistory,
		OnFailure:    failurePolicyKeep,
		CRDs:         crdPolicyCreate,
	}
}

func resolveReleaseLifecycle(input releasePolicyInput, operation string, emitMigrationWarning bool) (releaseLifecycleResolution, error) {
	resolution := releaseLifecycleResolution{
		Policy:       defaultReleasePolicy(operation),
		TimeoutField: "built-in default",
	}
	if err := applyCommonPolicy(&resolution, input.Timeout, input.ChartHooks, input.Wait, "release"); err != nil {
		return releaseLifecycleResolution{}, err
	}
	if input.History.Max != nil && operation == releaseOperationUpgrade {
		if err := applyMaxHistory(&resolution, *input.History.Max, "release.history.max"); err != nil {
			return releaseLifecycleResolution{}, err
		}
	}

	var err error
	switch operation {
	case releaseOperationInstall:
		err = applyInstallPolicy(&resolution, input.Install)
	case releaseOperationUpgrade:
		err = applyUpgradePolicy(&resolution, input.Upgrade)
	case releaseOperationDelete:
		err = applyDeletePolicy(&resolution, input.Delete)
	default:
		return releaseLifecycleResolution{}, fmt.Errorf("%w: %q", errUtils.ErrHelmUnsupportedOperation, operation)
	}
	if err != nil {
		return releaseLifecycleResolution{}, err
	}
	if !resolution.TimeoutExplicit && emitMigrationWarning && helmTimeoutMigrationActive {
		resolution.Warnings = append(resolution.Warnings, lifecycleWarning{
			Code:    warningTimeoutMigration,
			Field:   "release." + operation + ".timeout",
			Message: "helm release timeout is omitted; this release preserves 0s, but the default will become 5m in the next minor release",
		})
	}
	if err := validateAndDeriveLifecycle(&resolution); err != nil {
		return releaseLifecycleResolution{}, err
	}
	return resolution, nil
}

func applyCommonPolicy(resolution *releaseLifecycleResolution, timeout *string, chartHooks *bool, wait waitPolicyInput, path string) error {
	if timeout != nil {
		if err := applyTimeout(resolution, *timeout, path+".timeout"); err != nil {
			return err
		}
	}
	if chartHooks != nil {
		resolution.Policy.ChartHooks = *chartHooks
	}
	if wait.Strategy != nil {
		strategy, err := parseWaitStrategy(*wait.Strategy)
		if err != nil {
			return fmt.Errorf("%s.strategy: %w", path+".wait", err)
		}
		resolution.Policy.WaitStrategy = strategy
	}
	if wait.Jobs != nil && resolution.Policy.Operation != releaseOperationDelete {
		resolution.Policy.WaitForJobs = *wait.Jobs
	}
	return nil
}

func applyInstallPolicy(resolution *releaseLifecycleResolution, input installPolicyInput) error {
	if err := applyCommonPolicy(resolution, input.Timeout, input.ChartHooks, input.Wait, "release.install"); err != nil {
		return err
	}
	if input.CRDs != nil {
		policy, err := parseCRDPolicy(*input.CRDs)
		if err != nil {
			return err
		}
		resolution.Policy.CRDs = policy
	}
	if input.OnFailure != nil {
		policy, err := parseFailurePolicy(*input.OnFailure, releaseOperationInstall)
		if err != nil {
			return err
		}
		resolution.Policy.OnFailure = policy
	}
	return nil
}

func applyUpgradePolicy(resolution *releaseLifecycleResolution, input upgradePolicyInput) error {
	if err := applyCommonPolicy(resolution, input.Timeout, input.ChartHooks, input.Wait, "release.upgrade"); err != nil {
		return err
	}
	if input.OnFailure != nil {
		policy, err := parseFailurePolicy(*input.OnFailure, releaseOperationUpgrade)
		if err != nil {
			return err
		}
		resolution.Policy.OnFailure = policy
	}
	if input.CleanupOnFailure != nil {
		resolution.Policy.CleanupOnFailure = *input.CleanupOnFailure
	}
	return nil
}

func applyDeletePolicy(resolution *releaseLifecycleResolution, input deletePolicyInput) error {
	return applyCommonPolicy(resolution, input.Timeout, input.ChartHooks, input.Wait, "release.delete")
}

func validateAndDeriveLifecycle(resolution *releaseLifecycleResolution) error {
	recoveryEnabled := resolution.Policy.OnFailure == failurePolicyUninstall || resolution.Policy.OnFailure == failurePolicyRollback
	if recoveryEnabled && resolution.Policy.WaitStrategy == kube.HookOnlyStrategy {
		resolution.Policy.WaitStrategy = kube.StatusWatcherStrategy
		resolution.Warnings = append(resolution.Warnings, lifecycleWarning{
			Code:    warningWaitDerived,
			Field:   "release." + resolution.Policy.Operation + ".wait.strategy",
			Message: "helm wait strategy was derived as 'watcher' because the selected on_failure policy requires recovery",
		})
	}
	if resolution.Policy.WaitForJobs && resolution.Policy.WaitStrategy == kube.HookOnlyStrategy {
		return errUtils.ErrHelmWaitForJobsRequiresWait
	}
	return nil
}

// resolveReleaseLifecycleWithFlags resolves configuration for the selected
// action, then overlays only explicitly supplied CLI values at highest priority.
func resolveReleaseLifecycleWithFlags(input releasePolicyInput, operation string, flags map[string]any) (releaseLifecycleResolution, error) {
	resolution, err := resolveReleaseLifecycle(input, operation, true)
	if err != nil {
		return releaseLifecycleResolution{}, err
	}

	if err := validateLifecycleFlagApplicability(operation, flags); err != nil {
		return releaseLifecycleResolution{}, err
	}
	if value, ok := flags[cfg.HelmWaitStrategySectionName].(string); ok {
		normalized, warning := normalizeWaitFlag(value)
		strategy, parseErr := parseWaitStrategy(normalized)
		if parseErr != nil {
			return releaseLifecycleResolution{}, parseErr
		}
		resolution.Policy.WaitStrategy = strategy
		if warning != nil {
			resolution.Warnings = append(resolution.Warnings, *warning)
		}
	}
	if value, ok := flags[cfg.HelmWaitJobsSectionName].(bool); ok {
		resolution.Policy.WaitForJobs = value
	}
	if value, ok := flags[cfg.HelmTimeoutSectionName].(string); ok {
		if err := applyTimeout(&resolution, value, "--timeout"); err != nil {
			return releaseLifecycleResolution{}, err
		}
		resolution.Warnings = removeLifecycleWarning(resolution.Warnings, warningTimeoutMigration)
	}
	if value, ok := flags[cfg.HelmHistoryMaxSectionName].(int); ok {
		if err := applyMaxHistory(&resolution, value, "--history-max"); err != nil {
			return releaseLifecycleResolution{}, err
		}
	}
	if value, ok := flags[cfg.HelmChartHooksSectionName].(bool); ok {
		resolution.Policy.ChartHooks = value
	}
	if value, ok := flags[cfg.HelmCRDsSectionName].(string); ok {
		policy, parseErr := parseCRDPolicy(value)
		if parseErr != nil {
			return releaseLifecycleResolution{}, parseErr
		}
		resolution.Policy.CRDs = policy
	}
	if value, ok := flags[cfg.HelmOnFailureSectionName].(string); ok {
		policy, parseErr := parseFailurePolicy(value, operation)
		if parseErr != nil {
			return releaseLifecycleResolution{}, parseErr
		}
		resolution.Policy.OnFailure = policy
	}
	if value, ok := flags[cfg.HelmCleanupOnFailureSectionName].(bool); ok {
		resolution.Policy.CleanupOnFailure = value
	}

	resolution.Warnings = removeLifecycleWarning(resolution.Warnings, warningWaitDerived)
	if err := validateAndDeriveLifecycle(&resolution); err != nil {
		return releaseLifecycleResolution{}, err
	}
	return resolution, nil
}

func validateLifecycleFlagApplicability(operation string, flags map[string]any) error {
	inapplicable := func(key, expected string) error {
		if _, ok := flags[key]; ok {
			return fmt.Errorf("%w: %s requires %s", errUtils.ErrHelmLifecycleFlagInapplicable, key, expected)
		}
		return nil
	}
	switch operation {
	case releaseOperationInstall:
		if err := inapplicable(cfg.HelmCleanupOnFailureSectionName, "an upgrade operation"); err != nil {
			return err
		}
		return inapplicable(cfg.HelmHistoryMaxSectionName, "an upgrade operation")
	case releaseOperationUpgrade:
		return inapplicable(cfg.HelmCRDsSectionName, "an install operation")
	case releaseOperationDelete:
		for _, key := range []string{cfg.HelmOnFailureSectionName, cfg.HelmCleanupOnFailureSectionName, cfg.HelmWaitJobsSectionName, cfg.HelmHistoryMaxSectionName, cfg.HelmCRDsSectionName} {
			if err := inapplicable(key, "an install or upgrade operation"); err != nil {
				return err
			}
		}
	}
	return nil
}

func normalizeWaitFlag(value string) (string, *lifecycleWarning) {
	var warning *lifecycleWarning
	switch value {
	case "true":
		value = string(kube.StatusWatcherStrategy)
		warning = &lifecycleWarning{Code: warningWaitBoolean, Field: "--wait", Message: "boolean '--wait=true' is deprecated; use '--wait=watcher'"}
	case "false":
		value = string(kube.HookOnlyStrategy)
		warning = &lifecycleWarning{Code: warningWaitBoolean, Field: "--wait", Message: "boolean '--wait=false' is deprecated; use '--wait=hookOnly'"}
	}
	return value, warning
}

func hasExplicitLifecycleFlags(flags map[string]any) bool {
	for _, key := range lifecycleFlagKeys {
		if _, ok := flags[key]; ok {
			return true
		}
	}
	return false
}

func applyTimeout(resolution *releaseLifecycleResolution, value, field string) error {
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("%w: %s=%q: %w", errUtils.ErrHelmTimeoutInvalid, field, value, err)
	}
	if parsed < 0 {
		return fmt.Errorf("%w: %s must not be negative", errUtils.ErrHelmTimeoutInvalid, field)
	}
	resolution.Policy.Timeout = parsed
	resolution.TimeoutExplicit = true
	resolution.TimeoutField = field
	return nil
}

func applyMaxHistory(resolution *releaseLifecycleResolution, value int, field string) error {
	if value < 0 {
		return fmt.Errorf("%w: %s must not be negative", errUtils.ErrHelmMaxHistoryInvalid, field)
	}
	resolution.Policy.MaxHistory = value
	return nil
}

func parseFailurePolicy(value, operation string) (failurePolicy, error) {
	policy := failurePolicy(value)
	switch operation {
	case releaseOperationInstall:
		if policy == failurePolicyKeep || policy == failurePolicyUninstall {
			return policy, nil
		}
		return "", fmt.Errorf("%w: %q for install (want uninstall or keep)", errUtils.ErrHelmFailureActionInvalid, value)
	case releaseOperationUpgrade:
		if policy == failurePolicyKeep || policy == failurePolicyRollback {
			return policy, nil
		}
		return "", fmt.Errorf("%w: %q for upgrade (want rollback or keep)", errUtils.ErrHelmFailureActionInvalid, value)
	default:
		return "", fmt.Errorf("%w: on_failure does not apply to %s", errUtils.ErrHelmFailureActionInvalid, operation)
	}
}

func parseCRDPolicy(value string) (crdPolicy, error) {
	policy := crdPolicy(value)
	if policy == crdPolicyCreate || policy == crdPolicySkip {
		return policy, nil
	}
	return "", fmt.Errorf("%w: release.install.crds=%q (want create or skip)", errUtils.ErrHelmLifecycleDecode, value)
}

func parseWaitStrategy(value string) (kube.WaitStrategy, error) {
	switch kube.WaitStrategy(value) {
	case kube.HookOnlyStrategy, kube.StatusWatcherStrategy, kube.LegacyStrategy:
		return kube.WaitStrategy(value), nil
	default:
		return "", fmt.Errorf("%w: %q (want watcher, hookOnly, or legacy)", errUtils.ErrHelmWaitStrategyInvalid, value)
	}
}

func removeLifecycleWarning(warnings []lifecycleWarning, code lifecycleWarningCode) []lifecycleWarning {
	filtered := warnings[:0]
	for _, warning := range warnings {
		if warning.Code != code {
			filtered = append(filtered, warning)
		}
	}
	return filtered
}
