// Package dependencies builds and renders the Atmos component dependency graph
// for the `atmos list dependencies` command. Unlike the execution-ordering graph
// used by the scheduler (which rejects cycles), this builder is cycle-tolerant so
// the command can visualize circular dependencies instead of failing on them.
package dependencies

import (
	"errors"
	"fmt"
	"sort"

	"github.com/go-viper/mapstructure/v2"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/tags"
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
	targetReasons := make(map[string]string)

	// First pass: record all concrete component targets, including unavailable ones.
	walkAllComponents(stacks, func(stackName, componentName string, componentSection map[string]any) {
		nodeID := NodeID(componentName, stackName)
		if reason := componentAvailabilityReason(componentSection); reason != "" {
			targetReasons[nodeID] = reason
			return
		}
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
	var buildErr error
	walkComponents(stacks, func(stackName, componentName string, componentSection map[string]any) {
		if buildErr != nil {
			return
		}
		fromID := NodeID(componentName, stackName)
		deps, modern, err := extractComponentDependenciesWithStack(componentSection, stackName)
		if err != nil {
			buildErr = fmt.Errorf("parsing dependencies for %q in stack %q: %w", componentName, stackName, err)
			return
		}
		deps = normalizeListDependencies(deps, stackName)
		for i := range deps {
			dep := &deps[i]
			targetStack := stackName
			if dep.Stack != "" {
				targetStack = dep.Stack
			}
			toID := NodeID(dep.Component, targetStack)
			//nolint:nestif // Required, optional, legacy, and unavailable states have distinct contracts.
			if reason, unavailable := targetReasons[toID]; unavailable {
				if modern && !dep.IsRequired() {
					log.Debug("optional dependency skipped", "event", "optional_dependency_skipped", "from", fromID, "to", toID,
						"from_component", componentName, "from_stack", stackName, "to_component", dep.Component,
						"to_stack", targetStack, "kind", dep.Kind, "reason", reason)
				} else {
					log.Debug("dependency target not in graph", "from", fromID, "to", toID)
				}
				continue
			}
			if _, exists := graph.GetNode(toID); !exists {
				if modern && !dep.IsRequired() {
					log.Debug("optional dependency skipped", "event", "optional_dependency_skipped", "from", fromID, "to", toID,
						"from_component", componentName, "from_stack", stackName, "to_component", dep.Component,
						"to_stack", targetStack, "kind", dep.Kind, "reason", "target_missing")
				} else {
					log.Debug("dependency target not in graph", "from", fromID, "to", toID)
				}
				continue
			}
			if err := graph.AddDependencyWithOptional(fromID, toID, !dep.IsRequired()); err != nil {
				log.Debug("skipping dependency", "from", fromID, "to", toID, "error", err)
			}
		}
	})
	if buildErr != nil {
		return nil, buildErr
	}

	graph.IdentifyRoots()
	return graph, nil
}

// UnresolvedDependencySources returns, per stack, the sorted components whose
// dependency declarations contain an unresolved (templated or YAML-function)
// component/stack value. BuildGraph drops such edges — the literal target ID
// matches no node. For the FORWARD direction the scoped-closure loop converges
// anyway: the declaring component is already in the closure, so Phase C
// evaluation resolves its edges. In the REVERSE direction the declaring
// component is exactly the node the closure is trying to discover, so it must
// be conservatively evaluated for its edges to materialize at all.
func UnresolvedDependencySources(stacks map[string]any, leftDelim string) map[string][]string {
	defer perf.Track(nil, "dependencies.UnresolvedDependencySources")()

	sources := make(map[string][]string)
	walkComponents(stacks, func(stackName, componentName string, componentSection map[string]any) {
		deps := extractComponentDependencies(componentSection)
		for i := range deps {
			if tags.SelectorUnresolved(deps[i].Component, leftDelim) || tags.SelectorUnresolved(deps[i].Stack, leftDelim) {
				sources[stackName] = append(sources[stackName], componentName)
				break
			}
		}
	})
	for stackName := range sources {
		sort.Strings(sources[stackName])
	}
	return sources
}

// walkComponents iterates over every concrete terraform component in the stacks
// map, skipping abstract and disabled components.
func walkComponents(stacks map[string]any, fn func(stackName, componentName string, componentSection map[string]any)) {
	walkComponentsWithUnavailable(stacks, fn, false)
}

func walkAllComponents(stacks map[string]any, fn func(stackName, componentName string, componentSection map[string]any)) {
	walkComponentsWithUnavailable(stacks, fn, true)
}

func walkComponentsWithUnavailable(stacks map[string]any, fn func(stackName, componentName string, componentSection map[string]any), includeUnavailable bool) {
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
			if !includeUnavailable && shouldSkipComponent(componentSection) {
				continue
			}
			fn(stackName, componentName, componentSection)
		}
	}
}

func componentAvailabilityReason(componentSection map[string]any) string {
	metadataSection, ok := componentSection[cfg.MetadataSectionName].(map[string]any)
	if !ok {
		return ""
	}
	if metadataType, ok := metadataSection["type"].(string); ok && metadataType == "abstract" {
		return "target_abstract"
	}
	if enabled, ok := metadataSection["enabled"].(bool); ok && !enabled {
		return "target_disabled"
	}
	return ""
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

func extractComponentDependencies(componentSection map[string]any) []schema.ComponentDependency {
	deps, _, err := extractComponentDependenciesWithStack(componentSection, "")
	if err != nil {
		return nil
	}
	return deps
}

func extractComponentDependenciesWithStack(componentSection map[string]any, stackName string) ([]schema.ComponentDependency, bool, error) {
	deps, found, err := dependenciesFromComponentsSection(componentSection, stackName)
	if err != nil || found {
		return filterComponentDependencies(deps), found, err
	}
	return dependenciesFromSettings(componentSection), false, nil
}

// dependenciesFromComponentsSection reads the preferred `dependencies.components`
// surface and returns its component-to-component entries plus a boolean
// indicating whether the `components` key was present at all.
func dependenciesFromComponentsSection(componentSection map[string]any, stackName string) ([]schema.ComponentDependency, bool, error) {
	depsSection, ok := componentSection[cfg.DependenciesSectionName].(map[string]any)
	if !ok {
		return nil, false, nil
	}
	if _, hasComponents := depsSection["components"]; !hasComponents {
		return nil, false, nil
	}
	deps, err := schema.ParseComponentDependencies(depsSection, cfg.TerraformComponentType, stackName)
	if err != nil {
		if errors.Is(err, schema.ErrComponentDependencyInvalidRequired) {
			return nil, true, fmt.Errorf("%w: parse dependencies: %w", errUtils.ErrDependencyResolution, err)
		}
		return deps, true, nil
	}
	return deps, true, nil
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

func normalizeListDependencies(deps []schema.ComponentDependency, stackName string) []schema.ComponentDependency {
	normalized := make([]schema.ComponentDependency, 0, len(deps))
	indices := make(map[string]int, len(deps))
	for i := range deps {
		dep := &deps[i]
		if !dep.IsComponentDependency() || dep.Component == "" || (dep.Kind != "" && dep.Kind != cfg.TerraformComponentType) {
			continue
		}
		kind := dep.Kind
		if kind == "" {
			kind = cfg.TerraformComponentType
		}
		stack := dep.Stack
		if stack == "" {
			stack = stackName
		}
		key := dep.Component + "\x00" + kind + "\x00" + stack
		if index, exists := indices[key]; exists {
			normalized[index] = *dep
			continue
		}
		indices[key] = len(normalized)
		normalized = append(normalized, *dep)
	}
	return normalized
}
