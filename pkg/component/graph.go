package component

import (
	"context"
	"fmt"
	"sort"

	"github.com/mitchellh/mapstructure"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/tags"
)

// graphNodeIDFormat builds an unambiguous node ID from a component and stack name.
// Each field is length-prefixed (%d:%s) so that names containing the delimiter cannot
// collide: ("api-prod","dev") and ("api","prod-dev") produce distinct IDs.
const graphNodeIDFormat = "%d:%s|%d:%s"

type GraphSelection struct {
	NodeIDs             []string
	IncludeDependencies bool
	IncludeDependents   bool
}

type GraphExecutionOptions struct {
	Provider      ComponentProvider
	AtmosConfig   *schema.AtmosConfiguration
	Info          *schema.ConfigAndStacksInfo
	Stacks        map[string]any
	ComponentType string
	SubCommand    string
	Flags         map[string]any
	Selection     *GraphSelection
}

func ExecuteGraph(ctx context.Context, opts *GraphExecutionOptions) error {
	defer perf.Track(nil, "component.ExecuteGraph")()

	if opts == nil {
		return fmt.Errorf("%w: graph execution options are nil", errUtils.ErrGraphExecutionOptions)
	}
	if ctx == nil {
		return fmt.Errorf("%w: context is nil", errUtils.ErrGraphExecutionOptions)
	}

	order, err := prepareExecutionOrder(opts)
	if err != nil {
		return err
	}
	if len(order) == 0 {
		return nil
	}

	log.Info("Processing components in dependency order", "component_type", opts.ComponentType, "count", len(order))
	for i := range order {
		select {
		case <-ctx.Done():
			return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrGraphExecutionCanceled, ctx.Err())
		default:
		}

		if err := executeGraphNode(opts, &order[i]); err != nil {
			return err
		}
	}

	return nil
}

// prepareExecutionOrder validates options, builds and filters the graph, and returns
// the topologically sorted execution order. An empty order indicates no matching components.
func prepareExecutionOrder(opts *GraphExecutionOptions) (dependency.ExecutionOrder, error) {
	if opts.Provider == nil {
		return nil, fmt.Errorf("%w: component provider is nil", errUtils.ErrInvalidConfig)
	}
	if opts.Info == nil {
		return nil, fmt.Errorf("%w: config and stacks info is nil", errUtils.ErrInvalidConfig)
	}
	if opts.ComponentType == "" {
		opts.ComponentType = opts.Provider.GetType()
	}

	graph, err := BuildGraph(opts.Stacks, opts.ComponentType)
	if err != nil {
		return nil, err
	}
	graph = FilterGraph(graph, opts.Info, opts.Selection)
	if graph.Size() == 0 {
		log.Info("No components matched", "component_type", opts.ComponentType)
		return nil, nil
	}

	order, err := graph.TopologicalSort()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrTopologicalOrder, err)
	}
	return order, nil
}

// executeGraphNode executes a single graph node through the component provider.
func executeGraphNode(opts *GraphExecutionOptions, node *dependency.Node) error {
	nodeInfo := *opts.Info
	nodeInfo.ComponentType = opts.ComponentType
	nodeInfo.ComponentFromArg = node.Component
	nodeInfo.Component = node.Component
	nodeInfo.Stack = node.Stack
	nodeInfo.StackFromArg = node.Stack
	nodeInfo.SubCommand = opts.SubCommand
	nodeInfo.All = false
	nodeInfo.Affected = false

	if err := opts.Provider.Execute(&ExecutionContext{
		AtmosConfig:         opts.AtmosConfig,
		ComponentType:       opts.ComponentType,
		Component:           node.Component,
		Stack:               node.Stack,
		Command:             opts.ComponentType,
		SubCommand:          opts.SubCommand,
		ComponentConfig:     node.Metadata,
		ConfigAndStacksInfo: nodeInfo,
		Flags:               opts.Flags,
	}); err != nil {
		return fmt.Errorf("%w: component=%s stack=%s: %w", errUtils.ErrComponentExecutionFailed, node.Component, node.Stack, err)
	}
	return nil
}

