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

const (
	helmTimeoutMigrationActive = true
)

type lifecycleWarningCode string

const (
	warningWaitIgnored      lifecycleWarningCode = "wait_alias_ignored"
	warningWaitBoolean      lifecycleWarningCode = "wait_boolean_deprecated"
	warningWaitDerived      lifecycleWarningCode = "wait_strategy_derived_from_rollback"
	warningTimeoutMigration lifecycleWarningCode = "timeout_default_migration"
)

type lifecycleWarning struct {
	Code    lifecycleWarningCode
	Field   string
	Message string
}

var lifecycleFlagKeys = []string{
	cfg.HelmOnFailureSectionName,
	cfg.HelmWaitStrategySectionName,
	cfg.HelmWaitForJobsSectionName,
	cfg.HelmTimeoutSectionName,
	cfg.HelmMaxHistorySectionName,
	cfg.HelmDisableChartHooksSectionName,
	cfg.HelmSkipCRDsSectionName,
}

type failureAction string

const (
	failureActionRollback failureAction = "rollback"
	failureActionCleanup  failureAction = "cleanup"
)

// releaseLifecycle is the canonical Helm release policy after defaults,
// aliases, and derived constraints have been resolved.
type releaseLifecycle struct {
	OnFailure         []failureAction
	WaitStrategy      kube.WaitStrategy
	WaitForJobs       bool
	Timeout           time.Duration
	MaxHistory        int
	DisableChartHooks bool
	SkipCRDs          bool
}

// releaseLifecycleResolution retains presence-derived metadata that must not be
// sent to Helm actions but is needed for migration warnings and summaries.
type releaseLifecycleResolution struct {
	Policy          releaseLifecycle
	TimeoutExplicit bool
	Warnings        []lifecycleWarning
}

func defaultReleaseLifecycle() releaseLifecycle {
	return releaseLifecycle{
		WaitStrategy: kube.HookOnlyStrategy,
		MaxHistory:   cfg.HelmDefaultMaxHistory,
	}
}

// resolveReleaseLifecycle decodes and validates lifecycle fields from an
// already processed component section. Canonical fields win over aliases.
func resolveReleaseLifecycle(section map[string]any) (releaseLifecycleResolution, error) {
	resolution := releaseLifecycleResolution{Policy: defaultReleaseLifecycle()}
	resolvers := []func(map[string]any, *releaseLifecycleResolution) error{
		resolveFailurePolicy,
		resolveWaitPolicy,
		resolveBooleanPolicies,
		resolveTimeoutPolicy,
		resolveMaxHistory,
	}
	for _, resolver := range resolvers {
		if err := resolver(section, &resolution); err != nil {
			return releaseLifecycleResolution{}, err
		}
	}
	if err := validateAndDeriveLifecycle(&resolution); err != nil {
		return releaseLifecycleResolution{}, err
	}
	return resolution, nil
}

func validateAndDeriveLifecycle(resolution *releaseLifecycleResolution) error {
	if resolution.Policy.hasFailureAction(failureActionRollback) && resolution.Policy.WaitStrategy == kube.HookOnlyStrategy {
		resolution.Policy.WaitStrategy = kube.StatusWatcherStrategy
		resolution.Warnings = append(resolution.Warnings, lifecycleWarning{
			Code:    warningWaitDerived,
			Field:   cfg.HelmWaitStrategySectionName,
			Message: "helm wait strategy was derived as 'watcher' because on_failure includes 'rollback'",
		})
	}
	if resolution.Policy.WaitForJobs && resolution.Policy.WaitStrategy == kube.HookOnlyStrategy {
		return errUtils.ErrHelmWaitForJobsRequiresWait
	}
	return nil
}

func resolveFailurePolicy(section map[string]any, resolution *releaseLifecycleResolution) error {
	actions, err := optionalStringSliceField(section, cfg.HelmOnFailureSectionName)
	if err != nil || actions == nil {
		return err
	}

	requested := make(map[failureAction]struct{}, len(*actions))
	for _, value := range *actions {
		action := failureAction(value)
		switch action {
		case failureActionRollback, failureActionCleanup:
			requested[action] = struct{}{}
		default:
			return fmt.Errorf("%w: %q (want rollback or cleanup)", errUtils.ErrHelmFailureActionInvalid, value)
		}
	}

	for _, action := range []failureAction{failureActionRollback, failureActionCleanup} {
		if _, ok := requested[action]; ok {
			resolution.Policy.OnFailure = append(resolution.Policy.OnFailure, action)
		}
	}
	return nil
}

