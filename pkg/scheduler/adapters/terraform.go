package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mitchellh/mapstructure"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ansi"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
	ioLayer "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/scheduler"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	terraformDefaultCommand = "terraform"
	terraformNodeIDFormat   = "%s-%s"
)

const (
	terraformPlanLogOrderStream  = "stream"
	terraformPlanLogOrderGrouped = "grouped"
)

const (
	terraformCLIArgsEnv       = "TF_CLI_ARGS"
	terraformCLIArgsEnvPrefix = "TF_CLI_ARGS_"
)

// TerraformExecution contains the execution context for one Terraform component.
type TerraformExecution struct {
	Context       context.Context
	Info          schema.ConfigAndStacksInfo
	Stdout        io.Writer
	Stderr        io.Writer
	CaptureOutput bool
	Flush         func() error
}

// TerraformExecutionResult contains captured output for one Terraform component.
type TerraformExecutionResult struct {
	Stdout  string
	Stderr  string
	Changed bool
}

// CombinedOutput returns stdout and stderr in the order hooks expect.
func (r TerraformExecutionResult) CombinedOutput() string {
	if r.Stderr == "" {
		return r.Stdout
	}
	if r.Stdout == "" {
		return r.Stderr
	}
	return r.Stdout + "\n" + r.Stderr
}

// TerraformExecutor executes one resolved Terraform component instance.
type TerraformExecutor func(TerraformExecution) (TerraformExecutionResult, error)

// TerraformNodeOutcome records the scheduler-visible result for one Terraform node.
type TerraformNodeOutcome struct {
	Processed bool              `json:"processed"`
	Changed   bool              `json:"changed"`
	ExitCode  int               `json:"exit_code"`
	LogFiles  map[string]string `json:"log_files,omitempty"`
}

// TerraformOptions configures graph-backed Terraform bulk execution.
type TerraformOptions struct {
	AtmosConfig *schema.AtmosConfiguration
	Info        *schema.ConfigAndStacksInfo
	Stacks      map[string]any
	Executor    TerraformExecutor
}

