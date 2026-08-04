package helm

import (
	"fmt"
	"math"
	"reflect"
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

// decodeReleasePolicy decodes the strict component-owned release tree. It
// validates every operation section before chart acquisition, while selected-
// action CLI validation remains deferred until release state is known.
func decodeReleasePolicy(section map[string]any) (releasePolicyInput, error) {
	raw, ok := section[cfg.HelmReleaseSectionName]
	if !ok {
		return releasePolicyInput{}, nil
	}
	releaseMap, ok := raw.(map[string]any)
	if !ok {
		return releasePolicyInput{}, decodeFieldError(cfg.HelmReleaseSectionName, "an object", raw)
	}

	if err := rejectUnknownFields(releaseMap, cfg.HelmReleaseSectionName,
		cfg.HelmTimeoutSectionName,
		cfg.HelmChartHooksSectionName,
		cfg.HelmWaitSectionName,
		cfg.HelmHistorySectionName,
		cfg.HelmInstallSectionName,
		cfg.HelmUpgradeSectionName,
		cfg.HelmDeleteSectionName,
	); err != nil {
		return releasePolicyInput{}, err
	}

	input := releasePolicyInput{}
	var err error
	if input.Timeout, err = optionalStringField(releaseMap, cfg.HelmTimeoutSectionName, "release.timeout"); err != nil {
		return releasePolicyInput{}, err
	}
	if input.ChartHooks, err = optionalBoolField(releaseMap, cfg.HelmChartHooksSectionName, "release.chart_hooks"); err != nil {
		return releasePolicyInput{}, err
	}
	if input.Wait, err = decodeWaitPolicy(releaseMap, cfg.HelmWaitSectionName, "release.wait", true); err != nil {
		return releasePolicyInput{}, err
	}
	if input.History, err = decodeHistoryPolicy(releaseMap); err != nil {
		return releasePolicyInput{}, err
	}
	if input.Install, err = decodeInstallPolicy(releaseMap); err != nil {
		return releasePolicyInput{}, err
	}
	if input.Upgrade, err = decodeUpgradePolicy(releaseMap); err != nil {
		return releasePolicyInput{}, err
	}
	if input.Delete, err = decodeDeletePolicy(releaseMap); err != nil {
		return releasePolicyInput{}, err
	}

	for _, operation := range []string{releaseOperationInstall, releaseOperationUpgrade, releaseOperationDelete} {
		if _, err := resolveReleaseLifecycle(input, operation, false); err != nil {
			return releasePolicyInput{}, err
		}
	}
	return input, nil
}

func decodeWaitPolicy(parent map[string]any, key, path string, allowJobs bool) (waitPolicyInput, error) {
	raw, ok := parent[key]
	if !ok {
		return waitPolicyInput{}, nil
	}
	waitMap, ok := raw.(map[string]any)
	if !ok {
		return waitPolicyInput{}, decodeFieldError(path, "an object", raw)
	}
	allowed := []string{cfg.HelmWaitStrategySectionName}
	if allowJobs {
		allowed = append(allowed, cfg.HelmWaitJobsSectionName)
	}
	if err := rejectUnknownFields(waitMap, path, allowed...); err != nil {
		return waitPolicyInput{}, err
	}
	strategy, err := optionalStringField(waitMap, cfg.HelmWaitStrategySectionName, path+".strategy")
	if err != nil {
		return waitPolicyInput{}, err
	}
	jobs, err := optionalBoolField(waitMap, cfg.HelmWaitJobsSectionName, path+".jobs")
	if err != nil {
		return waitPolicyInput{}, err
	}
	return waitPolicyInput{Strategy: strategy, Jobs: jobs}, nil
}

func decodeHistoryPolicy(releaseMap map[string]any) (historyPolicyInput, error) {
	raw, ok := releaseMap[cfg.HelmHistorySectionName]
	if !ok {
		return historyPolicyInput{}, nil
	}
	historyMap, ok := raw.(map[string]any)
	if !ok {
		return historyPolicyInput{}, decodeFieldError("release.history", "an object", raw)
	}
	if err := rejectUnknownFields(historyMap, "release.history", cfg.HelmHistoryMaxSectionName); err != nil {
		return historyPolicyInput{}, err
	}
	maxHistory, err := optionalIntField(historyMap, cfg.HelmHistoryMaxSectionName, "release.history.max")
	if err != nil {
		return historyPolicyInput{}, err
	}
	return historyPolicyInput{Max: maxHistory}, nil
}

func decodeInstallPolicy(releaseMap map[string]any) (installPolicyInput, error) {
	operationMap, err := optionalObjectField(releaseMap, cfg.HelmInstallSectionName, "release.install")
	if err != nil || operationMap == nil {
		return installPolicyInput{}, err
	}
	if err := rejectUnknownFields(operationMap, "release.install",
		cfg.HelmTimeoutSectionName,
		cfg.HelmChartHooksSectionName,
		cfg.HelmWaitSectionName,
		cfg.HelmCRDsSectionName,
		cfg.HelmOnFailureSectionName,
	); err != nil {
		return installPolicyInput{}, err
	}
	input := installPolicyInput{}
	if input.Timeout, err = optionalStringField(operationMap, cfg.HelmTimeoutSectionName, "release.install.timeout"); err != nil {
		return installPolicyInput{}, err
	}
	if input.ChartHooks, err = optionalBoolField(operationMap, cfg.HelmChartHooksSectionName, "release.install.chart_hooks"); err != nil {
		return installPolicyInput{}, err
	}
	if input.Wait, err = decodeWaitPolicy(operationMap, cfg.HelmWaitSectionName, "release.install.wait", true); err != nil {
		return installPolicyInput{}, err
	}
	if input.CRDs, err = optionalStringField(operationMap, cfg.HelmCRDsSectionName, "release.install.crds"); err != nil {
		return installPolicyInput{}, err
	}
	if input.OnFailure, err = optionalStringField(operationMap, cfg.HelmOnFailureSectionName, "release.install.on_failure"); err != nil {
		return installPolicyInput{}, err
	}
	return input, nil
}

func decodeUpgradePolicy(releaseMap map[string]any) (upgradePolicyInput, error) {
	operationMap, err := optionalObjectField(releaseMap, cfg.HelmUpgradeSectionName, "release.upgrade")
	if err != nil || operationMap == nil {
		return upgradePolicyInput{}, err
	}
	if err := rejectUnknownFields(operationMap, "release.upgrade",
		cfg.HelmTimeoutSectionName,
		cfg.HelmChartHooksSectionName,
		cfg.HelmWaitSectionName,
		cfg.HelmOnFailureSectionName,
		cfg.HelmCleanupOnFailureSectionName,
	); err != nil {
		return upgradePolicyInput{}, err
	}
	input := upgradePolicyInput{}
	if input.Timeout, err = optionalStringField(operationMap, cfg.HelmTimeoutSectionName, "release.upgrade.timeout"); err != nil {
		return upgradePolicyInput{}, err
	}
	if input.ChartHooks, err = optionalBoolField(operationMap, cfg.HelmChartHooksSectionName, "release.upgrade.chart_hooks"); err != nil {
		return upgradePolicyInput{}, err
	}
	if input.Wait, err = decodeWaitPolicy(operationMap, cfg.HelmWaitSectionName, "release.upgrade.wait", true); err != nil {
		return upgradePolicyInput{}, err
	}
	if input.OnFailure, err = optionalStringField(operationMap, cfg.HelmOnFailureSectionName, "release.upgrade.on_failure"); err != nil {
		return upgradePolicyInput{}, err
	}
	if input.CleanupOnFailure, err = optionalBoolField(operationMap, cfg.HelmCleanupOnFailureSectionName, "release.upgrade.cleanup_on_failure"); err != nil {
		return upgradePolicyInput{}, err
	}
	return input, nil
}

func decodeDeletePolicy(releaseMap map[string]any) (deletePolicyInput, error) {
	operationMap, err := optionalObjectField(releaseMap, cfg.HelmDeleteSectionName, "release.delete")
	if err != nil || operationMap == nil {
		return deletePolicyInput{}, err
	}
	if err := rejectUnknownFields(operationMap, "release.delete",
		cfg.HelmTimeoutSectionName,
		cfg.HelmChartHooksSectionName,
		cfg.HelmWaitSectionName,
	); err != nil {
		return deletePolicyInput{}, err
	}
	input := deletePolicyInput{}
	if input.Timeout, err = optionalStringField(operationMap, cfg.HelmTimeoutSectionName, "release.delete.timeout"); err != nil {
		return deletePolicyInput{}, err
	}
	if input.ChartHooks, err = optionalBoolField(operationMap, cfg.HelmChartHooksSectionName, "release.delete.chart_hooks"); err != nil {
		return deletePolicyInput{}, err
	}
	if input.Wait, err = decodeWaitPolicy(operationMap, cfg.HelmWaitSectionName, "release.delete.wait", false); err != nil {
		return deletePolicyInput{}, err
	}
	return input, nil
}

func resolveReleaseLifecycle(input releasePolicyInput, operation string, emitMigrationWarning bool) (releaseLifecycleResolution, error) {
	resolution := releaseLifecycleResolution{Policy: defaultReleasePolicy(operation)}
	if err := applyCommonPolicy(&resolution, input.Timeout, input.ChartHooks, input.Wait, "release"); err != nil {
		return releaseLifecycleResolution{}, err
	}
	if input.History.Max != nil {
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

func optionalObjectField(section map[string]any, key, path string) (map[string]any, error) {
	value, ok := section[key]
	if !ok {
		return nil, nil
	}
	typed, ok := value.(map[string]any)
	if !ok {
		return nil, decodeFieldError(path, "an object", value)
	}
	return typed, nil
}

func optionalBoolField(section map[string]any, key, path string) (*bool, error) {
	value, ok := section[key]
	if !ok {
		return nil, nil
	}
	typed, ok := value.(bool)
	if !ok {
		return nil, decodeFieldError(path, "a boolean", value)
	}
	return &typed, nil
}

func optionalStringField(section map[string]any, key, path string) (*string, error) {
	value, ok := section[key]
	if !ok {
		return nil, nil
	}
	typed, ok := value.(string)
	if !ok {
		return nil, decodeFieldError(path, "a string", value)
	}
	return &typed, nil
}

func optionalIntField(section map[string]any, key, path string) (*int, error) {
	value, ok := section[key]
	if !ok {
		return nil, nil
	}
	integer, ok := exactInt(value)
	if !ok {
		return nil, decodeFieldError(path, "an integer", value)
	}
	return &integer, nil
}

func rejectUnknownFields(section map[string]any, path string, allowed ...string) error {
	accepted := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		accepted[key] = struct{}{}
	}
	for key := range section {
		if _, ok := accepted[key]; !ok {
			return fmt.Errorf("%w: unknown field %s.%s", errUtils.ErrHelmLifecycleDecode, path, key)
		}
	}
	return nil
}

func decodeFieldError(path, expected string, value any) error {
	return fmt.Errorf("%w: %s must be %s, got %T", errUtils.ErrHelmLifecycleDecode, path, expected, value)
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

func exactInt(value any) (int, bool) {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return 0, false
	}
	switch reflected.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		integer := reflected.Int()
		converted := int(integer)
		return converted, int64(converted) == integer
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		integer := reflected.Uint()
		if integer > math.MaxInt {
			return 0, false
		}
		return int(integer), true
	default:
		return 0, false
	}
}
