package exec

import (
	"fmt"
	"strings"

	"github.com/go-viper/mapstructure/v2"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// findDependentsFromIndex uses the pre-computed dependency index for O(1) lookup.
func findDependentsFromIndex(atmosConfig *schema.AtmosConfiguration, args *DescribeDependentsArgs, providedComponentVars *schema.Context, targetUnavailable bool) ([]schema.Dependent, error) {
	return findDependentsFromIndexWithStacks(atmosConfig, args, providedComponentVars, targetUnavailable, nil)
}

func findDependentsFromIndexWithStacks(atmosConfig *schema.AtmosConfiguration, args *DescribeDependentsArgs, providedComponentVars *schema.Context, targetUnavailable bool, stacks map[string]any) ([]schema.Dependent, error) {
	var dependents []schema.Dependent

	entries := args.DepIndex[args.Component]
	for i := range entries {
		e := &entries[i]

		// Skip self-references.
		if e.StackName == args.Stack && e.StackComponentName == args.Component {
			continue
		}

		dep := e.DependsOn
		unavailable, err := dependencyTargetUnavailable(&dependencyTargetParams{
			stacks:      stacks,
			args:        args,
			dep:         &dep,
			sourceStack: e.StackName,
			sourceType:  e.StackComponentType,
			fallback:    targetUnavailable,
		})
		if err != nil {
			return nil, err
		}
		if unavailable {
			continue
		}
		if !isDependencyMatch(&dependencyMatchParams{
			depSource:             e.DepSource,
			dependsOn:             &dep,
			args:                  args,
			stackName:             e.StackName,
			providedComponentVars: providedComponentVars,
			stackComponentVars:    &e.StackComponentVars,
		}) {
			continue
		}

		dependents = append(dependents, buildDependentEntry(atmosConfig, args, e))
	}

	return dependents, nil
}

// findDependentsByScan falls back to the full O(stacks * components) scan.
func findDependentsByScan(atmosConfig *schema.AtmosConfiguration, args *DescribeDependentsArgs, stacks map[string]any, providedComponentVars *schema.Context, targetUnavailable bool) ([]schema.Dependent, error) {
	var dependents []schema.Dependent

	for stackName, stackSection := range stacks {
		stackSectionMap, ok := stackSection.(map[string]any)
		if !ok {
			continue
		}
		stackComponentsSection, ok := stackSectionMap["components"].(map[string]any)
		if !ok {
			continue
		}

		for stackComponentType, stackComponentTypeSection := range stackComponentsSection {
			stackComponentTypeSectionMap, ok := stackComponentTypeSection.(map[string]any)
			if !ok {
				continue
			}

			for stackComponentName, stackComponent := range stackComponentTypeSectionMap {
				deps, err := scanComponentForDependents(&scanComponentParams{
					AtmosConfig:           atmosConfig,
					Args:                  args,
					Stacks:                stacks,
					StackName:             stackName,
					StackComponentType:    stackComponentType,
					StackComponentName:    stackComponentName,
					StackComponent:        stackComponent,
					ProvidedComponentVars: providedComponentVars,
					TargetUnavailable:     targetUnavailable,
				})
				if err != nil {
					return nil, err
				}
				dependents = append(dependents, deps...)
			}
		}
	}

	return dependents, nil
}

// scanComponentParams groups parameters for scanComponentForDependents.
type scanComponentParams struct {
	AtmosConfig           *schema.AtmosConfiguration
	Args                  *DescribeDependentsArgs
	Stacks                map[string]any
	StackName             string
	StackComponentType    string
	StackComponentName    string
	StackComponent        any
	ProvidedComponentVars *schema.Context
	TargetUnavailable     bool
}

// scanComponentForDependents checks a single component for dependencies on the provided component.
func scanComponentForDependents(p *scanComponentParams) ([]schema.Dependent, error) {
	stackComponentMap, ok := p.StackComponent.(map[string]any)
	if !ok {
		return nil, nil
	}

	if p.StackComponentName == p.Args.Component && (p.Args.Stack == "" || p.StackName == p.Args.Stack) {
		return nil, nil
	}
	if isAbstractOrDisabled(stackComponentMap, p.StackComponentName) {
		return nil, nil
	}

	stackComponentVarsSection, ok := stackComponentMap["vars"].(map[string]any)
	if !ok {
		return nil, nil
	}

	var stackComponentVars schema.Context
	if err := mapstructure.Decode(stackComponentVarsSection, &stackComponentVars); err != nil {
		return nil, fmt.Errorf("decode vars for component %q in stack %q: %w", p.StackComponentName, p.StackName, err)
	}

	result, err := getComponentDependenciesWithError(stackComponentMap)
	if err != nil {
		return nil, fmt.Errorf("parse dependencies for component %q in stack %q: %w", p.StackComponentName, p.StackName, err)
	}
	componentDeps, settingsSection, depSource := result.dependencies, result.settingsSection, result.source
	if len(componentDeps) == 0 {
		return nil, nil
	}

	return scanComponentDependencies(p, stackComponentMap, stackComponentVarsSection, &stackComponentVars, componentDependenciesResult{
		dependencies:    componentDeps,
		settingsSection: settingsSection,
		source:          depSource,
	})
}

func scanComponentDependencies(p *scanComponentParams, stackComponentMap, stackComponentVarsSection map[string]any, stackComponentVars *schema.Context, result componentDependenciesResult) ([]schema.Dependent, error) {
	var dependents []schema.Dependent
	for depIdx := range result.dependencies {
		dependsOn := &result.dependencies[depIdx]
		if dependsOn.Component != p.Args.Component {
			continue
		}
		unavailable, err := dependencyTargetUnavailable(&dependencyTargetParams{
			stacks:      p.Stacks,
			args:        p.Args,
			dep:         dependsOn,
			sourceStack: p.StackName,
			sourceType:  p.StackComponentType,
			fallback:    p.TargetUnavailable,
		})
		if err != nil {
			return nil, err
		}
		if unavailable {
			continue
		}
		if !isDependencyMatch(&dependencyMatchParams{
			depSource:             result.source,
			dependsOn:             dependsOn,
			args:                  p.Args,
			stackName:             p.StackName,
			providedComponentVars: p.ProvidedComponentVars,
			stackComponentVars:    stackComponentVars,
		}) {
			continue
		}

		e := &dependencyIndexEntry{
			StackName:                 p.StackName,
			StackComponentName:        p.StackComponentName,
			StackComponentType:        p.StackComponentType,
			StackComponentMap:         stackComponentMap,
			StackComponentVarsSection: stackComponentVarsSection,
			StackComponentVars:        *stackComponentVars,
			SettingsSection:           result.settingsSection,
		}
		dependents = append(dependents, buildDependentEntry(p.AtmosConfig, p.Args, e))
	}

	return dependents, nil
}

type dependencyTargetParams struct {
	stacks      map[string]any
	args        *DescribeDependentsArgs
	dep         *schema.ComponentDependency
	sourceStack string
	sourceType  string
	fallback    bool
}

func dependencyTargetUnavailable(p *dependencyTargetParams) (bool, error) {
	if !dependencyTargetsStackValues(p.dep, p.sourceStack, p.args.Stack) {
		return false, nil
	}
	if p.stacks == nil {
		return unavailableDependencyTarget(p.dep, p.fallback, p.dep.Stack, p.dep.Kind, errUtils.ErrDependencyTargetUnavailable)
	}

	targetStack := p.dep.Stack
	if targetStack == "" {
		targetStack = p.sourceStack
	}
	targetType := p.dep.Kind
	if targetType == "" {
		targetType = p.sourceType
	}
	target := findComponentSectionInCachedStacksByType(p.stacks, targetStack, p.args.Component, targetType)
	if target == nil {
		return unavailableDependencyTarget(p.dep, true, targetStack, targetType, errUtils.ErrDependencyTargetNotFound)
	}
	return unavailableDependencyTarget(p.dep, isAbstractOrDisabled(target, p.args.Component), targetStack, targetType, errUtils.ErrDependencyTargetUnavailable)
}

func unavailableDependencyTarget(dep *schema.ComponentDependency, unavailable bool, targetStack, targetType string, targetErr error) (bool, error) {
	if !unavailable {
		return false, nil
	}
	if !dep.IsRequired() {
		return true, nil
	}
	return false, fmt.Errorf("%w: component %q of kind %q in stack %q", targetErr, dep.Component, targetType, targetStack)
}

// buildDependentEntry constructs a Dependent struct from a dependency index entry.
func buildDependentEntry(atmosConfig *schema.AtmosConfiguration, args *DescribeDependentsArgs, e *dependencyIndexEntry) schema.Dependent {
	dependent := schema.Dependent{
		Component:     e.StackComponentName,
		ComponentPath: BuildComponentPath(atmosConfig, &e.StackComponentMap, e.StackComponentType),
		ComponentType: e.StackComponentType,
		Stack:         e.StackName,
		StackSlug:     fmt.Sprintf("%s-%s", e.StackName, strings.ReplaceAll(e.StackComponentName, "/", "-")),
		Namespace:     e.StackComponentVars.Namespace,
		Tenant:        e.StackComponentVars.Tenant,
		Environment:   e.StackComponentVars.Environment,
		Stage:         e.StackComponentVars.Stage,
	}

	if e.StackComponentType == "terraform" {
		configAndStacksInfo := schema.ConfigAndStacksInfo{
			ComponentFromArg:         e.StackComponentName,
			Stack:                    e.StackName,
			ComponentVarsSection:     e.StackComponentVarsSection,
			ComponentSettingsSection: e.SettingsSection,
			ComponentSection: map[string]any{
				cfg.VarsSectionName:     e.StackComponentVarsSection,
				cfg.SettingsSectionName: e.SettingsSection,
			},
		}

		if spaceliftStackName, err := BuildSpaceliftStackNameFromComponentConfig(atmosConfig, configAndStacksInfo); err == nil {
			dependent.SpaceliftStack = spaceliftStackName
		}
		if atlantisProjectName, err := BuildAtlantisProjectNameFromComponentConfig(atmosConfig, configAndStacksInfo); err == nil {
			dependent.AtlantisProject = atlantisProjectName
		}
	}

	if args.IncludeSettings {
		dependent.Settings = e.SettingsSection
	}

	return dependent
}