// ExecuteTerraform runs selected Terraform components through the shared scheduler.
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

	maxConcurrency := effectiveTerraformMaxConcurrency(opts.Info)
	if graph, err = prepareTerraformGraphForCommand(opts.Info, graph); err != nil {
		return err
	}

	if err := validateTerraformConcurrentExecution(opts.AtmosConfig, opts.Info, graph); err != nil {
		return err
	}
	output, err := newTerraformOutput(opts.AtmosConfig, opts.Info, maxConcurrency)
	if err != nil {
		return err
	}

	dispatcher := &TerraformDispatcher{
		atmosConfig: opts.AtmosConfig,
		info:        opts.Info,
		executor:    opts.Executor,
		locks:       newTerraformResourceLocks(),
		output:      output,
	}
	timings := newTerraformNodeTimings()
	result := scheduler.New(
		graph,
		dispatcher,
		scheduler.WithMaxConcurrency(maxConcurrency),
		scheduler.WithNodeStartHook(timings.Start),
		scheduler.WithNodeCompleteHook(timings.Complete),
	).Run(ctx)
	if err := writeTerraformSummary(opts.Info, result, timings); err != nil {
		return err
	}
	if result.Err != nil {
		return result.Err
	}

	if processedCount(result) == 0 {
		ui.Success("No components matched")
	}
	if terraformPlanChanged(result) {
		return errUtils.ExitCodeError{Code: 2}
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

// prepareTerraformGraphForCommand adjusts graph ordering for command-specific execution.
func prepareTerraformGraphForCommand(info *schema.ConfigAndStacksInfo, graph *dependency.Graph) (*dependency.Graph, error) {
	if info == nil || graph == nil || info.SubCommand != "destroy" {
		return graph, nil
	}
	return reverseTerraformGraph(graph)
}

// reverseTerraformGraph reverses dependency edges so destroy runs dependents first.
func reverseTerraformGraph(graph *dependency.Graph) (*dependency.Graph, error) {
	builder := dependency.NewBuilder()
	for _, nodeID := range sortedGraphNodeIDs(graph) {
		node := graph.Nodes[nodeID]
		if node == nil {
			continue
		}
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
	for _, nodeID := range sortedGraphNodeIDs(graph) {
		node := graph.Nodes[nodeID]
		if node == nil {
			continue
		}
		for _, dependencyID := range sortedCopy(node.Dependencies) {
			if _, ok := graph.Nodes[dependencyID]; !ok {
				continue
			}
			if err := builder.AddDependency(dependencyID, node.ID); err != nil {
				return nil, err
			}
		}
	}
	return builder.Build()
}

// cloneTerraformNodeMetadata shallow-copies node metadata for graph rewrites.
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

// validateTerraformConcurrentExecution enforces constraints for concurrent Terraform runs.
func validateTerraformConcurrentExecution(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, _ *dependency.Graph) error {
	if effectiveTerraformMaxConcurrency(info) <= 1 {
		return nil
	}
	if info.Identity == cfg.IdentityFlagSelectValue {
		return fmt.Errorf("%w: --max-concurrency requires a non-interactive identity value", errUtils.ErrInvalidConfig)
	}
	if requiresTerraformAutoApprove(info) && !hasTerraformAutoApprove(atmosConfig, info) {
		return fmt.Errorf("%w: concurrent Terraform %s requires -auto-approve", errUtils.ErrInvalidConfig, info.SubCommand)
	}
	return nil
}

// requiresTerraformAutoApprove reports whether concurrent execution must be explicitly approved.
func requiresTerraformAutoApprove(info *schema.ConfigAndStacksInfo) bool {
	return info != nil && (info.SubCommand == "apply" || info.SubCommand == "destroy")
}

// hasTerraformAutoApprove detects auto-approve from config, CLI flags, or Terraform env flags.
func hasTerraformAutoApprove(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) bool {
	if info == nil {
		return false
	}
	if info.SubCommand == "apply" && atmosConfig != nil && atmosConfig.Components.Terraform.ApplyAutoApprove {
		return true
	}
	if containsTerraformFlag(info.AdditionalArgsAndFlags, "-auto-approve") {
		return true
	}
	return hasTerraformAutoApproveEnv(info.SubCommand)
}

func hasTerraformAutoApproveEnv(subcommand string) bool {
	//nolint:forbidigo // TF_CLI_ARGS* are Terraform's native non-interactive flag paths.
	if containsTerraformFlag(strings.Fields(os.Getenv(terraformCLIArgsEnv)), "-auto-approve") {
		return true
	}
	if subcommand == "" {
		return false
	}
	//nolint:forbidigo // TF_CLI_ARGS_<subcommand> is Terraform's native command-specific flag path.
	return containsTerraformFlag(strings.Fields(os.Getenv(terraformCLIArgsEnvPrefix+subcommand)), "-auto-approve")
}

// containsTerraformFlag matches Terraform flags with either one or two leading dashes.
func containsTerraformFlag(args []string, flag string) bool {
	normalizedFlag := strings.TrimLeft(flag, "-")
	for _, arg := range args {
		normalizedArg := strings.TrimLeft(arg, "-")
		if normalizedArg == normalizedFlag {
			return true
		}
		if !strings.HasPrefix(normalizedArg, normalizedFlag+"=") {
			continue
		}
		value := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(normalizedArg, normalizedFlag+"=")))
		switch value {
		case "", "1", "t", "true", "y", "yes", "on":
			return true
		}
	}
	return false
}

// TerraformDispatcher adapts scheduler nodes to Terraform component execution.
type TerraformDispatcher struct {
	atmosConfig *schema.AtmosConfiguration
	info        *schema.ConfigAndStacksInfo
	executor    TerraformExecutor
	locks       *terraformResourceLocks
	output      *terraformOutput
}