func BuildGraph(stacks map[string]any, componentType string) (*dependency.Graph, error) {
	defer perf.Track(nil, "component.BuildGraph")()

	builder := dependency.NewBuilder()
	nodeIDs := make(map[string]struct{})

	if err := walkComponents(stacks, componentType, func(stackName, componentName string, componentSection map[string]any) error {
		if shouldSkipGraphComponent(componentSection) {
			return nil
		}
		nodeID := GraphNodeID(componentName, stackName)
		nodeIDs[nodeID] = struct{}{}
		return builder.AddNode(&dependency.Node{
			ID:        nodeID,
			Component: componentName,
			Stack:     stackName,
			Type:      componentType,
			Metadata:  componentSection,
		})
	}); err != nil {
		return nil, fmt.Errorf("%w: adding nodes: %w", errUtils.ErrBuildDepGraph, err)
	}

	if err := walkComponents(stacks, componentType, func(stackName, componentName string, componentSection map[string]any) error {
		if shouldSkipGraphComponent(componentSection) {
			return nil
		}
		return addComponentDependencies(builder, nodeIDs, dependencyParams{
			componentType:    componentType,
			stackName:        stackName,
			componentName:    componentName,
			componentSection: componentSection,
		})
	}); err != nil {
		return nil, fmt.Errorf("%w: adding dependencies: %w", errUtils.ErrBuildDepGraph, err)
	}

	graph, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrBuildDepGraph, err)
	}
	return graph, nil
}

func FilterGraph(graph *dependency.Graph, info *schema.ConfigAndStacksInfo, selection *GraphSelection) *dependency.Graph {
	defer perf.Track(nil, "component.FilterGraph")()

	if graph == nil {
		return dependency.NewGraph()
	}

	var filtered *dependency.Graph
	switch {
	case selection != nil:
		filtered = filterGraphBySelection(graph, selection)
	case info == nil || info.Stack == "":
		filtered = graph
	default:
		filtered = filterGraphByStack(graph, info.Stack)
	}

	// Tags/labels compose with whichever selection produced the graph above
	// (an explicit node selection, a stack filter, or neither), rather than
	// being an alternative selection mechanism. Because this lives in the
	// shared component package (not per component type), any component type —
	// built-in or custom — that calls ExecuteGraph/FilterGraph gets tag/label
	// filtering automatically.
	return filterGraphByTagsAndLabels(filtered, info)
}

// filterGraphByTagsAndLabels narrows graph nodes to those matching info.Tags
// (any-match) and info.Labels (all-match), applied as an additional pass. A
// no-op when neither is set.
func filterGraphByTagsAndLabels(graph *dependency.Graph, info *schema.ConfigAndStacksInfo) *dependency.Graph {
	if info == nil || (len(info.Tags) == 0 && len(info.Labels) == 0) {
		return graph
	}

	nodeIDs := make([]string, 0)
	for id, node := range graph.Nodes {
		if matchesGraphTagsAndLabels(node, info) {
			nodeIDs = append(nodeIDs, id)
		}
	}
	return graph.Filter(dependency.Filter{NodeIDs: sortedUniqueStrings(nodeIDs)})
}

// matchesGraphTagsAndLabels reports whether a node's component metadata
// matches the requested tags (any) and labels (all).
func matchesGraphTagsAndLabels(node *dependency.Node, info *schema.ConfigAndStacksInfo) bool {
	if node == nil {
		return false
	}
	metadataSection, _ := node.Metadata[cfg.MetadataSectionName].(map[string]any)

	if len(info.Tags) > 0 {
		nodeTags := tags.ToStringSlice(metadataSection["tags"])
		if !tags.MatchesTags(nodeTags, info.Tags, tags.TagModeAny) {
			return false
		}
	}
	if len(info.Labels) > 0 {
		nodeLabels := tags.ToStringMap(metadataSection["labels"])
		if !tags.MatchesLabels(nodeLabels, info.Labels) {
			return false
		}
	}
	return true
}

