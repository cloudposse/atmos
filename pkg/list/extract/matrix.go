package extract

import (
	"sort"

	"github.com/cloudposse/atmos/pkg/matrix"
	"github.com/cloudposse/atmos/pkg/perf"
)

// StacksMatrixEntries extracts all stack+component pairs from stacksMap as matrix entries.
// Each entry includes stack, component, component_path, and component_type — matching
// the format used by describe affected --format=matrix.
func StacksMatrixEntries(stacksMap map[string]any) []matrix.Entry {
	defer perf.Track(nil, "extract.StacksMatrixEntries")()

	if stacksMap == nil {
		return nil
	}

	componentTypes := getComponentTypes()
	var entries []matrix.Entry

	for _, stackName := range sortedKeys(stacksMap) {
		stackEntries := stackMatrixEntries(stackName, stacksMap[stackName], componentTypes)
		entries = append(entries, stackEntries...)
	}

	return entries
}

// stackMatrixEntries extracts matrix entries for a single stack.
func stackMatrixEntries(stackName string, stackData any, componentTypes []string) []matrix.Entry {
	stackMap, ok := stackData.(map[string]any)
	if !ok {
		return nil
	}

	componentsMap, ok := stackMap["components"].(map[string]any)
	if !ok {
		return nil
	}

	var entries []matrix.Entry
	for _, componentType := range componentTypes {
		typeComponents, ok := componentsMap[componentType].(map[string]any)
		if !ok {
			continue
		}
		entries = append(entries, componentTypeEntries(stackName, componentType, typeComponents)...)
	}
	return entries
}

// componentTypeEntries extracts matrix entries for all components of a single type within a stack.
func componentTypeEntries(stackName, componentType string, typeComponents map[string]any) []matrix.Entry {
	entries := make([]matrix.Entry, 0, len(typeComponents))
	for _, componentName := range sortedKeys(typeComponents) {
		componentData, ok := typeComponents[componentName].(map[string]any)
		if !ok {
			continue
		}
		entries = append(entries, matrix.Entry{
			Stack:         stackName,
			Component:     componentName,
			ComponentType: componentType,
			ComponentPath: extractComponentPath(componentData),
		})
	}
	return entries
}

// extractComponentPath reads component_info.component_path from a component data map.
func extractComponentPath(componentData map[string]any) string {
	componentInfo, ok := componentData["component_info"].(map[string]any)
	if !ok {
		return ""
	}
	path, _ := componentInfo["component_path"].(string)
	return path
}

// sortedKeys returns the keys of a map[string]any sorted alphabetically.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