func (policy releaseLifecycle) hasFailureAction(action failureAction) bool {
	for _, configured := range policy.OnFailure {
		if configured == action {
			return true
		}
	}
	return false
}

func (policy releaseLifecycle) failureActionStrings(operation string) []string {
	actions := make([]string, 0, len(policy.OnFailure))
	for _, action := range policy.OnFailure {
		if operation == releaseOperationInstall && action == failureActionCleanup {
			continue
		}
		actions = append(actions, string(action))
	}
	return actions
}

func resolveWaitPolicy(section map[string]any, resolution *releaseLifecycleResolution) error {
	wait, err := optionalBoolField(section, cfg.HelmWaitSectionName)
	if err != nil {
		return err
	}
	waitStrategy, err := optionalStringField(section, cfg.HelmWaitStrategySectionName)
	if err != nil {
		return err
	}
	if wait != nil {
		if *wait {
			resolution.Policy.WaitStrategy = kube.StatusWatcherStrategy
		} else {
			resolution.Policy.WaitStrategy = kube.HookOnlyStrategy
		}
	}
	if waitStrategy != nil {
		strategy, parseErr := parseWaitStrategy(*waitStrategy)
		if parseErr != nil {
			return parseErr
		}
		resolution.Policy.WaitStrategy = strategy
		if wait != nil {
			resolution.Warnings = append(resolution.Warnings, lifecycleWarning{
				Code:    warningWaitIgnored,
				Field:   cfg.HelmWaitSectionName,
				Message: "helm lifecycle field 'wait' is ignored because 'wait_strategy' is set",
			})
		}
	}
	return nil
}

func resolveTimeoutPolicy(section map[string]any, resolution *releaseLifecycleResolution) error {
	timeout, err := optionalStringField(section, cfg.HelmTimeoutSectionName)
	if err != nil {
		return err
	}
	resolution.TimeoutExplicit = timeout != nil
	if timeout == nil {
		if helmTimeoutMigrationActive {
			resolution.Warnings = append(resolution.Warnings, lifecycleWarning{
				Code:    warningTimeoutMigration,
				Field:   cfg.HelmTimeoutSectionName,
				Message: "helm lifecycle timeout is omitted; this release preserves 0s, but the default will become 5m in the next minor release",
			})
		}
		return nil
	}
	parsed, parseErr := time.ParseDuration(*timeout)
	if parseErr != nil {
		return fmt.Errorf("%w: %q: %w", errUtils.ErrHelmTimeoutInvalid, *timeout, parseErr)
	}
	if parsed < 0 {
		return fmt.Errorf("%w: %s must not be negative", errUtils.ErrHelmTimeoutInvalid, cfg.HelmTimeoutSectionName)
	}
	resolution.Policy.Timeout = parsed
	return nil
}

func resolveMaxHistory(section map[string]any, resolution *releaseLifecycleResolution) error {
	maxHistory, err := optionalIntField(section, cfg.HelmMaxHistorySectionName)
	if err != nil {
		return err
	}
	if maxHistory != nil {
		if *maxHistory < 0 {
			return fmt.Errorf("%w: %s must not be negative", errUtils.ErrHelmMaxHistoryInvalid, cfg.HelmMaxHistorySectionName)
		}
		resolution.Policy.MaxHistory = *maxHistory
	}
	return nil
}

func resolveBooleanPolicies(section map[string]any, resolution *releaseLifecycleResolution) error {
	fields := []struct {
		key    string
		target *bool
	}{
		{cfg.HelmWaitForJobsSectionName, &resolution.Policy.WaitForJobs},
		{cfg.HelmDisableChartHooksSectionName, &resolution.Policy.DisableChartHooks},
		{cfg.HelmSkipCRDsSectionName, &resolution.Policy.SkipCRDs},
	}
	for _, field := range fields {
		if err := applyOptionalBool(section, field.key, field.target); err != nil {
			return err
		}
	}
	return nil
}

func applyOptionalBool(section map[string]any, key string, target *bool) error {
	value, err := optionalBoolField(section, key)
	if err != nil {
		return err
	}
	if value != nil {
		*target = *value
	}
	return nil
}

func parseWaitStrategy(value string) (kube.WaitStrategy, error) {
	switch kube.WaitStrategy(value) {
	case kube.HookOnlyStrategy, kube.StatusWatcherStrategy, kube.LegacyStrategy:
		return kube.WaitStrategy(value), nil
	default:
		return "", fmt.Errorf("%w: %q (want watcher, hookOnly, or legacy)", errUtils.ErrHelmWaitStrategyInvalid, value)
	}
}

