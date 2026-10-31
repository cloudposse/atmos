package configuration

import (
	"context"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/component"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

const (
	componentsKey = "components"
	terraformType = "terraform"
)

func addressKey(a *pluginv1.ComponentAddress) [3]string {
	return [3]string{a.Stack, a.ComponentType, a.Component}
}

func (s *Session) discover(ctx context.Context) error {
	definitions := make(map[string]map[string]any)
	instances := make(map[[3]string]*pluginv1.DiscoveredComponent)
	if s.configErr != nil {
		return s.configErr
	}
	if s.hasProviderOptions() {
		registry, err := s.providers(ctx)
		if err != nil {
			return err
		}
		if err := s.validateOptions(registry); err != nil {
			return err
		}
	}
	if len(s.requests) == 0 {
		// Only explicit imports are in play and they were resolved with the stack manifests.
		s.definitions, s.instances = definitions, instances
		return nil
	}
	registry, err := s.providers(ctx)
	if err != nil {
		return err
	}
	failures := make(map[failedKey]*failedUnit)
	for _, request := range s.requests {
		provider, exists := registry[request.Provider]
		if !exists || provider.ComponentType() != terraformType {
			return errUtils.ErrPluginUnsupportedExtension
		}
		discovered, err := provider.Discover(ctx, request)
		if err != nil {
			return err
		}
		if err := recordDiagnostics(request.Provider, discovered.Diagnostics, failures); err != nil {
			return err
		}
		if err := addDiscovered(definitions, instances, s.withoutImported(discovered)); err != nil {
			return err
		}
	}
	// A unit with a missing or failed dependency fails alone; every healthy unit stays usable.
	settleFailures(definitions, instances, failures)

	s.definitions, s.instances, s.failures = definitions, instances, failures
	return nil
}

// withoutImported drops units that a stack imported explicitly: an imported unit is a
// component of the importing stack, not also a component of a derived stack.
func (s *Session) withoutImported(discovered *pluginv1.DiscoverComponentsResponse) *pluginv1.DiscoverComponentsResponse {
	s.importMu.Lock()
	defer s.importMu.Unlock()
	if len(s.imported) == 0 {
		return discovered
	}
	kept := &pluginv1.DiscoverComponentsResponse{Diagnostics: discovered.Diagnostics}
	for _, instance := range discovered.Components {
		if s.imported[instance.Source] == nil {
			kept.Components = append(kept.Components, instance)
		}
	}
	return kept
}

func addInstance(definitions map[string]map[string]any, instance *pluginv1.DiscoveredComponent) error {
	address := instance.Address
	if !filepath.IsLocal(address.Stack) || !filepath.IsLocal(address.Component) {
		return errUtils.ErrPluginProtocol
	}
	config, err := instanceConfig(instance, false)
	if err != nil {
		return err
	}
	definition := definitions[address.Stack]
	if definition == nil {
		definition = map[string]any{"name": address.Stack, componentsKey: map[string]any{terraformType: map[string]any{}}}
		definitions[address.Stack] = definition
	}
	definition[componentsKey].(map[string]any)[terraformType].(map[string]any)[address.Component] = config
	return nil
}

// instanceConfig builds the component section for one discovered unit. A unit imported into
// a stack depends on its siblings in that stack, so its dependencies carry no stack.
func instanceConfig(instance *pluginv1.DiscoveredComponent, imported bool) (map[string]any, error) {
	if !filepath.IsLocal(instance.Address.Component) {
		return nil, errUtils.ErrPluginProtocol
	}
	if instance.Engine != terraformType && instance.Engine != "opentofu" {
		return nil, errUtils.ErrPluginUnsupportedExtension
	}
	config, err := adapter.DecodeObject(instance.Configuration)
	if err != nil {
		return nil, err
	}
	settings, _ := config["settings"].(map[string]any)
	if settings == nil {
		settings = make(map[string]any)
	}
	configuration, _ := settings["configuration"].(map[string]any)
	if configuration == nil {
		configuration = make(map[string]any)
	}
	configuration["provider"], configuration["engine"], configuration["source"] = instance.Provider, instance.Engine, instance.Source
	settings["configuration"] = configuration
	config["settings"] = settings
	config["dependencies"] = instanceDependencies(instance, imported)
	return config, nil
}

func instanceDependencies(instance *pluginv1.DiscoveredComponent, imported bool) map[string]any {
	deps := map[string]any{"files": []any{}, "folders": []any{}, componentsKey: []any{}}
	for _, source := range instance.Sources {
		field := "files"
		if source.Kind == pluginv1.ComponentSource_DIRECTORY {
			field = "folders"
		}
		deps[field] = append(deps[field].([]any), source.Path)
	}
	for _, dependency := range instance.Dependencies {
		target := map[string]any{"name": dependency.Target.Component, "kind": dependency.Target.ComponentType}
		if !imported {
			target["stack"] = dependency.Target.Stack
		}
		deps[componentsKey] = append(deps[componentsKey].([]any), target)
	}
	return deps
}

func addDiscovered(definitions map[string]map[string]any, instances map[[3]string]*pluginv1.DiscoveredComponent, discovered *pluginv1.DiscoverComponentsResponse) error {
	for _, instance := range discovered.Components {
		key := addressKey(instance.Address)
		if previous, exists := instances[key]; exists {
			return errUtils.Build(errUtils.ErrDuplicatePluginExtension).
				WithExplanationf("Component `%s` in stack `%s` is produced by both %s and %s.", key[2], key[0], previous.Source, instance.Source).
				WithContext("component", key[2]).
				WithContext("stack", key[0]).
				WithHint("Remove one of the units. A symlinked `terragrunt.hcl` or overlapping `included_paths` entries can repeat a unit.").
				Err()
		}
		if err := addInstance(definitions, instance); err != nil {
			return err
		}
		instances[key] = instance
	}
	return nil
}
