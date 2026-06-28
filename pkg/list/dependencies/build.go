// Package dependencies builds and renders the Atmos component dependency graph
// for the `atmos list dependencies` command. Unlike the execution-ordering graph
// used by the scheduler (which rejects cycles), this builder is cycle-tolerant so
// the command can visualize circular dependencies instead of failing on them.
package dependencies

import (
	"fmt"

	"github.com/go-viper/mapstructure/v2"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// NodeID returns the canonical, collision-safe node ID for a component in a
// stack. It uses a length-prefixed encoding so that component/stack names
// containing the delimiter character never produce the same ID for distinct
// (component, stack) pairs (e.g. "app-prod"+"us" vs "app"+"prod-us").
func NodeID(component, stack string) string {
	return fmt.Sprintf("%d:%s/%d:%s", len(component), component, len(stack), stack)
}

// BuildGraph constructs a cycle-tolerant dependency graph from the described
// stacks map. It adds a node for every concrete (non-abstract, enabled)
// terraform component and an edge for every component-to-component dependency
// declared via either `dependencies.components` (preferred) or the legacy
// `settings.depends_on`. Edges to targets that are not present in the graph
// (e.g. disabled or filtered-out components) are skipped.
func BuildGraph(stacks map[string]any) (*dependency.Graph, error) {
	defer perf.Track(nil, "dependencies.BuildGraph")()

	graph := dependency.NewGraph()

	// First pass: add all concrete component nodes.
	walkComponents(stacks, func(stackName, componentName string, componentSection map[string]any) {
		nodeID := NodeID(componentName, stackName)
		node := &dependency.Node{
			ID:        nodeID,
			Component: componentName,
			Stack:     stackName,
			Type:      cfg.TerraformComponentType,
			Metadata:  componentSection,
		}
		if err := graph.AddNode(node); err != nil {
			log.Debug("skipping node", "id", nodeID, "error", err)
		}
	})

	// Second pass: add dependency edges now that all nodes exist.
	walkComponents(stacks, func(stackName, componentName string, componentSection map[string]any) {
		fromID := NodeID(componentName, stackName)
		deps := extractComponentDependencies(componentSection)
		for i := range deps {
			dep := &deps[i]
			targetStack := stackName
			if dep.Stack != "" {
				targetStack = dep.Stack
			}
			toID := NodeID(dep.Component, targetStack)
			if _, exists := graph.GetNode(toID); !exists {
				log.Debug("dependency target not in graph", "from", fromID, "to", toID)
				continue
			}
			// AddDependency on the Graph (not the validating Builder) tolerates
			// cycles so they can be visualized rather than rejected.
			if err := graph.AddDependency(fromID, toID); err != nil {
				log.Debug("skipping dependency", "from", fromID, "to", toID, "error", err)
			}
		}
	})

	graph.IdentifyRoots()
	return graph, nil
}

// walkComponents iterates over every concrete terraform component in the stacks
// map, skipping abstract and disabled components. It mirrors the traversal used
// by the execution graph builder (internal/exec.walkTerraformComponents).
func walkComponents(stacks map[string]any, fn func(stackName, componentName string, componentSection map[string]any)) {
	for stackName, stackSection := range stacks {
		stackSectionMap, ok := stackSection.(map[string]any)
		if !ok {
			continue
		}
		componentsSection, ok := stackSectionMap[cfg.ComponentsSectionName].(map[string]any)
		if !ok {
			continue
		}
		terraformSection, ok := componentsSection[cfg.TerraformSectionName].(map[string]any)
		if !ok {
			continue
		}
		for componentName, compSection := range terraformSection {
			componentSection, ok := compSection.(map[string]any)
			if !ok {
				continue
			}
			if shouldSkipComponent(componentSection) {
				continue
			}
			fn(stackName, componentName, componentSection)
		}
	}
}

// shouldSkipComponent reports whether a component is abstract or disabled and
// therefore should not appear in the dependency graph.
func shouldSkipComponent(componentSection map[string]any) bool {
	metadataSection, ok := componentSection[cfg.MetadataSectionName].(map[string]any)
	if !ok {
		return false
	}
	if metadataType, ok := metadataSection["type"].(string); ok && metadataType == "abstract" {
		return true
	}
	if enabled, ok := metadataSection["enabled"].(bool); ok && !enabled {
		return true
	}
	return false
}

// extractComponentDependencies returns the component-to-component dependencies
// declared by a component, reading from `dependencies.components` first
// (preferred) and falling back to legacy `settings.depends_on` only when the
// `dependencies.components` key is entirely absent. An explicitly empty
// `dependencies.components: []` is treated as authoritative and clears all
// edges (no fallback to settings). File and folder dependencies are
// intentionally excluded — they are not component edges. This mirrors
// getComponentDependencies in internal/exec/describe_dependents.go so
// `list dependencies` and `describe dependents` agree on the relationships.
func extractComponentDependencies(componentSection map[string]any) []schema.ComponentDependency {
	if deps, found := dependenciesFromComponentsSection(componentSection); found {
		return deps
	}
	return dependenciesFromSettings(componentSection)
}

// dependenciesFromComponentsSection reads the preferred `dependencies.components`
// surface and returns its component-to-component entries plus a boolean
// indicating whether the `components` key was present at all. When found is
// false the caller may fall back to legacy settings. When found is true but the
// returned slice is empty, the explicit empty list is the authoritative answer.
func dependenciesFromComponentsSection(componentSection map[string]any) ([]schema.ComponentDependency, bool) {
	depsSection, ok := componentSection[cfg.DependenciesSectionName].(map[string]any)
	if !ok {
		return nil, false
	}
	if _, hasComponents := depsSection["components"]; !hasComponents {
		return nil, false
	}
	var deps schema.Dependencies
	if err := mapstructure.Decode(depsSection, &deps); err != nil {
		// Decode error with the key present: treat as found (authoritative)
		// so we do not silently fall back to stale settings.
		return nil, true
	}
	if normErr := deps.Normalize(); normErr != nil {
		log.Warn("invalid dependencies section; entries may be silently ignored", "error", normErr)
	}
	return filterComponentDependencies(deps.Components), true
}

// dependenciesFromSettings reads the legacy `settings.depends_on` surface.
func dependenciesFromSettings(componentSection map[string]any) []schema.ComponentDependency {
	settingsSection, ok := componentSection[cfg.SettingsSectionName].(map[string]any)
	if !ok {
		return nil
	}
	var settings schema.Settings
	if err := mapstructure.Decode(settingsSection, &settings); err != nil {
		return nil
	}
	if len(settings.DependsOn) == 0 {
		return nil
	}
	deps := make([]schema.ComponentDependency, 0, len(settings.DependsOn))
	for key := range settings.DependsOn {
		ctx := settings.DependsOn[key]
		if ctx.Component == "" {
			continue
		}
		deps = append(deps, schema.ComponentDependency{Component: ctx.Component, Stack: ctx.Stack})
	}
	return deps
}

// filterComponentDependencies keeps only component-to-component dependencies,
// dropping file/folder path entries (which are not graph edges).
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
