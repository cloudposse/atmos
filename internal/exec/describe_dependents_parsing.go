package exec

import (
	"fmt"

	"github.com/go-viper/mapstructure/v2"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// dependencySource indicates where dependencies were loaded from.
type dependencySource int

const (
	dependencySourceNone dependencySource = iota
	dependencySourceDependenciesComponents
	dependencySourceSettingsDependsOn
)

type componentDependenciesResult struct {
	dependencies    []schema.ComponentDependency
	settingsSection map[string]any
	source          dependencySource
}

// getComponentDependencies keeps the historical test-facing shape.
func getComponentDependencies(componentMap map[string]any) ([]schema.ComponentDependency, map[string]any, dependencySource) {
	result, _ := getComponentDependenciesWithError(componentMap)
	return result.dependencies, result.settingsSection, result.source
}

// getComponentDependenciesWithError extracts component dependencies and returns parse failures.
func getComponentDependenciesWithError(componentMap map[string]any) (componentDependenciesResult, error) {
	// Get settings section for later use (Spacelift/Atlantis config and IncludeSettings).
	settingsSection, _ := componentMap["settings"].(map[string]any)

	dependenciesValue, exists := componentMap[cfg.DependenciesSectionName]
	depsSection, ok := dependenciesValue.(map[string]any)
	if exists && !ok {
		return componentDependenciesResult{settingsSection: settingsSection}, fmt.Errorf("%w: %s must be a map", errUtils.ErrInvalidDependenciesSection, cfg.DependenciesSectionName)
	}
	if _, hasComponents := depsSection["components"]; ok && hasComponents {
		componentDeps, err := schema.ParseComponentDependencies(depsSection, "", "")
		if err != nil {
			return componentDependenciesResult{settingsSection: settingsSection}, err
		}
		componentDeps = filterComponentDependencies(componentDeps)
		if len(componentDeps) > 0 {
			return componentDependenciesResult{
				dependencies:    componentDeps,
				settingsSection: settingsSection,
				source:          dependencySourceDependenciesComponents,
			}, nil
		}
	}

	if deps, source, found := getLegacyComponentDependencies(componentMap, settingsSection); found {
		return componentDependenciesResult{dependencies: deps, settingsSection: settingsSection, source: source}, nil
	}

	return componentDependenciesResult{settingsSection: settingsSection}, nil
}

func getLegacyComponentDependencies(componentMap, settingsSection map[string]any) ([]schema.ComponentDependency, dependencySource, bool) {
	if settingsSection != nil {
		var settings schema.Settings
		if err := mapstructure.Decode(settingsSection, &settings); err == nil && len(settings.DependsOn) > 0 {
			log.Debug("'settings.depends_on' is deprecated, use 'dependencies.components' instead. See: https://atmos.tools/stacks/dependencies/components")
			deps := make([]schema.ComponentDependency, 0, len(settings.DependsOn))
			for key := range settings.DependsOn {
				ctx := settings.DependsOn[key]
				deps = append(deps, contextToComponentDependency(&ctx))
			}
			return deps, dependencySourceSettingsDependsOn, true
		}
	}

	if directDependsOn, ok := componentMap["depends_on"]; ok {
		var settings schema.Settings
		if err := mapstructure.Decode(map[string]any{"depends_on": directDependsOn}, &settings); err == nil && len(settings.DependsOn) > 0 {
			log.Debug("component depends_on is deprecated, use dependencies.components instead")
			deps := make([]schema.ComponentDependency, 0, len(settings.DependsOn))
			for key := range settings.DependsOn {
				ctx := settings.DependsOn[key]
				deps = append(deps, contextToComponentDependency(&ctx))
			}
			return deps, dependencySourceSettingsDependsOn, true
		}
	}

	return nil, dependencySourceNone, false
}

// filterComponentDependencies removes file/folder path dependencies from the
// dependents path. Those entries affect `describe affected`, but they are not
// component-to-component relationships and must not suppress settings.depends_on
// fallback during mixed migrations.
func filterComponentDependencies(deps []schema.ComponentDependency) []schema.ComponentDependency {
	if len(deps) == 0 {
		return nil
	}

	result := make([]schema.ComponentDependency, 0, len(deps))
	for i := range deps {
		if !deps[i].IsComponentDependency() || deps[i].Component == "" {
			continue
		}
		result = append(result, deps[i])
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// contextToComponentDependency converts a legacy schema.Context to schema.ComponentDependency.
// This is used to support the deprecated settings.depends_on format.
// All context fields are preserved for matching logic.
func contextToComponentDependency(ctx *schema.Context) schema.ComponentDependency {
	return schema.ComponentDependency{
		Component:   ctx.Component,
		Stack:       ctx.Stack,
		Namespace:   ctx.Namespace,
		Tenant:      ctx.Tenant,
		Environment: ctx.Environment,
		Stage:       ctx.Stage,
	}
}
