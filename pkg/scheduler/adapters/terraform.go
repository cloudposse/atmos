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

const terraformNodeIDFormat = "%s-%s"

const (
	terraformPlanLogOrderStream  = "stream"
	terraformPlanLogOrderGrouped = "grouped"
)

// TerraformExecution contains the execution context for one Terraform component.
type TerraformExecution struct {
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

	if opts.Info.SubCommand == "destroy" {
		graph, err = reverseTerraformGraph(graph)
		if err != nil {
			return fmt.Errorf("%w: reverse terraform graph: %w", errUtils.ErrBuildDepGraph, err)
		}
	}

	maxConcurrency := effectiveTerraformMaxConcurrency(opts.Info)
	if err := validateTerraformConcurrentPlan(opts.Info, graph); err != nil {
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

func validateTerraformConcurrentPlan(info *schema.ConfigAndStacksInfo, _ *dependency.Graph) error {
	if effectiveTerraformMaxConcurrency(info) <= 1 {
		return nil
	}
	if info.Identity == cfg.IdentityFlagSelectValue {
		return fmt.Errorf("%w: --max-concurrency requires a non-interactive identity value", errUtils.ErrInvalidConfig)
	}
	return nil
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

	execution := TerraformExecution{
		Info: nodeInfo,
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

type terraformOutput struct {
	logOrder      string
	hideNoChanges bool
	logDir        string
	stdoutMu      sync.Mutex
	stderrMu      sync.Mutex
	groupMu       sync.Mutex
}

func newTerraformOutput(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, maxConcurrency int) (*terraformOutput, error) {
	hideNoChanges := info != nil && info.TerraformPlanHideNoChanges
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
	logDir := terraformLogDir(atmosConfig)
	return &terraformOutput{
		logOrder:      logOrder,
		hideNoChanges: hideNoChanges,
		logDir:        logDir,
	}, nil
}

func (o *terraformOutput) captureOutput() bool {
	return o != nil && o.logOrder == terraformPlanLogOrderGrouped
}

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

func (o *terraformOutput) openNodeLogFiles(node *dependency.Node) (*os.File, *os.File, map[string]string) {
	if o == nil || o.logDir == "" {
		return nil, nil, nil
	}
	if err := os.MkdirAll(o.logDir, 0o755); err != nil {
		log.Warn("Failed to create Terraform plan log directory", "dir", o.logDir, "error", err)
		return nil, nil, nil
	}
	nodeName := safeTerraformLogName(terraformNodeLabel(node))
	stdoutPath := filepath.Join(o.logDir, nodeName+".stdout.log")
	stderrPath := filepath.Join(o.logDir, nodeName+".stderr.log")
	stdoutFile, stdoutErr := os.Create(stdoutPath)
	if stdoutErr != nil {
		log.Warn("Failed to create Terraform plan stdout log", "file", stdoutPath, "error", stdoutErr)
	}
	stderrFile, stderrErr := os.Create(stderrPath)
	if stderrErr != nil {
		log.Warn("Failed to create Terraform plan stderr log", "file", stderrPath, "error", stderrErr)
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

func combineWriters(primary, secondary io.Writer) io.Writer {
	if primary == nil {
		primary = io.Discard
	}
	if secondary == nil {
		return primary
	}
	return io.MultiWriter(primary, secondary)
}

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

func terraformLogDir(atmosConfig *schema.AtmosConfiguration) string {
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
	return filepath.Join(basePath, ".atmos", "logs", "terraform", "plan")
}

func safeTerraformLogName(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return "terraform"
	}
	replacer := strings.NewReplacer("/", "__", "\\", "__", ":", "_", " ", "_")
	return replacer.Replace(label)
}

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
	fmt.Fprintf(stderr, "\n[%s] terraform plan %s\n", label, status)
	replayGroupedOutput(ioLayer.MaskWriter(os.Stdout), result.Stdout)
	replayGroupedOutput(stderr, result.Stderr)
	fmt.Fprintf(stderr, "[%s] end terraform plan output\n", label)
}

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

func terraformPlanChangedError(info *schema.ConfigAndStacksInfo, err error) bool {
	if info == nil || info.SubCommand != "plan" {
		return false
	}
	var exitCodeErr errUtils.ExitCodeError
	return errors.As(err, &exitCodeErr) && exitCodeErr.Code == 2
}

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

func newTerraformNodeTimings() *terraformNodeTimings {
	return &terraformNodeTimings{
		timings: make(map[string]terraformNodeTiming),
	}
}

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

func (t *terraformNodeTimings) Get(nodeID string) (terraformNodeTiming, bool) {
	if t == nil {
		return terraformNodeTiming{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	timing, ok := t.timings[nodeID]
	return timing, ok
}

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

func terraformNodeLabel(node *dependency.Node) string {
	if node == nil {
		return "terraform"
	}
	if node.Stack == "" {
		return node.Component
	}
	return node.Stack + "/" + node.Component
}

func effectiveTerraformMaxConcurrency(info *schema.ConfigAndStacksInfo) int {
	if info == nil || info.SubCommand != "plan" || info.MaxConcurrency <= 1 {
		return 1
	}
	return info.MaxConcurrency
}
