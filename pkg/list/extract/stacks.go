package extract

import (
	"fmt"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/tags"
)

// Stacks transforms stacksMap into structured stack data.
// It returns []map[string]any suitable for the renderer pipeline.
// Exposes vars from components for template access (e.g., {{ .vars.namespace }}).
// When tagsFilter/labelsFilter are non-empty, only stacks where at least one
// component independently satisfies both filters (tags: any-match, labels:
// all-match) are returned — union semantics across the stack's components.
func Stacks(stacksMap map[string]any, tagsFilter []string, labelsFilter map[string]string) ([]map[string]any, error) {
	defer perf.Track(nil, "extract.Stacks")()

	if stacksMap == nil {
		return nil, errUtils.ErrStackNotFound
	}

	var stacks []map[string]any

	for stackName, stackData := range stacksMap {
		stackMap, isMap := stackData.(map[string]any)
		if isMap && !stackMatchesAnyComponent(stackMap, tagsFilter, labelsFilter) {
			continue
		}
		if !isMap && (len(tagsFilter) > 0 || len(labelsFilter) > 0) {
			// A stack without component data can never match an active filter.
			continue
		}

		stack := map[string]any{
			"stack": stackName,
		}

		// Extract vars from stack data.
		if isMap {
			extractStackVars(stack, stackMap)
		}

		stacks = append(stacks, stack)
	}

	return stacks, nil
}

// stackMatchesAnyComponent reports whether at least one component in stackMap
// (across every registered component type) independently satisfies both the
// tags (any-match) and labels (all-match) filters. Empty filters always match.
func stackMatchesAnyComponent(stackMap map[string]any, tagsFilter []string, labelsFilter map[string]string) bool {
	defer perf.Track(nil, "extract.stackMatchesAnyComponent")()

	if len(tagsFilter) == 0 && len(labelsFilter) == 0 {
		return true
	}

	componentsMap, ok := stackMap["components"].(map[string]any)
	if !ok {
		return false
	}

	for _, componentType := range getComponentTypes() {
		typeComponents, ok := componentsMap[componentType].(map[string]any)
		if !ok {
			continue
		}
		for _, componentData := range typeComponents {
			if componentMatchesTagsLabels(componentData, tagsFilter, labelsFilter) {
				return true
			}
		}
	}

	return false
}

// componentMatchesTagsLabels reports whether a single component's
// metadata.tags/metadata.labels satisfy the filters (tags: any-match, labels:
// all-match). Empty filters always match.
func componentMatchesTagsLabels(componentData any, tagsFilter []string, labelsFilter map[string]string) bool {
	if len(tagsFilter) == 0 && len(labelsFilter) == 0 {
		return true
	}

	compMap, ok := componentData.(map[string]any)
	if !ok {
		return false
	}
	metadata, _ := compMap[fieldMetadata].(map[string]any)
	compTags := getStringSliceFromMetadata(metadata, fieldTags)
	compLabels := getStringMapFromMetadata(metadata, fieldLabels)
	return tags.MatchesTags(compTags, tagsFilter, tags.TagModeAny) && tags.MatchesLabels(compLabels, labelsFilter)
}

// extractStackVars extracts vars from components and exposes them for template access.
// Vars are found within components (stackMap["components"]["terraform"]["<component>"]["vars"]).
// Templates can access any var via {{ .vars.fieldname }}.
func extractStackVars(stack map[string]any, stackMap map[string]any) {
	defer perf.Track(nil, "extract.extractStackVars")()

	// Try to get vars from any component in the stack.
	// Structure: stackMap["components"]["terraform"]["<component>"]["vars"]
	vars := findVarsFromComponents(stackMap)
	if vars == nil {
		// Set empty vars map if not found.
		stack["vars"] = map[string]any{}
		return
	}

	// Expose full vars for template access (e.g., {{ .vars.namespace }}).
	stack["vars"] = vars
}

