package helm

import (
	"fmt"
	"reflect"
	"time"

	"helm.sh/helm/v4/pkg/kube"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
)

const (
	defaultHelmMaxHistory      = 10
	helmTimeoutMigrationActive = true
)

type lifecycleWarningCode string

const (
	warningAtomicDeprecated lifecycleWarningCode = "atomic_deprecated"
	warningWaitIgnored      lifecycleWarningCode = "wait_alias_ignored"
	warningTimeoutMigration lifecycleWarningCode = "timeout_default_migration"
)

type lifecycleWarning struct {
	Code    lifecycleWarningCode
	Field   string
	Message string
}

// releaseLifecycle is the canonical Helm release policy after defaults,
// aliases, and derived constraints have been resolved.
type releaseLifecycle struct {
	RollbackOnFailure bool
	WaitStrategy      kube.WaitStrategy
	WaitForJobs       bool
	Timeout           time.Duration
	CleanupOnFail     bool
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
		MaxHistory:   defaultHelmMaxHistory,
	}
}

// resolveReleaseLifecycle decodes and validates lifecycle fields from an
// already processed component section. Canonical fields win over aliases.
func resolveReleaseLifecycle(section map[string]any) (releaseLifecycleResolution, error) {
	resolution := releaseLifecycleResolution{Policy: defaultReleaseLifecycle()}

	rollbackOnFailure, err := optionalBoolField(section, cfg.HelmRollbackOnFailureSectionName)
	if err != nil {
		return releaseLifecycleResolution{}, err
	}
	atomic, err := optionalBoolField(section, cfg.HelmAtomicSectionName)
	if err != nil {
		return releaseLifecycleResolution{}, err
	}
	if atomic != nil {
		resolution.Policy.RollbackOnFailure = *atomic
		resolution.Warnings = append(resolution.Warnings, lifecycleWarning{
			Code:    warningAtomicDeprecated,
			Field:   cfg.HelmAtomicSectionName,
			Message: "helm lifecycle field 'atomic' is deprecated; use 'rollback_on_failure'",
		})
	}
	if rollbackOnFailure != nil {
		resolution.Policy.RollbackOnFailure = *rollbackOnFailure
	}

	wait, err := optionalBoolField(section, cfg.HelmWaitSectionName)
	if err != nil {
		return releaseLifecycleResolution{}, err
	}
	waitStrategy, err := optionalStringField(section, cfg.HelmWaitStrategySectionName)
	if err != nil {
		return releaseLifecycleResolution{}, err
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
			return releaseLifecycleResolution{}, parseErr
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

	waitForJobs, err := optionalBoolField(section, cfg.HelmWaitForJobsSectionName)
	if err != nil {
		return releaseLifecycleResolution{}, err
	}
	if waitForJobs != nil {
		resolution.Policy.WaitForJobs = *waitForJobs
	}

	timeout, err := optionalStringField(section, cfg.HelmTimeoutSectionName)
	if err != nil {
		return releaseLifecycleResolution{}, err
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
	} else {
		parsed, parseErr := time.ParseDuration(*timeout)
		if parseErr != nil {
			return releaseLifecycleResolution{}, fmt.Errorf("%w: %q: %w", errUtils.ErrHelmTimeoutInvalid, *timeout, parseErr)
		}
		if parsed < 0 {
			return releaseLifecycleResolution{}, fmt.Errorf("%w: %s must not be negative", errUtils.ErrHelmTimeoutInvalid, cfg.HelmTimeoutSectionName)
		}
		resolution.Policy.Timeout = parsed
	}

	cleanupOnFail, err := optionalBoolField(section, cfg.HelmCleanupOnFailSectionName)
	if err != nil {
		return releaseLifecycleResolution{}, err
	}
	if cleanupOnFail != nil {
		resolution.Policy.CleanupOnFail = *cleanupOnFail
	}

	maxHistory, err := optionalIntField(section, cfg.HelmMaxHistorySectionName)
	if err != nil {
		return releaseLifecycleResolution{}, err
	}
	if maxHistory != nil {
		if *maxHistory < 0 {
			return releaseLifecycleResolution{}, fmt.Errorf("%w: %s must not be negative", errUtils.ErrHelmMaxHistoryInvalid, cfg.HelmMaxHistorySectionName)
		}
		resolution.Policy.MaxHistory = *maxHistory
	}

	disableChartHooks, err := optionalBoolField(section, cfg.HelmDisableChartHooksSectionName)
	if err != nil {
		return releaseLifecycleResolution{}, err
	}
	if disableChartHooks != nil {
		resolution.Policy.DisableChartHooks = *disableChartHooks
	}

	skipCRDs, err := optionalBoolField(section, cfg.HelmSkipCRDsSectionName)
	if err != nil {
		return releaseLifecycleResolution{}, err
	}
	if skipCRDs != nil {
		resolution.Policy.SkipCRDs = *skipCRDs
	}

	if resolution.Policy.RollbackOnFailure && resolution.Policy.WaitStrategy == kube.HookOnlyStrategy {
		resolution.Policy.WaitStrategy = kube.StatusWatcherStrategy
	}
	if resolution.Policy.WaitForJobs && resolution.Policy.WaitStrategy == kube.HookOnlyStrategy {
		return releaseLifecycleResolution{}, errUtils.ErrHelmWaitForJobsRequiresWait
	}

	return resolution, nil
}

func parseWaitStrategy(value string) (kube.WaitStrategy, error) {
	switch kube.WaitStrategy(value) {
	case kube.HookOnlyStrategy, kube.StatusWatcherStrategy, kube.LegacyStrategy:
		return kube.WaitStrategy(value), nil
	default:
		return "", fmt.Errorf("%w: %q (want watcher, hookOnly, or legacy)", errUtils.ErrHelmWaitStrategyInvalid, value)
	}
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
		converted := int(integer)
		return converted, converted >= 0 && uint64(converted) == integer
	default:
		return 0, false
	}
}
