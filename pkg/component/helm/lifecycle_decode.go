package helm

import (
	"fmt"
	"math"
	"reflect"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
)

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
	var unknown []string
	for key := range section {
		if _, ok := accepted[key]; !ok {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Errorf("%w: unknown field %s.%s", errUtils.ErrHelmLifecycleDecode, path, unknown[0])
	}
	return nil
}

func decodeFieldError(path, expected string, value any) error {
	return fmt.Errorf("%w: %s must be %s, got %T", errUtils.ErrHelmLifecycleDecode, path, expected, value)
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
	case reflect.Float64:
		number := reflected.Float()
		if math.IsNaN(number) || math.IsInf(number, 0) || math.Trunc(number) != number ||
			number < float64(math.MinInt) || number >= -float64(math.MinInt) {
			return 0, false
		}
		return int(number), true
	default:
		return 0, false
	}
}