// filterGraphBySelection filters the graph to the explicitly selected node IDs.
func filterGraphBySelection(graph *dependency.Graph, selection *GraphSelection) *dependency.Graph {
	nodeIDs := sortedUniqueStrings(selection.NodeIDs)
	// Fast-path: the selection already covers every node and pulls in no extra
	// dependencies/dependents. Validate set equality (not just count) so a
	// same-sized but different selection does not return the full graph.
	if !selection.IncludeDependencies && !selection.IncludeDependents && len(nodeIDs) == graph.Size() && allNodesPresent(graph, nodeIDs) {
		return graph
	}
	return graph.Filter(dependency.Filter{
		NodeIDs:             nodeIDs,
		IncludeDependencies: selection.IncludeDependencies,
		IncludeDependents:   selection.IncludeDependents,
	})
}

// allNodesPresent reports whether every ID in nodeIDs exists in the graph.
func allNodesPresent(graph *dependency.Graph, nodeIDs []string) bool {
	for _, id := range nodeIDs {
		if _, ok := graph.Nodes[id]; !ok {
			return false
		}
	}
	return true
}

// filterGraphByStack filters the graph to the nodes belonging to the given stack.
func filterGraphByStack(graph *dependency.Graph, stack string) *dependency.Graph {
	nodeIDs := make([]string, 0)
	for id, node := range graph.Nodes {
		if node != nil && node.Stack == stack {
			nodeIDs = append(nodeIDs, id)
		}
	}
	return graph.Filter(dependency.Filter{NodeIDs: sortedUniqueStrings(nodeIDs)})
}

func GraphNodeID(componentName, stackName string) string {
	defer perf.Track(nil, "component.GraphNodeID")()

	return fmt.Sprintf(graphNodeIDFormat, len(componentName), componentName, len(stackName), stackName)
}

func walkComponents(stacks map[string]any, componentType string, fn func(stackName, componentName string, componentSection map[string]any) error) error {
	stackNames := make([]string, 0, len(stacks))
	for stackName := range stacks {
		stackNames = append(stackNames, stackName)
	}
	sort.Strings(stackNames)

	for _, stackName := range stackNames {
		stackSection, ok := stacks[stackName].(map[string]any)
		if !ok {
			continue
		}
		componentsSection, ok := stackSection[cfg.ComponentsSectionName].(map[string]any)
		if !ok {
			continue
		}
		componentTypeSection, ok := componentsSection[componentType].(map[string]any)
		if !ok {
			continue
		}

		componentNames := make([]string, 0, len(componentTypeSection))
		for componentName := range componentTypeSection {
			componentNames = append(componentNames, componentName)
		}
		sort.Strings(componentNames)

		for _, componentName := range componentNames {
			componentSection, ok := componentTypeSection[componentName].(map[string]any)
			if !ok {
				continue
			}
			if err := fn(stackName, componentName, componentSection); err != nil {
				return err
			}
		}
	}
	return nil
}

// dependencyParams bundles the inputs needed to add a component's dependencies to the graph.
type dependencyParams struct {
	componentType    string
	stackName        string
	componentName    string
	componentSection map[string]any
}

func addComponentDependencies(
	builder *dependency.GraphBuilder,
	nodeIDs map[string]struct{},
	params dependencyParams,
) error {
	fromID := GraphNodeID(params.componentName, params.stackName)
	deps := componentDependencies(params.componentSection)
	for i := range deps {
		dep := &deps[i]
		if !dep.IsComponentDependency() {
			continue
		}
		if dep.Kind != "" && dep.Kind != params.componentType {
			continue
		}
		if dep.Component == "" {
			continue
		}
		depStack := dep.Stack
		if depStack == "" {
			depStack = params.stackName
		}
		toID := GraphNodeID(dep.Component, depStack)
		if _, ok := nodeIDs[toID]; !ok {
			log.Warn("Dependency target not found", "from", fromID, "to", toID)
			continue
		}
		if err := builder.AddDependency(fromID, toID); err != nil {
			return err
		}
	}
	return nil
}

