package configuration

import (
	"context"
	"fmt"
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
	names := make([]string, 0, len(s.requests))
	for _, request := range s.requests {
		names = append(names, request.Provider)
	}
	registry, err := s.loader(ctx, names)
	if err != nil {
		return err
	}
	definitions := make(map[string]map[string]any)
	instances := make(map[[3]string]*pluginv1.DiscoveredComponent)
	for _, request := range s.requests {
		provider, exists := registry[request.Provider]
		if !exists || provider.ComponentType() != terraformType {
			return errUtils.ErrPluginUnsupportedExtension
		}
		discovered, err := provider.Discover(ctx, request)
		if err != nil {
			return err
		}
		if err := addDiscovered(definitions, instances, discovered); err != nil {
			return err
		}
	}
	if err := checkDependencies(instances); err != nil {
		return err
	}

	s.registry, s.definitions, s.instances = registry, definitions, instances
	return nil
}

func addInstance(definitions map[string]map[string]any, instance *pluginv1.DiscoveredComponent) error {
	address := instance.Address
	if !filepath.IsLocal(address.Stack) || !filepath.IsLocal(address.Component) {
		return errUtils.ErrPluginProtocol
	}
	if instance.Engine != terraformType && instance.Engine != "opentofu" {
		return errUtils.ErrPluginUnsupportedExtension
	}
	config, err := adapter.DecodeObject(instance.Configuration)
	if err != nil {
		return err
	}
	settings, _ := config["settings"].(map[string]any)
	if settings == nil {
		settings = make(map[string]any)
	}
	settings["configuration"] = map[string]any{"provider": instance.Provider, "engine": instance.Engine, "source": instance.Source}
	config["settings"] = settings
	config["dependencies"] = instanceDependencies(instance)
	definition := definitions[address.Stack]
	if definition == nil {
		definition = map[string]any{"name": address.Stack, componentsKey: map[string]any{terraformType: map[string]any{}}}
		definitions[address.Stack] = definition
	}
	definition[componentsKey].(map[string]any)[terraformType].(map[string]any)[address.Component] = config
	return nil
}

func checkDependencies(instances map[[3]string]*pluginv1.DiscoveredComponent) error {
	for _, instance := range instances {
		for _, dep := range instance.Dependencies {
			if _, exists := instances[addressKey(dep.Target)]; !exists {
				return fmt.Errorf("%w: dependency %s/%s is outside discovery scope", errUtils.ErrPluginInvalidConfig, dep.Target.Stack, dep.Target.Component)
			}
		}
	}
	return nil
}

func instanceDependencies(instance *pluginv1.DiscoveredComponent) map[string]any {
	deps := map[string]any{"files": []any{}, "folders": []any{}, componentsKey: []any{}}
	for _, source := range instance.Sources {
		field := "files"
		if source.Kind == pluginv1.ComponentSource_DIRECTORY {
			field = "folders"
		}
		deps[field] = append(deps[field].([]any), source.Path)
	}
	for _, dependency := range instance.Dependencies {
		deps[componentsKey] = append(deps[componentsKey].([]any), map[string]any{"name": dependency.Target.Component, "stack": dependency.Target.Stack, "kind": dependency.Target.ComponentType})
	}
	return deps
}

func addDiscovered(definitions map[string]map[string]any, instances map[[3]string]*pluginv1.DiscoveredComponent, discovered *pluginv1.DiscoverComponentsResponse) error {
	for _, instance := range discovered.Components {
		key := addressKey(instance.Address)
		if _, exists := instances[key]; exists {
			return errUtils.ErrDuplicatePluginExtension
		}
		if err := addInstance(definitions, instance); err != nil {
			return err
		}
		instances[key] = instance
	}
	return nil
}
