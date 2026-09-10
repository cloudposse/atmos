// Package dependencies builds and renders the Atmos component dependency graph
// for the `atmos list dependencies` command. Unlike the execution-ordering graph
// used by the scheduler (which rejects cycles), this builder is cycle-tolerant so
// the command can visualize circular dependencies instead of failing on them.
package dependencies

import (
	"fmt"
	"maps"
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

const (
	logFieldFrom = "from"
	logFieldTo   = "to"
)

// NodeID returns the canonical, collision-safe node ID for a component in a
// stack. It uses a length-prefixed encoding so that component/stack names
// containing the delimiter character never produce the same ID for distinct
// (component, stack) pairs (e.g. "app-prod"+"us" vs "app"+"prod-us").
func NodeID(component, stack string) string {
	return fmt.Sprintf("%d:%s/%d:%s", len(component), component, len(stack), stack)
}

func componentNodeID(component, stack, componentType string) string {
	if componentType == cfg.TerraformComponentType {
		return NodeID(component, stack)
	}
	return fmt.Sprintf("%s/%d:%s", NodeID(component, stack), len(componentType), componentType)
}

// BuildGraph constructs a cycle-tolerant dependency graph from the described
// stacks map. It adds a node for every concrete (non-abstract, enabled)
// component and an edge for every component-to-component dependency
// declared via either `dependencies.components` (preferred) or the legacy
// `settings.depends_on`. Edges to targets that are not present in the graph
// (e.g. disabled or filtered-out components) are skipped when optional or
// when called by scoped structural discovery; required targets otherwise fail.
func BuildGraph(stacks map[string]any) (*dependency.Graph, error) {
	defer perf.Track(nil, "dependencies.BuildGraph")()
	return buildGraph(stacks, nil, "")
}

// buildGraph constructs a graph and validates required targets declared by
// validationSources. A nil source set validates every component; an empty set
// performs structural discovery only. Scoped graph construction defers
// unresolved required values until the graph's selected sources are rendered.
func buildGraph(stacks map[string]any, validationSources map[string]bool, leftDelim string) (*dependency.Graph, error) {
	graph := dependency.NewGraph()
	targetReasons := make(map[string]string)

	// First pass: record all concrete component targets, including unavailable ones.
	walkAllComponents(stacks, func(stackName, componentType, componentName string, componentSection map[string]any) {
		nodeID := componentNodeID(componentName, stackName, componentType)
		if reason := componentAvailabilityReason(componentSection); reason != "" {
			targetReasons[nodeID] = reason
			return
		}
		node := &dependency.Node{
			ID:        nodeID,
			Component: componentName,
			Stack:     stackName,
			Type:      componentType,
			Metadata:  componentSection,
		}
		if err := graph.AddNode(node); err != nil {
			log.Debug("skipping node", "id", nodeID, "error", err)
		}
	})

	// Second pass: add dependency edges now that all nodes exist.
	var buildErr error
	walkComponents(stacks, func(stackName, componentType, componentName string, componentSection map[string]any) {
		if buildErr != nil {
			return
		}
		fromID := componentNodeID(componentName, stackName, componentType)
		deps, modern, err := extractComponentDependenciesWithStack(componentSection, componentType, stackName, leftDelim)
		if err != nil {
			buildErr = fmt.Errorf("parsing dependencies for %q in stack %q: %w", componentName, stackName, err)
			return
		}
		deps = normalizeListDependencies(deps, stackName, componentType)
		edges := graphDependencyBuilder{
			graph:             graph,
			targetReasons:     targetReasons,
			validationSources: validationSources,
			fromID:            fromID,
			stackName:         stackName,
			componentType:     componentType,
			componentName:     componentName,
			modern:            modern,
		}
		for i := range deps {
			if err := edges.add(&deps[i]); err != nil {
				buildErr = err
				return
			}
		}
	})
	if buildErr != nil {
		return nil, buildErr
	}

	graph.IdentifyRoots()
	return graph, nil
}

func shouldValidateDependencyTarget(sourceID string, validationSources map[string]bool) bool {
	return validationSources == nil || validationSources[sourceID]
}

type graphDependencyBuilder struct {
	graph             *dependency.Graph
	targetReasons     map[string]string
	validationSources map[string]bool
	fromID            string
	stackName         string
	componentType     string
	componentName     string
	modern            bool
}

func (b *graphDependencyBuilder) add(dep *schema.ComponentDependency) error {
	targetStack := b.stackName
	if dep.Stack != "" {
		targetStack = dep.Stack
	}
	targetType := dep.Kind
	if targetType == "" {
		targetType = b.componentType
	}
	toID := componentNodeID(dep.Component, targetStack, targetType)
	if reason, unavailable := b.targetReasons[toID]; unavailable {
		return b.handleUnavailable(dep, toID, targetStack, reason)
	}
	if _, exists := b.graph.GetNode(toID); !exists {
		return b.handleMissing(dep, toID, targetStack)
	}
	if err := b.graph.AddDependencyWithOptional(b.fromID, toID, !dep.IsRequired()); err != nil {
		log.Debug("skipping dependency", logFieldFrom, b.fromID, logFieldTo, toID, "error", err)
	}
	return nil
}

func (b *graphDependencyBuilder) handleUnavailable(dep *schema.ComponentDependency, toID, targetStack, reason string) error {
	if !b.modern {
		log.Debug("dependency target not in graph", logFieldFrom, b.fromID, logFieldTo, toID)
		return nil
	}
	if !dep.IsRequired() {
		b.logOptionalDependencySkipped(dep, toID, targetStack, reason)
		return nil
	}
	if shouldValidateDependencyTarget(b.fromID, b.validationSources) {
		targetErr := errUtils.ErrDependencyTargetUnavailable
		if reason == "target_missing" {
			targetErr = errUtils.ErrDependencyTargetNotFound
		}
		return fmt.Errorf("%w: from=%s to=%s reason=%s", targetErr, b.fromID, toID, reason)
	}
	return nil
}

func (b *graphDependencyBuilder) handleMissing(dep *schema.ComponentDependency, toID, targetStack string) error {
	if !b.modern {
		log.Debug("dependency target not in graph", logFieldFrom, b.fromID, logFieldTo, toID)
		return nil
	}
	if !dep.IsRequired() {
		b.logOptionalDependencySkipped(dep, toID, targetStack, "target_missing")
		return nil
	}
	if shouldValidateDependencyTarget(b.fromID, b.validationSources) {
		return fmt.Errorf("%w: from=%s to=%s", errUtils.ErrDependencyTargetNotFound, b.fromID, toID)
	}
	return nil
}

func (b *graphDependencyBuilder) logOptionalDependencySkipped(dep *schema.ComponentDependency, toID, targetStack, reason string) {
	log.Debug("optional dependency skipped", "event", "optional_dependency_skipped", logFieldFrom, b.fromID, logFieldTo, toID,
		"from_component", b.componentName, "from_stack", b.stackName, "to_component", dep.Component,
		"to_stack", targetStack, "kind", dep.Kind, "reason", reason)
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
	walkComponents(stacks, func(stackName, componentType, componentName string, componentSection map[string]any) {
		deps, _, err := extractComponentDependenciesWithStack(componentSection, componentType, "", leftDelim)
		if err != nil {
			return
		}
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

// RequiredDependencySources returns modern dependency sources that require one
// of the selected components. Reverse scoped evaluation needs these sources
// when an unavailable target prevents the structural graph from recording an edge.
func RequiredDependencySources(stacks map[string]any, targets []rootTarget) map[string][]string {
	defer perf.Track(nil, "dependencies.RequiredDependencySources")()

	targetsByStack := make(map[string]map[string]struct{}, len(targets))
	for _, target := range targets {
		if targetsByStack[target.stack] == nil {
			targetsByStack[target.stack] = make(map[string]struct{})
		}
		targetsByStack[target.stack][target.component] = struct{}{}
	}

	sources := make(map[string][]string)
	walkComponents(stacks, func(stackName, componentType, componentName string, componentSection map[string]any) {
		deps, modern, err := extractComponentDependenciesWithStack(componentSection, componentType, "", "")
		if err != nil || !modern {
			return
		}
		for i := range deps {
			targetStack := deps[i].Stack
			if targetStack == "" {
				targetStack = stackName
			}
			if _, ok := targetsByStack[targetStack][deps[i].Component]; ok && deps[i].IsRequired() {
				sources[stackName] = append(sources[stackName], componentName)
				return
			}
		}
	})
	for stackName := range sources {
		sort.Strings(sources[stackName])
	}
	return sources
}

// LegacyDependencySources returns the components that declare legacy
// settings.depends_on dependencies. Their context-based target matching is
// resolved by describe dependents rather than the structural graph.
func LegacyDependencySources(stacks map[string]any) map[string][]string {
	sources := make(map[string][]string)
	walkComponents(stacks, func(stackName, componentType, componentName string, componentSection map[string]any) {
		deps, modern, err := extractComponentDependenciesWithStack(componentSection, componentType, "", "")
		if err == nil && !modern && len(deps) > 0 {
			sources[stackName] = append(sources[stackName], componentName)
		}
	})
	for stackName := range sources {
		sort.Strings(sources[stackName])
	}
	return sources
}

// walkComponents iterates over every concrete component in the stacks
// map, skipping abstract and disabled components.
func walkComponents(stacks map[string]any, fn func(stackName, componentType, componentName string, componentSection map[string]any)) {
	walkComponentsWithUnavailable(stacks, fn, false)
}

func walkAllComponents(stacks map[string]any, fn func(stackName, componentType, componentName string, componentSection map[string]any)) {
	walkComponentsWithUnavailable(stacks, fn, true)
}

func walkComponentsWithUnavailable(stacks map[string]any, fn func(stackName, componentType, componentName string, componentSection map[string]any), includeUnavailable bool) {
	for stackName, stackSection := range stacks {
		componentsSection, ok := stackComponentsSection(stackSection)
		if !ok {
			continue
		}
		for componentType, componentTypeSection := range componentsSection {
			walkComponentType(stackName, componentType, componentTypeSection, includeUnavailable, fn)
		}
	}
}

func stackComponentsSection(stackSection any) (map[string]any, bool) {
	stackSectionMap, ok := stackSection.(map[string]any)
	if !ok {
		return nil, false
	}
	componentsSection, ok := stackSectionMap[cfg.ComponentsSectionName].(map[string]any)
	return componentsSection, ok
}

func walkComponentType(stackName, componentType string, componentTypeSection any, includeUnavailable bool, fn func(string, string, string, map[string]any)) {
	components, ok := componentTypeSection.(map[string]any)
	if !ok {
		return
	}
	for componentName, compSection := range components {
		componentSection, ok := compSection.(map[string]any)
		if !ok || (!includeUnavailable && shouldSkipComponent(componentSection)) {
			continue
		}
		fn(stackName, componentType, componentName, componentSection)
	}
}

func componentAvailabilityReason(componentSection map[string]any) string {
	metadataSection, ok := componentSection[cfg.MetadataSectionName].(map[string]any)
	if !ok {
		return ""
	}
	if metadataType, ok := metadataSection["type"].(string); ok && metadataType == "abstract" {
		return "target_missing"
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
	deps, _, err := extractComponentDependenciesWithStack(componentSection, cfg.TerraformComponentType, "", "")
	if err != nil {
		return nil
	}
	return deps
}

func extractComponentDependenciesWithStack(componentSection map[string]any, componentType, stackName, leftDelim string) ([]schema.ComponentDependency, bool, error) {
	deps, found, err := dependenciesFromComponentsSection(componentSection, componentType, stackName, leftDelim)
	if err != nil || found {
		return filterComponentDependencies(deps), found, err
	}
	return dependenciesFromSettings(componentSection), false, nil
}

// dependenciesFromComponentsSection reads the preferred `dependencies.components`
// surface and returns its component-to-component entries plus a boolean
// indicating whether the `components` key was present at all.
func dependenciesFromComponentsSection(componentSection map[string]any, componentType, stackName, leftDelim string) ([]schema.ComponentDependency, bool, error) {
	dependenciesValue, exists := componentSection[cfg.DependenciesSectionName]
	if !exists {
		return nil, false, nil
	}
	depsSection, ok := dependenciesValue.(map[string]any)
	if !ok {
		return nil, true, fmt.Errorf("%w: %w", errUtils.ErrDependencyResolution, errUtils.ErrInvalidDependenciesSection)
	}
	if _, hasComponents := depsSection["components"]; !hasComponents {
		return nil, false, nil
	}
	depsSection = deferUnresolvedRequired(depsSection, leftDelim)
	deps, err := schema.ParseComponentDependencies(depsSection, componentType, stackName)
	if err != nil {
		return nil, true, fmt.Errorf("%w: parse dependencies: %w", errUtils.ErrDependencyResolution, err)
	}
	return deps, true, nil
}

// deferUnresolvedRequired removes unresolved required values from the
// lightweight graph so they retain the conservative required default until the
// selected component is rendered during scoped evaluation.
func deferUnresolvedRequired(depsSection map[string]any, leftDelim string) map[string]any {
	entries, ok := depsSection["components"].([]any)
	if !ok {
		return depsSection
	}
	deferred := false
	clonedEntries := make([]any, len(entries))
	for i, entry := range entries {
		component, ok := entry.(map[string]any)
		if !ok || !tags.SelectorUnresolved(component["required"], leftDelim) {
			clonedEntries[i] = entry
			continue
		}
		clonedComponent := maps.Clone(component)
		delete(clonedComponent, "required")
		clonedEntries[i] = clonedComponent
		deferred = true
	}
	if !deferred {
		return depsSection
	}
	clonedSection := maps.Clone(depsSection)
	clonedSection["components"] = clonedEntries
	return clonedSection
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

func normalizeListDependencies(deps []schema.ComponentDependency, stackName, componentType string) []schema.ComponentDependency {
	normalized := make([]schema.ComponentDependency, 0, len(deps))
	indices := make(map[string]int, len(deps))
	for i := range deps {
		dep := &deps[i]
		if !dep.IsComponentDependency() || dep.Component == "" {
			continue
		}
		kind := dep.Kind
		if kind == "" {
			kind = componentType
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
