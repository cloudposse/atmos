package exec

import (
	"slices"
	"sort"

	comp "github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
)

// findComponentSectionInCachedStacks extracts a component section from pre-computed stacks.
// Returns nil if the stack or component is not found (caller falls back to ExecuteDescribeComponent).
func findComponentSectionInCachedStacks(stacks map[string]any, stackName, componentName string) map[string]any {
	component, _ := findComponentSectionInCachedStacksWithType(stacks, stackName, componentName)
	return component
}

func findComponentSectionInCachedStacksWithType(stacks map[string]any, stackName, componentName string) (map[string]any, string) {
	stackSection, ok := stacks[stackName].(map[string]any)
	if !ok {
		return nil, ""
	}
	componentsSection, ok := stackSection["components"].(map[string]any)
	if !ok {
		return nil, ""
	}
	componentTypes := componentSectionSearchOrder()
	// Match the same configured component-type precedence as describe component.
	// Unknown component types are considered afterward in lexical order.
	for _, componentType := range componentTypes {
		if comp := findComponentSectionInCachedStacksByType(stacks, stackName, componentName, componentType); comp != nil {
			return comp, componentType
		}
	}
	knownTypes := make(map[string]struct{}, len(componentTypes))
	for _, componentType := range componentTypes {
		knownTypes[componentType] = struct{}{}
	}
	remainingTypes := make([]string, 0, len(componentsSection))
	for componentType := range componentsSection {
		if _, ok := knownTypes[componentType]; !ok {
			remainingTypes = append(remainingTypes, componentType)
		}
	}
	sort.Strings(remainingTypes)
	for _, componentType := range remainingTypes {
		if comp := findComponentSectionInCachedStacksByType(stacks, stackName, componentName, componentType); comp != nil {
			return comp, componentType
		}
	}
	return nil, ""
}

func findComponentSectionInCachedStacksByType(stacks map[string]any, stackName, componentName, componentType string) map[string]any {
	stackSection, ok := stacks[stackName].(map[string]any)
	if !ok {
		return nil
	}
	componentsSection, ok := stackSection["components"].(map[string]any)
	if !ok {
		return nil
	}
	componentTypeMap, ok := componentsSection[componentType].(map[string]any)
	if !ok {
		return nil
	}
	component, _ := componentTypeMap[componentName].(map[string]any)
	return component
}

// componentSectionSearchOrder returns every component-type section name to search when
// resolving a dependency's component config. Combines the legacy types that predate the
// component-provider registry (terraform/helmfile/packer, never registered via
// component.Register) with every dynamically registered provider type (helm, kubernetes,
// ansible, container, emulator, and any future type such as aws/cloudformation), so a new
// registered component type needs no additional touch here.
func componentSectionSearchOrder() []string {
	types := []string{
		cfg.TerraformComponentType, cfg.HelmfileComponentType, cfg.PackerComponentType,
		cfg.AnsibleComponentType, cfg.ContainerComponentType, cfg.EmulatorComponentType,
		cfg.KubernetesComponentType, cfg.HelmComponentType,
	}
	for _, componentType := range comp.ListTypes() {
		if !slices.Contains(types, componentType) {
			types = append(types, componentType)
		}
	}
	return types
}
