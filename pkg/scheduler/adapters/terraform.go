package adapters

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"sync"

	"github.com/mitchellh/mapstructure"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/scheduler"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const terraformNodeIDFormat = "%s-%s"

// TerraformExecutor executes one resolved Terraform component instance.
type TerraformExecutor func(schema.ConfigAndStacksInfo) error

// TerraformOptions configures graph-backed Terraform bulk execution.
type TerraformOptions struct {
	AtmosConfig *schema.AtmosConfiguration
	Info        *schema.ConfigAndStacksInfo
	Stacks      map[string]any
	Executor    TerraformExecutor
}

// ExecuteTerraform runs selected Terraform components through the shared scheduler.
// This PR intentionally fixes effective concurrency at 1.
func ExecuteTerraform(ctx context.Context, opts TerraformOptions) error {
	defer perf.Track(opts.AtmosConfig, "scheduler.adapters.ExecuteTerraform")()

	if opts.AtmosConfig == nil {
		return fmt.Errorf("%w: atmos config is nil", errUtils.ErrInvalidConfig)
	}
	if opts.Info == nil {
		return fmt.Errorf("%w: terraform info is nil", errUtils.ErrInvalidConfig)
	}
	if opts.Executor == nil {
		return fmt.Errorf("%w: terraform executor is nil", errUtils.ErrInvalidConfig)
	}

	graph, err := BuildTerraformGraph(opts.Stacks)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrBuildDepGraph, err)
	}

	graph, err = FilterTerraformGraph(opts.AtmosConfig, graph, opts.Info)
	if err != nil {
		return err
	}

	if graph.Size() == 0 {
		ui.Success("No components matched")
		return nil
	}

	if opts.Info.SubCommand == "destroy" {
		graph, err = reverseTerraformGraph(graph)
		if err != nil {
			return fmt.Errorf("%w: reverse terraform graph: %w", errUtils.ErrBuildDepGraph, err)
		}
	}

	dispatcher := &TerraformDispatcher{
		atmosConfig: opts.AtmosConfig,
		info:        opts.Info,
		executor:    opts.Executor,
		locks:       newTerraformResourceLocks(),
	}
	result := scheduler.New(
		graph,
		dispatcher,
		scheduler.WithMaxConcurrency(effectiveTerraformMaxConcurrency(opts.Info)),
	).Run(ctx)
	if result.Err != nil {
		return result.Err
	}

	if processedCount(result) == 0 {
		ui.Success("No components matched")
	}
	return nil
}

// BuildTerraformGraph builds a Terraform component graph from described stacks.
func BuildTerraformGraph(stacks map[string]any) (*dependency.Graph, error) {
	defer perf.Track(nil, "scheduler.adapters.BuildTerraformGraph")()

	builder := dependency.NewBuilder()
	nodeIDs := make(map[string]struct{})

	if err := walkTerraformComponents(stacks, func(stackName, componentName string, componentSection map[string]any) error {
		if shouldSkipComponent(componentSection) {
			return nil
		}

		nodeID := terraformNodeID(componentName, stackName)
		nodeIDs[nodeID] = struct{}{}
		return builder.AddNode(&dependency.Node{
			ID:        nodeID,
			Component: componentName,
			Stack:     stackName,
			Type:      cfg.TerraformComponentType,
			Metadata:  componentSection,
		})
	}); err != nil {
		return nil, fmt.Errorf("adding nodes: %w", err)
	}

	if err := walkTerraformComponents(stacks, func(stackName, componentName string, componentSection map[string]any) error {
		if shouldSkipComponent(componentSection) {
			return nil
		}
		return addTerraformDependencies(builder, nodeIDs, stackName, componentName, componentSection)
	}); err != nil {
		return nil, fmt.Errorf("adding dependencies: %w", err)
	}

	graph, err := builder.Build()
	if err != nil {
		return nil, err
	}
	log.Debug("Terraform dependency graph built", "nodes", graph.Size(), "roots", len(graph.Roots))
	return graph, nil
}

func reverseTerraformGraph(graph *dependency.Graph) (*dependency.Graph, error) {
	builder := dependency.NewBuilder()

	for _, id := range sortedGraphNodeIDs(graph) {
		node := graph.Nodes[id]
		if err := builder.AddNode(&dependency.Node{
			ID:        node.ID,
			Component: node.Component,
			Stack:     node.Stack,
			Type:      node.Type,
			Metadata:  cloneTerraformNodeMetadata(node.Metadata),
		}); err != nil {
			return nil, err
		}
	}

	for _, id := range sortedGraphNodeIDs(graph) {
		node := graph.Nodes[id]
		for _, dependencyID := range sortedStringValues(node.Dependencies) {
			if _, ok := graph.Nodes[dependencyID]; !ok {
				continue
			}
			if err := builder.AddDependency(dependencyID, id); err != nil {
				return nil, err
			}
		}
	}

	return builder.Build()
}

func cloneTerraformNodeMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return nil
	}
	cloned := make(map[string]any, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

// FilterTerraformGraph narrows graph nodes to the user-selected bulk operation set.
func FilterTerraformGraph(atmosConfig *schema.AtmosConfiguration, graph *dependency.Graph, info *schema.ConfigAndStacksInfo) (*dependency.Graph, error) {
	defer perf.Track(atmosConfig, "scheduler.adapters.FilterTerraformGraph")()

	nodeIDs, err := selectedTerraformNodeIDs(atmosConfig, graph, info)
	if err != nil {
		return nil, err
	}
	if len(nodeIDs) == graph.Size() {
		return graph, nil
	}
	return graph.Filter(dependency.Filter{
		NodeIDs:             nodeIDs,
		IncludeDependencies: false,
		IncludeDependents:   false,
	}), nil
}

// TerraformDispatcher adapts scheduler nodes to Terraform component execution.
type TerraformDispatcher struct {
	atmosConfig *schema.AtmosConfiguration
	info        *schema.ConfigAndStacksInfo
	executor    TerraformExecutor
	locks       *terraformResourceLocks
}

// Dispatch executes one Terraform scheduler node.
func (d *TerraformDispatcher) Dispatch(_ context.Context, node *dependency.Node) (scheduler.Result, error) {
	if node == nil {
		return scheduler.Result{}, fmt.Errorf("%w: node is nil", errUtils.ErrInvalidConfig)
	}
	if d.shouldSkipByQuery(node) {
		return scheduler.Result{NodeID: node.ID, Status: scheduler.StatusSucceeded, Value: false}, nil
	}

	command := fmt.Sprintf("atmos terraform %s %s -s %s", d.info.SubCommand, node.Component, node.Stack)
	log.Debug("Executing", "command", command)

	if d.info.DryRun {
		ui.Successf("Would %s `%s` in `%s` (dry run)", d.info.SubCommand, node.Component, node.Stack)
		return scheduler.Result{NodeID: node.ID, Status: scheduler.StatusSucceeded, Value: true}, nil
	}

	nodeInfo := *d.info
	nodeInfo.Component = node.Component
	nodeInfo.ComponentFromArg = node.Component
	nodeInfo.Stack = node.Stack
	nodeInfo.StackFromArg = node.Stack

	unlock := d.lockTerraformResource(node)
	defer unlock()

	if err := d.executor(nodeInfo); err != nil {
		return scheduler.Result{NodeID: node.ID, Status: scheduler.StatusFailed, Value: true}, fmt.Errorf("%w: component=%s stack=%s: %w", errUtils.ErrTerraformExecFailed, node.Component, node.Stack, err)
	}
	return scheduler.Result{NodeID: node.ID, Status: scheduler.StatusSucceeded, Value: true}, nil
}

func (d *TerraformDispatcher) lockTerraformResource(node *dependency.Node) func() {
	key := terraformResourceKey(node)
	if key == "" {
		return func() {}
	}
	if d.locks == nil {
		return func() {}
	}
	log.Debug("Locking Terraform execution resource", "component", node.Component, "stack", node.Stack, "resource", key)
	return d.locks.Lock(key)
}

func (d *TerraformDispatcher) shouldSkipByQuery(node *dependency.Node) bool {
	if d.info.Query == "" || node.Metadata == nil {
		return false
	}
	passed, err := evaluateTerraformQuery(d.atmosConfig, node.Metadata, d.info.Query)
	if err != nil {
		log.Debug("Error evaluating query", "error", err, "component", node.Component, "stack", node.Stack)
		return true
	}
	if !passed {
		command := fmt.Sprintf("atmos terraform %s %s -s %s", d.info.SubCommand, node.Component, node.Stack)
		log.Debug("Skipping component due to query criteria", "command", command, "query", d.info.Query)
	}
	return !passed
}

func selectedTerraformNodeIDs(atmosConfig *schema.AtmosConfiguration, graph *dependency.Graph, info *schema.ConfigAndStacksInfo) ([]string, error) {
	nodeIDs := make([]string, 0, graph.Size())
	for _, id := range sortedGraphNodeIDs(graph) {
		node := graph.Nodes[id]
		if !matchesTerraformSelection(atmosConfig, node, info) {
			continue
		}
		nodeIDs = append(nodeIDs, id)
	}
	return nodeIDs, nil
}

func matchesTerraformSelection(atmosConfig *schema.AtmosConfiguration, node *dependency.Node, info *schema.ConfigAndStacksInfo) bool {
	if info.Stack != "" && node.Stack != info.Stack {
		return false
	}
	if len(info.Components) > 0 && !containsString(info.Components, node.Component) {
		return false
	}
	if info.Query != "" {
		passed, err := evaluateTerraformQuery(atmosConfig, node.Metadata, info.Query)
		if err != nil {
			log.Debug("Error evaluating query", "error", err, "component", node.Component, "stack", node.Stack)
			return false
		}
		return passed
	}
	return true
}

func evaluateTerraformQuery(atmosConfig *schema.AtmosConfiguration, metadata map[string]any, query string) (bool, error) {
	queryResult, err := u.EvaluateYqExpression(atmosConfig, metadata, query)
	if err != nil {
		return false, fmt.Errorf("%w: %w", errUtils.ErrQueryEvaluation, err)
	}
	queryPassed, ok := queryResult.(bool)
	return ok && queryPassed, nil
}

func addTerraformDependencies(
	builder *dependency.GraphBuilder,
	nodeIDs map[string]struct{},
	stackName string,
	componentName string,
	componentSection map[string]any,
) error {
	fromID := terraformNodeID(componentName, stackName)
	for _, dep := range terraformDependencies(componentSection) {
		if !dep.IsComponentDependency() {
			continue
		}
		if dep.Kind != "" && dep.Kind != cfg.TerraformComponentType {
			continue
		}
		if dep.Component == "" {
			continue
		}
		depStack := dep.Stack
		if depStack == "" {
			depStack = stackName
		}
		toID := terraformNodeID(dep.Component, depStack)
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

func terraformDependencies(componentSection map[string]any) []schema.ComponentDependency {
	if depsSection, ok := componentSection[cfg.DependenciesSectionName].(map[string]any); ok {
		if _, hasComponents := depsSection["components"]; hasComponents {
			var deps schema.Dependencies
			if err := mapstructure.Decode(depsSection, &deps); err == nil && len(deps.Components) > 0 {
				return deps.Components
			}
		}
	}

	settingsSection, ok := componentSection[cfg.SettingsSectionName].(map[string]any)
	if !ok {
		return nil
	}
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

func walkTerraformComponents(stacks map[string]any, fn func(stackName, componentName string, componentSection map[string]any) error) error {
	for _, stackName := range sortedStackNames(stacks) {
		stackSectionMap, ok := stacks[stackName].(map[string]any)
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
		for _, componentName := range sortedComponentNames(terraformSection) {
			componentSection, ok := terraformSection[componentName].(map[string]any)
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

func sortedStackNames(stacks map[string]any) []string {
	names := make([]string, 0, len(stacks))
	for name := range stacks {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedComponentNames(components map[string]any) []string {
	names := make([]string, 0, len(components))
	for name := range components {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedGraphNodeIDs(graph *dependency.Graph) []string {
	ids := make([]string, 0, graph.Size())
	for id := range graph.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func sortedStringValues(values []string) []string {
	sorted := make([]string, len(values))
	copy(sorted, values)
	sort.Strings(sorted)
	return sorted
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func processedCount(result *scheduler.AggregateResult) int {
	count := 0
	if result == nil {
		return count
	}
	for _, nodeResult := range result.Results {
		processed, ok := nodeResult.Value.(bool)
		if ok && processed {
			count++
		}
	}
	return count
}

func terraformNodeID(componentName, stackName string) string {
	return fmt.Sprintf(terraformNodeIDFormat, componentName, stackName)
}

type terraformResourceLocks struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func newTerraformResourceLocks() *terraformResourceLocks {
	return &terraformResourceLocks{
		locks: make(map[string]*sync.Mutex),
	}
}

func (l *terraformResourceLocks) Lock(key string) func() {
	l.mu.Lock()
	lock, ok := l.locks[key]
	if !ok {
		lock = &sync.Mutex{}
		l.locks[key] = lock
	}
	l.mu.Unlock()

	lock.Lock()
	return lock.Unlock
}

func terraformResourceKey(node *dependency.Node) string {
	if node == nil {
		return ""
	}
	if path := componentInfoPath(node.Metadata); path != "" {
		return "path:" + filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	}
	if component := componentField(node.Metadata); component != "" {
		return "component:" + component
	}
	if component := metadataComponent(node.Metadata); component != "" {
		return "component:" + component
	}
	return "component:" + node.Component
}

func componentInfoPath(metadata map[string]any) string {
	componentInfo, ok := metadata["component_info"].(map[string]any)
	if !ok {
		return ""
	}
	path, _ := componentInfo[cfg.ComponentPathSectionName].(string)
	return path
}

func componentField(metadata map[string]any) string {
	component, _ := metadata[cfg.ComponentSectionName].(string)
	return component
}

func metadataComponent(metadata map[string]any) string {
	metadataSection, ok := metadata[cfg.MetadataSectionName].(map[string]any)
	if !ok {
		return ""
	}
	component, _ := metadataSection[cfg.ComponentSectionName].(string)
	return component
}

func effectiveTerraformMaxConcurrency(info *schema.ConfigAndStacksInfo) int {
	if info == nil || info.SubCommand != "plan" || info.MaxConcurrency <= 1 {
		return 1
	}
	return info.MaxConcurrency
}
