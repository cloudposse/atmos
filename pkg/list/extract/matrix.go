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

	// Sort stack names for deterministic output.
	stackNames := make([]string, 0, len(stacksMap))
	for stackName := range stacksMap {
		stackNames = append(stackNames, stackName)
	}
	sort.Strings(stackNames)

	for _, stackName := range stackNames {
		stackData := stacksMap[stackName]
		stackMap, ok := stackData.(map[string]any)
		if !ok {
			continue
		}

		componentsMap, ok := stackMap["components"].(map[string]any)
		if !ok {
			continue
		}

		for _, componentType := range componentTypes {
			typeComponents, ok := componentsMap[componentType].(map[string]any)
			if !ok {
				continue
			}

			// Sort component names for deterministic output.
			componentNames := make([]string, 0, len(typeComponents))
			for name := range typeComponents {
				componentNames = append(componentNames, name)
			}
			sort.Strings(componentNames)

			for _, componentName := range componentNames {
				componentData, ok := typeComponents[componentName].(map[string]any)
				if !ok {
					continue
				}

				entry := matrix.Entry{
					Stack:         stackName,
					Component:     componentName,
					ComponentType: componentType,
				}

				// Extract component_path from component_info.
				if componentInfo, ok := componentData["component_info"].(map[string]any); ok {
					if path, ok := componentInfo["component_path"].(string); ok {
						entry.ComponentPath = path
					}
				}

				entries = append(entries, entry)
			}
		}
	}

	return entries
}

// StacksMatrixEntriesForComponent extracts matrix entries filtered by a specific component name.
func StacksMatrixEntriesForComponent(componentName string, stacksMap map[string]any) []matrix.Entry {
	defer perf.Track(nil, "extract.StacksMatrixEntriesForComponent")()

	if stacksMap == nil {
		return nil
	}

	componentTypes := getComponentTypes()
	var entries []matrix.Entry

	// Sort stack names for deterministic output.
	stackNames := make([]string, 0, len(stacksMap))
	for stackName := range stacksMap {
		stackNames = append(stackNames, stackName)
	}
	sort.Strings(stackNames)

	for _, stackName := range stackNames {
		stackData := stacksMap[stackName]
		stackMap, ok := stackData.(map[string]any)
		if !ok {
			continue
		}

		componentsMap, ok := stackMap["components"].(map[string]any)
		if !ok {
			continue
		}

		for _, componentType := range componentTypes {
			typeComponents, ok := componentsMap[componentType].(map[string]any)
			if !ok {
				continue
			}

			componentData, ok := typeComponents[componentName].(map[string]any)
			if !ok {
				continue
			}

			entry := matrix.Entry{
				Stack:         stackName,
				Component:     componentName,
				ComponentType: componentType,
			}

			if componentInfo, ok := componentData["component_info"].(map[string]any); ok {
				if path, ok := componentInfo["component_path"].(string); ok {
					entry.ComponentPath = path
				}
			}

			entries = append(entries, entry)
		}
	}

	return entries
}