// findVarsFromComponents finds vars from the first component in the stack.
// Components structure: stackMap["components"]["<type>"]["<component>"]["vars"].
// Checks built-in component types (terraform, helmfile, packer) plus any
// additional types registered in the component registry.
func findVarsFromComponents(stackMap map[string]any) map[string]any {
	defer perf.Track(nil, "extract.findVarsFromComponents")()

	componentsMap, ok := stackMap["components"].(map[string]any)
	if !ok {
		return nil
	}

	// Get all component types to check.
	componentTypes := getComponentTypes()

	// Check each component type.
	for _, componentType := range componentTypes {
		typeComponents, ok := componentsMap[componentType].(map[string]any)
		if !ok {
			continue
		}

		// Sort component names for deterministic selection.
		componentNames := make([]string, 0, len(typeComponents))
		for name := range typeComponents {
			componentNames = append(componentNames, name)
		}
		sort.Strings(componentNames)

		// Get vars from the first component found (alphabetically).
		for _, name := range componentNames {
			componentMap, ok := typeComponents[name].(map[string]any)
			if !ok {
				continue
			}
			if vars, ok := componentMap["vars"].(map[string]any); ok {
				return vars
			}
		}
	}

	return nil
}

// getComponentTypes returns all component types to check.
// Includes built-in types (terraform, helmfile, packer, ansible) plus any
// additional types registered in the component registry.
func getComponentTypes() []string {
	defer perf.Track(nil, "extract.getComponentTypes")()

	// Start with built-in component types.
	// These are the standard types defined in pkg/config/const.go.
	typeSet := map[string]struct{}{
		config.TerraformComponentType: {},
		config.HelmfileComponentType:  {},
		config.PackerComponentType:    {},
		config.AnsibleComponentType:   {},
	}

	// Add any additional types from the component registry.
	// This supports custom component types registered via plugins.
	for _, t := range component.ListTypes() {
		typeSet[t] = struct{}{}
	}

	// Convert set to sorted slice for deterministic iteration.
	types := make([]string, 0, len(typeSet))
	for t := range typeSet {
		types = append(types, t)
	}
	sort.Strings(types)

	return types
}

// StacksForComponent extracts stacks that contain a specific component.
// When tagsFilter/labelsFilter are non-empty, only stacks where that specific
// component's own metadata.tags/metadata.labels satisfy the filters are
// returned. ErrNoStacksFound is reserved for "component not found in any
// stack"; a component that exists but matches no filter yields an empty
// result with no error.
func StacksForComponent(componentName string, stacksMap map[string]any, tagsFilter []string, labelsFilter map[string]string) ([]map[string]any, error) {
	defer perf.Track(nil, "extract.StacksForComponent")()

	if stacksMap == nil {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrStackNotFound, componentName)
	}

	// Get all component types to check.
	componentTypes := getComponentTypes()

	var stacks []map[string]any
	componentFound := false

	for stackName, stackData := range stacksMap {
		found, matched := findComponentInStack(stackData, componentName, componentTypes, tagsFilter, labelsFilter)
		if found {
			componentFound = true
		}
		if matched {
			stack := map[string]any{
				"stack":     stackName,
				"component": componentName,
			}
			stacks = append(stacks, stack)
		}
	}

	if !componentFound {
		return nil, errUtils.ErrNoStacksFound
	}

	return stacks, nil
}

// findComponentInStack reports whether componentName exists in the stack
// (found) and, when it does, whether its own metadata satisfies the
// tags/labels filters (matched).
func findComponentInStack(stackData any, componentName string, componentTypes []string, tagsFilter []string, labelsFilter map[string]string) (found, matched bool) {
	stackMap, ok := stackData.(map[string]any)
	if !ok {
		return false, false
	}

	componentsMap, ok := stackMap["components"].(map[string]any)
	if !ok {
		return false, false
	}

	for _, componentType := range componentTypes {
		typeComponents, ok := componentsMap[componentType].(map[string]any)
		if !ok {
			continue
		}
		componentData, exists := typeComponents[componentName]
		if !exists {
			continue
		}
		found = true
		if componentMatchesTagsLabels(componentData, tagsFilter, labelsFilter) {
			return true, true
		}
	}

	return found, false
}