func componentDependencies(componentSection map[string]any) []schema.ComponentDependency {
	if deps := dependenciesFromSection(componentSection); len(deps) > 0 {
		return deps
	}

	settingsSection, ok := componentSection[cfg.SettingsSectionName].(map[string]any)
	if !ok {
		return nil
	}
	return legacyDependenciesFromSettings(settingsSection)
}

// dependenciesFromSection extracts dependencies from the 'dependencies.components' section.
func dependenciesFromSection(componentSection map[string]any) []schema.ComponentDependency {
	depsSection, ok := componentSection[cfg.DependenciesSectionName].(map[string]any)
	if !ok {
		return nil
	}
	if _, hasComponents := depsSection["components"]; !hasComponents {
		return nil
	}
	var deps schema.Dependencies
	if err := mapstructure.Decode(depsSection, &deps); err != nil {
		return nil
	}
	_ = deps.Normalize()
	return deps.Components
}

// legacyDependenciesFromSettings extracts dependencies from the deprecated 'settings.depends_on' section.
func legacyDependenciesFromSettings(settingsSection map[string]any) []schema.ComponentDependency {
	if dependsOn, ok := settingsSection["depends_on"]; ok {
		deps := parseLegacyDependsOn(dependsOn)
		if len(deps) > 0 {
			log.Debug("'settings.depends_on' is deprecated, use 'dependencies.components' instead. See: https://atmos.tools/stacks/dependencies/components")
			return deps
		}
	}

	var settings schema.Settings
	if err := mapstructure.Decode(settingsSection, &settings); err != nil || len(settings.DependsOn) == 0 {
		return nil
	}

	log.Debug("'settings.depends_on' is deprecated, use 'dependencies.components' instead. See: https://atmos.tools/stacks/dependencies/components")
	deps := make([]schema.ComponentDependency, 0, len(settings.DependsOn))
	for key := range settings.DependsOn {
		ctx := settings.DependsOn[key]
		deps = append(deps, schema.ComponentDependency{
			Component:   ctx.Component,
			Stack:       ctx.Stack,
			Namespace:   ctx.Namespace,
			Tenant:      ctx.Tenant,
			Environment: ctx.Environment,
			Stage:       ctx.Stage,
		})
	}
	return deps
}

func parseLegacyDependsOn(dependsOn any) []schema.ComponentDependency {
	switch deps := dependsOn.(type) {
	case []any:
		return parseLegacyDependsOnList(deps)
	case map[string]any:
		values := make([]any, 0, len(deps))
		for _, dep := range deps {
			values = append(values, dep)
		}
		return parseLegacyDependsOnList(values)
	case map[any]any:
		values := make([]any, 0, len(deps))
		for _, dep := range deps {
			values = append(values, dep)
		}
		return parseLegacyDependsOnList(values)
	default:
		return nil
	}
}

func parseLegacyDependsOnList(values []any) []schema.ComponentDependency {
	deps := make([]schema.ComponentDependency, 0, len(values))
	for _, value := range values {
		dep, ok := parseLegacyDependsOnEntry(value)
		if ok {
			deps = append(deps, dep)
		}
	}
	return deps
}

func parseLegacyDependsOnEntry(value any) (schema.ComponentDependency, bool) {
	switch dep := value.(type) {
	case string:
		if dep == "" {
			return schema.ComponentDependency{}, false
		}
		return schema.ComponentDependency{Component: dep}, true
	case map[string]any:
		component, _ := dep["component"].(string)
		if component == "" {
			return schema.ComponentDependency{}, false
		}
		stack, _ := dep["stack"].(string)
		return schema.ComponentDependency{Component: component, Stack: stack}, true
	case map[any]any:
		component, _ := dep["component"].(string)
		if component == "" {
			return schema.ComponentDependency{}, false
		}
		stack, _ := dep["stack"].(string)
		return schema.ComponentDependency{Component: component, Stack: stack}, true
	default:
		return schema.ComponentDependency{}, false
	}
}

func shouldSkipGraphComponent(componentSection map[string]any) bool {
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

func sortedUniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
