package configuration

import (
	"context"
	"maps"
	"slices"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/component"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

// failedKey is the stack and component of a unit whose discovery failed.
type failedKey [2]string

// failedUnit is a discovered unit that cannot run. It stays visible as a placeholder component
// so that commands naming it, and bulk runs that select it, fail with its own diagnostic while
// every healthy component keeps working.
type failedUnit struct {
	provider, source string
	err              error
}

// recordDiagnostics interprets a discovery response's diagnostics. Warnings print and never fail.
// An error naming a unit is held for the commands that target that unit; an error that names no
// unit is a project-level failure and is returned.
func recordDiagnostics(provider string, diagnostics []*pluginv1.Diagnostic, failures map[failedKey]*failedUnit) error {
	var projectLevel error
	for _, diagnostic := range diagnostics {
		err := adapter.DiagnosticError(diagnostic)
		if err == nil {
			adapter.WarnDiagnostic(diagnostic)
			continue
		}
		detail := diagnostic.GetDetail()
		if detail.GetUnit() == "" || detail.GetStack() == "" {
			if projectLevel == nil {
				projectLevel = err
			}
			continue
		}
		key := failedKey{detail.Stack, detail.Unit}
		if failures[key] == nil {
			failures[key] = &failedUnit{provider: provider, source: detail.GetLocation().GetFile(), err: err}
		}
	}
	return projectLevel
}

// settleFailures removes units whose dependencies are missing or failed, then adds a placeholder
// component for every failed unit. Removal repeats until no healthy unit depends on a failure.
func settleFailures(definitions map[string]map[string]any, instances map[[3]string]*pluginv1.DiscoveredComponent, failures map[failedKey]*failedUnit) {
	for {
		broken := false
		for _, key := range slices.SortedFunc(maps.Keys(instances), func(a, b [3]string) int { return slices.Compare(a[:], b[:]) }) {
			instance := instances[key]
			if err := brokenDependency(instance, instances, failures); err != nil {
				failures[failedKey{instance.Address.Stack, instance.Address.Component}] = &failedUnit{provider: instance.Provider, source: instance.Source, err: err}
				delete(instances, key)
				removeComponent(definitions, instance.Address)
				broken = true
			}
		}
		if !broken {
			break
		}
	}
	for key, failure := range failures {
		addPlaceholder(definitions, key, failure)
	}
}

func brokenDependency(instance *pluginv1.DiscoveredComponent, instances map[[3]string]*pluginv1.DiscoveredComponent, failures map[failedKey]*failedUnit) error {
	for _, dep := range instance.Dependencies {
		target := dep.Target.Stack + "/" + dep.Target.Component
		if failure := failures[failedKey{dep.Target.Stack, dep.Target.Component}]; failure != nil {
			return errUtils.Build(errUtils.ErrPluginInvalidConfig).
				WithExplanationf("Component `%s` in stack `%s` depends on `%s`, which failed discovery: %s", instance.Address.Component, instance.Address.Stack, target, failure.err).
				WithHintf("Fix `%s` first; run `atmos terraform plan %s -s %s` to see its error.", target, dep.Target.Component, dep.Target.Stack).
				Err()
		}
		if _, exists := instances[addressKey(dep.Target)]; !exists {
			return errUtils.Build(errUtils.ErrPluginInvalidConfig).
				WithExplanationf("Component `%s` in stack `%s` (%s) depends on `%s`, which is not part of the discovered components.", instance.Address.Component, instance.Address.Stack, instance.Source, target).
				WithHint("Include the dependency in `discovery.included_paths` and keep it out of `discovery.excluded_paths`.").
				WithHint("A unit imported with `terragrunt://` belongs to the importing stack and is not discovered separately; import its dependents the same way.").
				Err()
		}
	}
	return nil
}

func removeComponent(definitions map[string]map[string]any, address *pluginv1.ComponentAddress) {
	if definition := definitions[address.Stack]; definition != nil {
		if components, ok := definition[componentsKey].(map[string]any); ok {
			if terraform, ok := components[terraformType].(map[string]any); ok {
				delete(terraform, address.Component)
			}
		}
	}
}

// addPlaceholder declares a failed unit as a component that carries no variables and is marked
// failed, so that Prepare and the bulk pre-flight raise its diagnostic.
func addPlaceholder(definitions map[string]map[string]any, key failedKey, failure *failedUnit) {
	definition := definitions[key[0]]
	if definition == nil {
		definition = map[string]any{"name": key[0], componentsKey: map[string]any{terraformType: map[string]any{}}}
		definitions[key[0]] = definition
	}
	configuration := map[string]any{"provider": failure.provider, "engine": terraformType, "failed": true}
	if failure.source != "" {
		configuration["source"] = failure.source
	}
	definition[componentsKey].(map[string]any)[terraformType].(map[string]any)[key[1]] = map[string]any{
		"vars":     map[string]any{},
		"settings": map[string]any{"configuration": configuration},
	}
}

// ComponentFailure returns the diagnostic of a unit that failed discovery, or nil for a healthy
// or unknown component. It waits for discovery so a bulk run can reject a broken unit before it
// runs any infrastructure command.
func (s *Session) ComponentFailure(ctx context.Context, stack, component string) error {
	defer perf.Track(nil, "configurationadapter.ComponentFailure")()

	if err := s.ensure(ctx); err != nil {
		return err
	}
	if failure := s.failures[failedKey{stack, component}]; failure != nil {
		return failure.err
	}
	return nil
}

// failureFor wraps a unit's discovery diagnostic for the command that targets it.
func (s *Session) failureFor(stack, component string) error {
	if failure := s.failures[failedKey{stack, component}]; failure != nil {
		return failure.err
	}
	return nil
}
