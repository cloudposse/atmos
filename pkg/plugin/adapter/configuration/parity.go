package configuration

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/component"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/schema"
)

// preparedErrors copies the provider's retry and ignore policies. Anything the host cannot honor,
// such as a pattern that does not compile, is a protocol error and never a silently skipped policy.
func preparedErrors(source *pluginv1.ComponentErrorsConfiguration) (*schema.ComponentErrors, error) {
	if source == nil || len(source.GetRetry())+len(source.GetIgnore()) == 0 {
		return nil, nil
	}
	result := &schema.ComponentErrors{}
	for _, retry := range source.GetRetry() {
		if retry == nil || retry.GetMaxAttempts() < 1 || validatePatterns(retry.GetRetryableErrors()) != nil {
			return nil, fmt.Errorf("%w: invalid retry policy", errUtils.ErrPluginProtocol)
		}
		result.Retry = append(result.Retry, schema.ComponentRetryPolicy{
			Name: retry.GetName(), Patterns: slices.Clone(retry.GetRetryableErrors()),
			MaxAttempts: int(retry.GetMaxAttempts()), SleepInterval: time.Duration(retry.GetSleepIntervalSeconds()) * time.Second,
		})
	}
	for _, ignore := range source.GetIgnore() {
		policy, err := preparedIgnore(ignore)
		if err != nil {
			return nil, err
		}
		result.Ignore = append(result.Ignore, policy)
	}
	return result, nil
}

// preparedIgnore copies one ignore policy and decodes its signals to plain values.
func preparedIgnore(ignore *pluginv1.ComponentIgnorePolicy) (schema.ComponentIgnorePolicy, error) {
	if ignore == nil || validatePatterns(ignore.GetIgnorableErrors()) != nil {
		return schema.ComponentIgnorePolicy{}, fmt.Errorf("%w: invalid ignore policy", errUtils.ErrPluginProtocol)
	}
	signals := make(map[string]any, len(ignore.GetSignals()))
	for name, value := range ignore.GetSignals() {
		decoded, err := adapter.DecodeValue(value)
		if err != nil {
			return schema.ComponentIgnorePolicy{}, err
		}
		signals[name] = decoded
	}
	return schema.ComponentIgnorePolicy{Name: ignore.GetName(), Patterns: slices.Clone(ignore.GetIgnorableErrors()), Message: ignore.GetMessage(), Signals: signals}, nil
}

// validatePatterns checks that every error pattern compiles, ignoring the "!" veto prefix.
func validatePatterns(patterns []string) error {
	for _, pattern := range patterns {
		if _, err := regexp.Compile(strings.TrimPrefix(pattern, "!")); err != nil {
			return err
		}
	}
	return nil
}

// preparedExclusion records that the provider wants this operation skipped.
func preparedExclusion(source *pluginv1.ComponentExclude) *schema.ComponentExclusion {
	if source == nil || !source.GetCondition() {
		return nil
	}
	return &schema.ComponentExclusion{Actions: slices.Clone(source.GetActions())}
}

// checkTerraformBinary verifies that the executable the unit selects now is the one it selected at
// discovery, because discovery handed it to the host as the component's `command`. A difference
// means the unit's configuration changed between the two evaluations.
func checkTerraformBinary(info *schema.ConfigAndStacksInfo, instance *pluginv1.DiscoveredComponent, binary string) error {
	discovered, err := adapter.DecodeObject(instance.Configuration)
	if err != nil {
		return err
	}
	// A terraform_binary removed after discovery is a change too: the host would otherwise keep
	// running the executable discovery selected.
	command, _ := discovered["command"].(string)
	if command == binary {
		return nil
	}
	return errUtils.Build(fmt.Errorf("%w: component %q in stack %q now selects %q, but was discovered with %q", errUtils.ErrComponentBinaryChanged, info.ComponentFromArg, info.Stack, binary, command)).
		WithContext("component", info.ComponentFromArg).
		WithContext("stack", info.Stack).
		WithHint("Run the command again; `terraform_binary` must evaluate to the same value every time, so avoid values that differ between invocations").
		Err()
}

// warnDiagnostics prints the provider's warnings about a unit and never fails the command.
func warnDiagnostics(diagnostics []*pluginv1.Diagnostic, warnings *adapter.WarningDeduper) {
	for _, diagnostic := range diagnostics {
		adapter.WarnDiagnostic(diagnostic, warnings)
	}
}

// applyParity copies the settings that select how a unit runs into the prepared configuration:
// the executable, error handling, exclusion, and feature values. Error handling lives with the
// execution metadata because the host applies it around the same subprocesses as hooks.
func applyParity(prepared *schema.PreparedComponentConfiguration, result *pluginv1.PrepareComponentResponse) error {
	policies, err := preparedErrors(result.GetErrors())
	if err != nil {
		return err
	}
	if policies != nil {
		if prepared.Execution == nil {
			prepared.Execution = &schema.ComponentExecution{}
		}
		prepared.Execution.Errors = policies
		// The unit's errors block is evaluated once, so a saved plan applied with the
		// apply-time metadata keeps the same retry and ignore policies.
		if prepared.Execution.Apply != nil {
			prepared.Execution.Apply.Errors = policies
		}
	}
	prepared.TerraformBinary = result.GetTerraformBinary()
	prepared.Exclusion = preparedExclusion(result.GetExclude())
	prepared.FeatureFlags = result.GetFeatureFlags()
	return nil
}
