package exec

import (
	"github.com/go-viper/mapstructure/v2"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// dependencyIndexEntry holds pre-parsed component data for dependency lookups.
type dependencyIndexEntry struct {
	StackName                 string
	StackComponentName        string
	StackComponentType        string
	StackComponentMap         map[string]any
	StackComponentVarsSection map[string]any
	StackComponentVars        schema.Context
	SettingsSection           map[string]any
	DepSource                 dependencySource
	DependsOn                 schema.ComponentDependency
}

// dependencyIndex maps a component name to all (stack, component) pairs that depend on it.
// Built once from the stacks data, it eliminates the O(N*M) scan in ExecuteDescribeDependents.
type dependencyIndex map[string][]dependencyIndexEntry

// buildDependencyIndex pre-parses all components across all stacks and builds a reverse
// dependency index: for each component name X, the index contains all entries where some
// component in some stack declares a dependency on X.
func buildDependencyIndex(stacks map[string]any) dependencyIndex {
	idx := make(dependencyIndex)

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
				indexComponentDependencies(
					idx, stackName, stackComponentType, stackComponentName, stackComponent,
				)
			}
		}
	}

	return idx
}

// indexComponentDependencies parses a single component and adds its dependencies to the index.
func indexComponentDependencies(
	idx dependencyIndex,
	stackName, stackComponentType, stackComponentName string,
	stackComponent any,
) {
	stackComponentMap, ok := stackComponent.(map[string]any)
	if !ok {
		return
	}

	if isAbstractOrDisabled(stackComponentMap, stackComponentName) {
		return
	}

	stackComponentVarsSection, ok := stackComponentMap["vars"].(map[string]any)
	if !ok {
		return
	}

	var stackComponentVars schema.Context
	if err := mapstructure.Decode(stackComponentVarsSection, &stackComponentVars); err != nil {
		log.Debug("Failed to decode component vars during index build",
			"component", stackComponentName, "stack", stackName, "error", err)
		return
	}

	componentDeps, settingsSection, depSource := getComponentDependencies(stackComponentMap)
	if len(componentDeps) == 0 {
		return
	}

	for i := range componentDeps {
		dep := &componentDeps[i]
		if dep.Component == "" {
			continue
		}
		entry := dependencyIndexEntry{
			StackName:                 stackName,
			StackComponentName:        stackComponentName,
			StackComponentType:        stackComponentType,
			StackComponentMap:         stackComponentMap,
			StackComponentVarsSection: stackComponentVarsSection,
			StackComponentVars:        stackComponentVars,
			SettingsSection:           settingsSection,
			DepSource:                 depSource,
			DependsOn:                 *dep,
		}
		idx[dep.Component] = append(idx[dep.Component], entry)
	}
}

// isAbstractOrDisabled checks if a component should be skipped during indexing.
func isAbstractOrDisabled(componentMap map[string]any, componentName string) bool {
	metadataSection, ok := componentMap["metadata"].(map[string]any)
	if !ok {
		return false
	}
	if metadataType, ok := metadataSection["type"].(string); ok && metadataType == "abstract" {
		return true
	}
	return !isComponentEnabled(metadataSection, componentName)
}