// Dispatch executes one Terraform scheduler node.
func (d *TerraformDispatcher) Dispatch(ctx context.Context, node *dependency.Node) (scheduler.Result, error) {
	if node == nil {
		return scheduler.Result{}, fmt.Errorf("%w: node is nil", errUtils.ErrInvalidConfig)
	}
	if err := ctx.Err(); err != nil {
		return scheduler.Result{NodeID: node.ID, Status: scheduler.StatusFailed}, err
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

	execution := TerraformExecution{
		Context: ctx,
		Info:    nodeInfo,
	}
	outcome := TerraformNodeOutcome{
		Processed: true,
	}
	if d.output != nil {
		var logFiles map[string]string
		execution.Stdout, execution.Stderr, execution.Flush, logFiles = d.output.nodeWriters(node)
		execution.CaptureOutput = d.output.captureOutput()
		outcome.LogFiles = logFiles
	}

	execResult, err := d.executor(execution)
	outcome.ExitCode = terraformExitCode(err)
	outcome.Changed = terraformPlanChangedError(d.info, err)
	if execution.Flush != nil {
		if flushErr := execution.Flush(); flushErr != nil && err == nil {
			err = flushErr
			outcome.ExitCode = terraformExitCode(err)
		}
	}
	if d.output != nil {
		execResult.Changed = outcome.Changed
		d.output.finishNode(node, execResult, err)
	}
	if outcome.Changed {
		return scheduler.Result{NodeID: node.ID, Status: scheduler.StatusSucceeded, Value: outcome}, nil
	}
	if err != nil {
		return scheduler.Result{NodeID: node.ID, Status: scheduler.StatusFailed, Value: outcome}, fmt.Errorf("%w: component=%s stack=%s: %w", errUtils.ErrTerraformExecFailed, node.Component, node.Stack, err)
	}
	return scheduler.Result{NodeID: node.ID, Status: scheduler.StatusSucceeded, Value: outcome}, nil
}

// lockTerraformResource serializes nodes that share the same physical Terraform workdir.
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

// shouldSkipByQuery evaluates the query filter for a scheduler node.
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

// selectedTerraformNodeIDs returns graph node IDs matching CLI selection flags.
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

// matchesTerraformSelection reports whether a node is included by stack, component, and query filters.
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

// evaluateTerraformQuery runs a yq expression against Terraform component metadata.
func evaluateTerraformQuery(atmosConfig *schema.AtmosConfiguration, metadata map[string]any, query string) (bool, error) {
	queryResult, err := u.EvaluateYqExpression(atmosConfig, metadata, query)
	if err != nil {
		return false, fmt.Errorf("%w: %w", errUtils.ErrQueryEvaluation, err)
	}
	queryPassed, ok := queryResult.(bool)
	return ok && queryPassed, nil
}

// addTerraformDependencies adds component dependency edges for one graph node.
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

// terraformDependencies extracts modern or legacy dependency declarations from a component.
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

// parseLegacyDependsOn normalizes settings.depends_on into component dependencies.
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

// parseLegacyDependsOnList parses a legacy depends_on sequence.
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

// parseLegacyDependsOnEntry parses one legacy dependency entry.
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

// shouldSkipComponent reports whether a component is abstract or disabled.
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

// walkTerraformComponents visits Terraform components in deterministic stack/component order.
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

// sortedStackNames returns stable stack names for deterministic graph construction.
func sortedStackNames(stacks map[string]any) []string {
	names := make([]string, 0, len(stacks))
	for name := range stacks {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// sortedComponentNames returns stable component names for deterministic graph construction.
func sortedComponentNames(components map[string]any) []string {
	names := make([]string, 0, len(components))
	for name := range components {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// sortedGraphNodeIDs returns graph node IDs in lexical order.
func sortedGraphNodeIDs(graph *dependency.Graph) []string {
	ids := make([]string, 0, graph.Size())
	for id := range graph.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// sortedCopy returns a sorted copy of values.
func sortedCopy(values []string) []string {
	copied := append([]string{}, values...)
	sort.Strings(copied)
	return copied
}

// containsString reports whether values contains value.
func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

// processedCount counts nodes that actually reached Terraform execution.
func processedCount(result *scheduler.AggregateResult) int {
	count := 0
	if result == nil {
		return count
	}
	for _, nodeResult := range result.Results {
		outcome, ok := nodeResult.Value.(TerraformNodeOutcome)
		if ok && outcome.Processed {
			count++
			continue
		}
		processed, ok := nodeResult.Value.(bool)
		if ok && processed {
			count++
		}
	}
	return count
}

// terraformNodeID returns the stable scheduler node ID for a component stack pair.
func terraformNodeID(componentName, stackName string) string {
	return fmt.Sprintf(terraformNodeIDFormat, componentName, stackName)
}

type terraformResourceLocks struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// newTerraformResourceLocks creates a keyed lock registry for shared Terraform resources.
func newTerraformResourceLocks() *terraformResourceLocks {
	return &terraformResourceLocks{
		locks: make(map[string]*sync.Mutex),
	}
}

// Lock acquires the mutex for key and returns its unlock function.
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

// terraformResourceKey returns the physical resource key used to serialize aliases.
func terraformResourceKey(node *dependency.Node) string {
	if node == nil {
		return ""
	}
	if provWorkdir.IsWorkdirEnabled(node.Metadata) {
		return "workdir:" + terraformNodeLabel(node)
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

// componentInfoPath extracts component_info.component_path from metadata.
func componentInfoPath(metadata map[string]any) string {
	componentInfo, ok := metadata["component_info"].(map[string]any)
	if !ok {
		return ""
	}
	path, _ := componentInfo[cfg.ComponentPathSectionName].(string)
	return path
}

// componentField extracts the direct component field from metadata.
func componentField(metadata map[string]any) string {
	component, _ := metadata[cfg.ComponentSectionName].(string)
	return component
}

// metadataComponent extracts metadata.component from metadata.
func metadataComponent(metadata map[string]any) string {
	metadataSection, ok := metadata[cfg.MetadataSectionName].(map[string]any)
	if !ok {
		return ""
	}
	component, _ := metadataSection[cfg.ComponentSectionName].(string)
	return component
}

type terraformOutput struct {
	command       string
	logOrder      string
	hideNoChanges bool
	logDir        string
	stdoutMu      sync.Mutex
	stderrMu      sync.Mutex
	groupMu       sync.Mutex
}

// newTerraformOutput configures concurrent Terraform output streaming or grouping.
func newTerraformOutput(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, maxConcurrency int) (*terraformOutput, error) {
	hideNoChanges := info != nil && info.SubCommand == "plan" && info.TerraformPlanHideNoChanges
	if maxConcurrency <= 1 && !hideNoChanges {
		return nil, nil
	}
	logOrder := terraformPlanLogOrderStream
	if info != nil && info.TerraformPlanLogOrder != "" {
		logOrder = strings.ToLower(info.TerraformPlanLogOrder)
	}
	if hideNoChanges {
		logOrder = terraformPlanLogOrderGrouped
	}
	switch logOrder {
	case terraformPlanLogOrderStream, terraformPlanLogOrderGrouped:
	default:
		return nil, fmt.Errorf("%w: unsupported Terraform plan log order %q", errUtils.ErrInvalidConfig, logOrder)
	}
	command := terraformOutputCommand(info)
	logDir := terraformLogDir(atmosConfig, command)
	return &terraformOutput{
		command:       command,
		logOrder:      logOrder,
		hideNoChanges: hideNoChanges,
		logDir:        logDir,
	}, nil
}

// captureOutput reports whether the executor should capture stdout and stderr.
func (o *terraformOutput) captureOutput() bool {
	return o != nil && o.logOrder == terraformPlanLogOrderGrouped
}

// nodeWriters returns stdout/stderr writers, a flush function, and created log paths for a node.
func (o *terraformOutput) nodeWriters(node *dependency.Node) (io.Writer, io.Writer, func() error, map[string]string) {
	if o == nil {
		return nil, nil, nil, nil
	}
	stdoutFile, stderrFile, logFiles := o.openNodeLogFiles(node)
	if o.logOrder == terraformPlanLogOrderGrouped {
		stdout := combineWriters(io.Discard, stdoutFile)
		stderr := combineWriters(io.Discard, stderrFile)
		return stdout, stderr, closeTerraformLogFiles(stdoutFile, stderrFile), logFiles
	}
	label := terraformNodeLabel(node)
	stdout := ioLayer.NewLinePrefixWriter(label, os.Stdout, &o.stdoutMu)
	stderr := ioLayer.NewLinePrefixWriter(label, os.Stderr, &o.stderrMu)
	return combineWriters(stdout, stdoutFile), combineWriters(stderr, stderrFile), func() error {
		if err := stdout.Flush(); err != nil {
			return err
		}
		if err := stderr.Flush(); err != nil {
			return err
		}
		return closeTerraformLogFiles(stdoutFile, stderrFile)()
	}, logFiles
}

// openNodeLogFiles creates per-node stdout and stderr log files when logging is enabled.
func (o *terraformOutput) openNodeLogFiles(node *dependency.Node) (*os.File, *os.File, map[string]string) {
	if o == nil || o.logDir == "" {
		return nil, nil, nil
	}
	if err := os.MkdirAll(o.logDir, 0o755); err != nil {
		log.Warn("Failed to create Terraform log directory", "dir", o.logDir, "error", err)
		return nil, nil, nil
	}
	nodeName := safeTerraformLogName(terraformNodeLabel(node))
	stdoutPath := filepath.Join(o.logDir, nodeName+".stdout.log")
	stderrPath := filepath.Join(o.logDir, nodeName+".stderr.log")
	stdoutFile, stdoutErr := os.Create(stdoutPath)
	if stdoutErr != nil {
		log.Warn("Failed to create Terraform stdout log", "file", stdoutPath, "error", stdoutErr)
	}
	stderrFile, stderrErr := os.Create(stderrPath)
	if stderrErr != nil {
		log.Warn("Failed to create Terraform stderr log", "file", stderrPath, "error", stderrErr)
	}
	logFiles := make(map[string]string)
	if stdoutFile != nil {
		logFiles["stdout"] = stdoutPath
	}
	if stderrFile != nil {
		logFiles["stderr"] = stderrPath
	}
	if len(logFiles) == 0 {
		return stdoutFile, stderrFile, nil
	}
	return stdoutFile, stderrFile, logFiles
}

// combineWriters combines a primary writer with an optional secondary writer.
func combineWriters(primary, secondary io.Writer) io.Writer {
	if primary == nil {
		primary = io.Discard
	}
	if secondary == nil {
		return primary
	}
	return io.MultiWriter(primary, secondary)
}

// closeTerraformLogFiles returns a cleanup function that closes all non-nil files.
func closeTerraformLogFiles(files ...*os.File) func() error {
	return func() error {
		var errs []error
		for _, file := range files {
			if file == nil {
				continue
			}
			if err := file.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	}
}

// terraformLogDir returns the base directory for Terraform command logs.
func terraformLogDir(atmosConfig *schema.AtmosConfiguration, command string) string {
	if atmosConfig == nil {
		return ""
	}
	basePath := atmosConfig.BasePathAbsolute
	if basePath == "" {
		basePath = atmosConfig.BasePath
	}
	if basePath == "" {
		return ""
	}
	return filepath.Join(basePath, ".atmos", "logs", "terraform", command)
}

// terraformOutputCommand returns the command name used in log paths and messages.
func terraformOutputCommand(info *schema.ConfigAndStacksInfo) string {
	if info == nil || info.SubCommand == "" {
		return terraformDefaultCommand
	}
	return info.SubCommand
}

// safeTerraformLogName converts a node label into a filesystem-safe log prefix.
func safeTerraformLogName(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return "terraform"
	}
	replacer := strings.NewReplacer("/", "__", "\\", "__", ":", "_", " ", "_")
	return replacer.Replace(label)
}

// finishNode replays grouped node output after the node completes.
func (o *terraformOutput) finishNode(node *dependency.Node, result TerraformExecutionResult, execErr error) {
	if o == nil || o.logOrder != terraformPlanLogOrderGrouped {
		return
	}
	if o.hideNoChanges && terraformPlanHasNoChanges(result, execErr) {
		return
	}
	o.groupMu.Lock()
	defer o.groupMu.Unlock()

	label := terraformNodeLabel(node)
	status := "succeeded"
	if execErr != nil {
		status = "failed"
	}
	stderr := ioLayer.MaskWriter(os.Stderr)
	command := o.command
	if command == "" {
		command = terraformDefaultCommand
	}
	writeGroupedOutputMarker(stderr, "\n["+label+"] "+command+" output "+status+"\n")
	replayGroupedOutput(ioLayer.MaskWriter(os.Stdout), result.Stdout)
	replayGroupedOutput(stderr, result.Stderr)
	writeGroupedOutputMarker(stderr, "["+label+"] end "+command+" output\n")
}

func writeGroupedOutputMarker(w io.Writer, marker string) {
	_, _ = io.WriteString(w, marker)
}

// replayGroupedOutput writes captured output and ensures it ends with a newline.
func replayGroupedOutput(w io.Writer, output string) {
	if output == "" {
		return
	}
	if _, err := io.WriteString(w, output); err != nil {
		return
	}
	if output[len(output)-1] != '\n' {
		_, _ = io.WriteString(w, "\n")
	}
}

// terraformPlanHasNoChanges reports whether plan output indicates no infrastructure changes.
func terraformPlanHasNoChanges(result TerraformExecutionResult, execErr error) bool {
	var exitCodeErr errUtils.ExitCodeError
	if errors.As(execErr, &exitCodeErr) {
		return exitCodeErr.Code == 0
	}
	if execErr != nil {
		return false
	}
	output := ansi.Strip(result.CombinedOutput())
	return strings.Contains(output, "No changes. Your infrastructure matches the configuration.") ||
		strings.Contains(output, "No changes. Infrastructure is up-to-date.")
}

// terraformPlanChangedError treats Terraform plan exit code 2 as a changed result.
func terraformPlanChangedError(info *schema.ConfigAndStacksInfo, err error) bool {
	if info == nil || info.SubCommand != "plan" {
		return false
	}
	var exitCodeErr errUtils.ExitCodeError
	return errors.As(err, &exitCodeErr) && exitCodeErr.Code == 2
}

// terraformExitCode maps execution errors into summary exit codes.
func terraformExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitCodeErr errUtils.ExitCodeError
	if errors.As(err, &exitCodeErr) {
		return exitCodeErr.Code
	}
	return 1
}

// terraformPlanChanged reports whether any scheduled plan node found changes.
func terraformPlanChanged(result *scheduler.AggregateResult) bool {
	if result == nil {
		return false
	}
	for _, nodeResult := range result.Results {
		outcome, ok := nodeResult.Value.(TerraformNodeOutcome)
		if ok && outcome.Changed {
			return true
		}
	}
	return false
}

type terraformSummary struct {
	Results []terraformSummaryResult `json:"results"`
}

type terraformSummaryResult struct {
	NodeID     string            `json:"node_id"`
	Stack      string            `json:"stack"`
	Component  string            `json:"component"`
	Status     scheduler.Status  `json:"status"`
	Processed  bool              `json:"processed"`
	Changed    bool              `json:"changed"`
	ExitCode   int               `json:"exit_code"`
	StartedAt  string            `json:"started_at,omitempty"`
	FinishedAt string            `json:"finished_at,omitempty"`
	DurationMS int64             `json:"duration_ms,omitempty"`
	LogFiles   map[string]string `json:"log_files,omitempty"`
	Error      string            `json:"error,omitempty"`
}

type terraformNodeTiming struct {
	startedAt  time.Time
	finishedAt time.Time
}

type terraformNodeTimings struct {
	mu      sync.Mutex
	timings map[string]terraformNodeTiming
}

// newTerraformNodeTimings creates a concurrency-safe timing recorder.
func newTerraformNodeTimings() *terraformNodeTimings {
	return &terraformNodeTimings{
		timings: make(map[string]terraformNodeTiming),
	}
}

// Start records the UTC start time for a node.
func (t *terraformNodeTimings) Start(node *dependency.Node) {
	if t == nil || node == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	timing := t.timings[node.ID]
	timing.startedAt = time.Now().UTC()
	t.timings[node.ID] = timing
}

// Complete records the UTC finish time for a node.
func (t *terraformNodeTimings) Complete(node *dependency.Node, _ scheduler.Result) {
	if t == nil || node == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	timing := t.timings[node.ID]
	timing.finishedAt = time.Now().UTC()
	t.timings[node.ID] = timing
}

// Get returns the recorded timing for nodeID.
func (t *terraformNodeTimings) Get(nodeID string) (terraformNodeTiming, bool) {
	if t == nil {
		return terraformNodeTiming{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	timing, ok := t.timings[nodeID]
	return timing, ok
}

// writeTerraformSummary writes deterministic per-node execution results to JSON.
func writeTerraformSummary(info *schema.ConfigAndStacksInfo, result *scheduler.AggregateResult, timings *terraformNodeTimings) error {
	if info == nil || info.TerraformPlanSummaryFile == "" || result == nil {
		return nil
	}
	summary := terraformSummary{Results: make([]terraformSummaryResult, 0, len(result.Results))}
	for _, nodeResult := range result.Results {
		outcome, _ := nodeResult.Value.(TerraformNodeOutcome)
		entry := terraformSummaryResult{
			NodeID:    nodeResult.NodeID,
			Stack:     nodeResult.Node.Stack,
			Component: nodeResult.Node.Component,
			Status:    nodeResult.Status,
			Processed: outcome.Processed,
			Changed:   outcome.Changed,
			ExitCode:  outcome.ExitCode,
			LogFiles:  outcome.LogFiles,
		}
		if timing, ok := timings.Get(nodeResult.NodeID); ok {
			if !timing.startedAt.IsZero() {
				entry.StartedAt = timing.startedAt.Format(time.RFC3339Nano)
			}
			if !timing.finishedAt.IsZero() {
				entry.FinishedAt = timing.finishedAt.Format(time.RFC3339Nano)
			}
			if !timing.startedAt.IsZero() && !timing.finishedAt.IsZero() {
				entry.DurationMS = timing.finishedAt.Sub(timing.startedAt).Milliseconds()
			}
		}
		if nodeResult.Err != nil {
			entry.Error = nodeResult.Err.Error()
			if entry.ExitCode == 0 {
				entry.ExitCode = 1
			}
		}
		summary.Results = append(summary.Results, entry)
	}
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	path := info.TerraformPlanSummaryFile
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// terraformNodeLabel returns the human-readable stack/component label for a node.
func terraformNodeLabel(node *dependency.Node) string {
	if node == nil {
		return "terraform"
	}
	if node.Stack == "" {
		return node.Component
	}
	return node.Stack + "/" + node.Component
}

// effectiveTerraformMaxConcurrency returns the scheduler concurrency for Terraform plan.
func effectiveTerraformMaxConcurrency(info *schema.ConfigAndStacksInfo) int {
	if info == nil || !supportsTerraformConcurrency(info.SubCommand) || info.MaxConcurrency <= 1 {
		return 1
	}
	return info.MaxConcurrency
}

// supportsTerraformConcurrency reports whether subCommand can run through the scheduler concurrently.
func supportsTerraformConcurrency(subCommand string) bool {
	switch subCommand {
	case "plan", "apply", "destroy":
		return true
	default:
		return false
	}
}
