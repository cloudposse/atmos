package exec

import (
	comp "github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
)

// findComponentSectionInCachedStacks extracts a component section from pre-computed stacks.
// Returns nil if the stack or component is not found (caller falls back to ExecuteDescribeComponent).
func findComponentSectionInCachedStacks(stacks map[string]any, stackName, componentName string) map[string]any {
	stackSection, ok := stacks[stackName].(map[string]any)
	if !ok {
		return nil
	}
	componentsSection, ok := stackSection["components"].(map[string]any)
	if !ok {
		return nil
	}

	for _, componentType := range componentSectionSearchOrder() {
		typeSection, ok := componentsSection[componentType].(map[string]any)
		if !ok {
			continue
		}
		if compSection, ok := typeSection[componentName].(map[string]any); ok {
			return compSection
		}
	}
	return nil
}

// componentSectionSearchOrder returns every component-type section name to search when
// resolving a dependency's component config. Combines the legacy types that predate the
// component-provider registry (terraform/helmfile/packer, never registered via
// component.Register) with every dynamically registered provider type (helm, kubernetes,
// ansible, container, emulator, and any future type such as aws/cloudformation), so a new
// registered component type needs no additional touch here.
func componentSectionSearchOrder() []string {
	types := []string{cfg.TerraformComponentType, cfg.HelmfileComponentType, cfg.PackerComponentType}
	types = append(types, comp.ListTypes()...)
	return types
}