// resolveReleaseLifecycleWithFlags overlays only explicitly provided command
// flags on the processed component, then performs the normal alias resolution
// and validation once. Resolving from raw input is important because a CLI
// rollback override can change whether watcher was derived from hookOnly.
func resolveReleaseLifecycleWithFlags(section map[string]any, flags map[string]any) (releaseLifecycleResolution, error) {
	overlaid := make(map[string]any, len(section))
	for key, value := range section {
		overlaid[key] = value
	}
	extraWarnings := make([]lifecycleWarning, 0, 1)
	if value, ok := flags[cfg.HelmWaitStrategySectionName].(string); ok {
		value, warning := normalizeWaitFlag(value)
		if warning != nil {
			extraWarnings = append(extraWarnings, *warning)
		}
		overlaid[cfg.HelmWaitStrategySectionName] = value
	}
	overlayLifecyclePassthroughFlags(overlaid, flags)

	resolution, err := resolveReleaseLifecycle(overlaid)
	if err != nil {
		return releaseLifecycleResolution{}, err
	}
	resolution.Warnings = append(resolution.Warnings, extraWarnings...)
	return resolution, nil
}

func normalizeWaitFlag(value string) (string, *lifecycleWarning) {
	var warning *lifecycleWarning
	switch value {
	case "true":
		value = string(kube.StatusWatcherStrategy)
		warning = &lifecycleWarning{
			Code: warningWaitBoolean, Field: cfg.HelmWaitStrategySectionName,
			Message: "boolean '--wait=true' is deprecated; use '--wait=watcher'",
		}
	case "false":
		value = string(kube.HookOnlyStrategy)
		warning = &lifecycleWarning{
			Code: warningWaitBoolean, Field: cfg.HelmWaitStrategySectionName,
			Message: "boolean '--wait=false' is deprecated; use '--wait=hookOnly'",
		}
	}
	return value, warning
}

func overlayLifecyclePassthroughFlags(overlaid map[string]any, flags map[string]any) {
	for _, key := range []string{
		cfg.HelmOnFailureSectionName,
		cfg.HelmWaitForJobsSectionName,
		cfg.HelmTimeoutSectionName,
		cfg.HelmMaxHistorySectionName,
		cfg.HelmDisableChartHooksSectionName,
		cfg.HelmSkipCRDsSectionName,
	} {
		if value, ok := flags[key]; ok {
			overlaid[key] = value
		}
	}
}

func hasExplicitLifecycleFlags(flags map[string]any) bool {
	for _, key := range lifecycleFlagKeys {
		if _, ok := flags[key]; ok {
			return true
		}
	}
	return false
}

func optionalBoolField(section map[string]any, key string) (*bool, error) {
	value, ok := section[key]
	if !ok {
		return nil, nil
	}
	typed, ok := value.(bool)
	if !ok {
		return nil, fmt.Errorf("%w: %s must be a boolean, got %T", errUtils.ErrHelmLifecycleDecode, key, value)
	}
	return &typed, nil
}

func optionalStringField(section map[string]any, key string) (*string, error) {
	value, ok := section[key]
	if !ok {
		return nil, nil
	}
	typed, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("%w: %s must be a string, got %T", errUtils.ErrHelmLifecycleDecode, key, value)
	}
	return &typed, nil
}

func optionalStringSliceField(section map[string]any, key string) (*[]string, error) {
	value, ok := section[key]
	if !ok {
		return nil, nil
	}

	var values []string
	switch typed := value.(type) {
	case []string:
		values = append(values, typed...)
	case []any:
		values = make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%w: %s items must be strings, got %T", errUtils.ErrHelmLifecycleDecode, key, item)
			}
			values = append(values, text)
		}
	default:
		return nil, fmt.Errorf("%w: %s must be a list of strings, got %T", errUtils.ErrHelmLifecycleDecode, key, value)
	}
	return &values, nil
}

func optionalIntField(section map[string]any, key string) (*int, error) {
	value, ok := section[key]
	if !ok {
		return nil, nil
	}
	integer, ok := exactInt(value)
	if !ok {
		return nil, fmt.Errorf("%w: %s must be an integer, got %T", errUtils.ErrHelmLifecycleDecode, key, value)
	}
	return &integer, nil
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
