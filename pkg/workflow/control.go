package workflow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"text/template"

	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/dependency"
	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/scheduler"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	controlNameSep = "_"

	ControlFailWaitAll    = "wait_all"
	ControlFailFast       = "fail_fast"
	ControlFailBestEffort = "best_effort"

	ControlOutputGrouped  = "grouped"
	ControlOutputPrefixed = "prefixed"
	ControlOutputNone     = "none"

	ControlOutputCompletion = "completion"
	ControlOutputDefinition = "definition"
)

type ControlChild struct {
	Step   schema.WorkflowStep
	Matrix map[string]string
}

type ControlChildOutput struct {
	Mode   string
	Prefix string
}

type ControlChildResult struct {
	Stdout   string
	Stderr   string
	Canceled bool
}

type ControlChildExecutor func(ctx context.Context, child *ControlChild, output ControlChildOutput) (*ControlChildResult, error)

type ControlTemplateDataFunc func(stepName string, matrix map[string]string) map[string]any

type ControlStoreResultFunc func(result *scheduler.Result)

type ControlExecutionOptions struct {
	TemplateData ControlTemplateDataFunc
	StoreResult  ControlStoreResultFunc
}

type controlOutputConfig struct {
	mode        string
	order       string
	showSummary bool
	prefix      string
}

type controlFailConfig struct {
	mode        string
	maxFailures int
}

type controlNode struct {
	step        schema.WorkflowStep
	matrix      map[string]string
	displayName string
}

type ControlResult struct {
	Name      string
	Stdout    string
	Stderr    string
	Status    string
	Err       error
	Canceled  bool
	Completed int64
}

func ExecuteControlStep(ctx context.Context, parent *schema.WorkflowStep, executor ControlChildExecutor, opts ControlExecutionOptions) error {
	graph, order, err := buildControlGraph(parent)
	if err != nil {
		return err
	}
	if len(order) == 0 {
		return fmt.Errorf("%w: control step %q has no executable children", schema.ErrWorkflowControlStepInvalid, parent.Name)
	}

	outputCfg := effectiveControlOutput(parent)
	failCfg := effectiveControlFail(parent)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var completedSeq atomic.Int64
	var failures atomic.Int64

	// renderMu serializes every grouped-mode print (start banners and
	// completion blocks) across worker goroutines - nothing in pkg/ui,
	// pkg/data, or pkg/io serializes concurrent writers for us. rendered
	// tracks which node IDs were already streamed live from the completion
	// hook so the post-Run flush below doesn't print them a second time.
	var renderMu sync.Mutex
	rendered := make(map[string]bool, len(order))
	liveStream := outputCfg.mode == ControlOutputGrouped && outputCfg.order == ControlOutputCompletion

	dispatcher := newControlDispatcher(&controlDispatchConfig{
		executor:     executor,
		dataFunc:     opts.TemplateData,
		outputCfg:    outputCfg,
		failCfg:      failCfg,
		cancel:       cancel,
		completedSeq: &completedSeq,
		failures:     &failures,
	})

	schedOpts := []scheduler.Option{
		scheduler.WithMaxConcurrency(effectiveControlConcurrency(parent, len(order))),
		scheduler.WithNodeCompleteHook(func(node *dependency.Node, result scheduler.Result) {
			if !liveStream {
				return
			}
			renderMu.Lock()
			defer renderMu.Unlock()
			renderControlChildBlock(&result)
			rendered[node.ID] = true
		}),
	}
	if outputCfg.mode == ControlOutputGrouped {
		// Starting has no ordering constraint to preserve (unlike completion
		// blocks under order: definition), so announce every child live
		// regardless of the configured order.
		schedOpts = append(schedOpts, scheduler.WithNodeStartHook(func(node *dependency.Node) {
			renderMu.Lock()
			defer renderMu.Unlock()
			stepPkg.AnnounceStepStart(nil, controlNodeLabel(node))
		}))
	}
	if failCfg.mode == ControlFailFast && failCfg.maxFailures <= 1 {
		schedOpts = append(schedOpts, scheduler.WithFailFast(true))
	}

	aggregate := scheduler.New(graph, dispatcher, schedOpts...).Run(runCtx)
	// Flush anything not already streamed live: everything, in definition
	// order, when order: definition (liveStream is false so rendered stays
	// empty); only skipped/never-dispatched nodes when order: completion
	// (liveStream already streamed dispatched nodes, rendered filters them out).
	renderControlOutput(aggregate, order, rendered, outputCfg)
	storeControlResults(aggregate, opts.StoreResult)
	if outputCfg.showSummary {
		renderControlSummary(parent, aggregate)
	}

	if failCfg.mode == ControlFailBestEffort {
		return nil
	}
	return aggregate.Err
}

type controlDispatchConfig struct {
	executor     ControlChildExecutor
	dataFunc     ControlTemplateDataFunc
	outputCfg    controlOutputConfig
	failCfg      controlFailConfig
	cancel       context.CancelFunc
	completedSeq *atomic.Int64
	failures     *atomic.Int64
}

func newControlDispatcher(cfg *controlDispatchConfig) scheduler.Dispatcher {
	return scheduler.DispatcherFunc(func(ctx context.Context, node *dependency.Node) (scheduler.Result, error) {
		child := node.Metadata["child"].(controlNode)
		label := controlDisplayLabel(child.displayName, child.matrix)
		resolved, err := resolveControlStep(&child.step, child.matrix, cfg.dataFunc)
		if err != nil {
			return scheduler.Result{Value: &ControlResult{Name: label, Err: err, Status: string(scheduler.StatusFailed)}}, err
		}
		child.step = resolved
		childOutput := ControlChildOutput{
			Mode:   cfg.outputCfg.mode,
			Prefix: controlPrefix(cfg.outputCfg, child.step.Name, child.matrix, cfg.dataFunc),
		}
		execResult, dispatchErr := cfg.executor(ctx, &ControlChild{Step: child.step, Matrix: child.matrix}, childOutput)
		nodeResult := controlNodeResult(label, cfg.completedSeq.Add(1), execResult, dispatchErr)
		if dispatchErr != nil && shouldCancelControl(cfg.failCfg, cfg.failures.Add(1)) {
			nodeResult.Canceled = nodeResult.Canceled || errors.Is(dispatchErr, context.Canceled)
			cfg.cancel()
		}
		return scheduler.Result{Value: nodeResult}, dispatchErr
	})
}

// controlDisplayLabel builds a human-readable name for a parallel/matrix
// child result. Matrix children carry a hash-qualified graph node ID (see
// matrixRowSuffix) that exists purely to keep dependency-graph IDs
// collision-free after sanitization - it's not meant for display, so
// banners/summaries use the original step name plus the raw matrix values
// instead (e.g. "test (go=1.22, os=linux)").
func controlDisplayLabel(name string, matrix map[string]string) string {
	if len(matrix) == 0 {
		return name
	}
	return name + " (" + matrixRowLabel(matrix) + ")"
}

// controlNodeLabel resolves a graph node's display label from its stored
// controlNode metadata, which is set at graph-build time and so is available
// even for skipped/canceled nodes that were never dispatched (unlike
// ControlResult, which only exists once a node actually ran).
func controlNodeLabel(node *dependency.Node) string {
	if child, ok := node.Metadata["child"].(controlNode); ok {
		return controlDisplayLabel(child.displayName, child.matrix)
	}
	return node.ID
}

func controlNodeResult(name string, completed int64, execResult *ControlChildResult, err error) *ControlResult {
	nodeResult := &ControlResult{Name: name, Completed: completed}
	if execResult != nil {
		nodeResult.Stdout = execResult.Stdout
		nodeResult.Stderr = execResult.Stderr
		nodeResult.Canceled = execResult.Canceled
	}
	if err != nil {
		nodeResult.Err = err
		nodeResult.Status = string(scheduler.StatusFailed)
		return nodeResult
	}
	nodeResult.Status = string(scheduler.StatusSucceeded)
	return nodeResult
}

func storeControlResults(aggregate *scheduler.AggregateResult, store ControlStoreResultFunc) {
	if aggregate == nil || store == nil {
		return
	}
	for i := range aggregate.Results {
		store(&aggregate.Results[i])
	}
}

func buildControlGraph(parent *schema.WorkflowStep) (*dependency.Graph, []string, error) {
	if parent.Type == schema.TaskTypeMatrix {
		return buildMatrixGraph(parent)
	}
	return buildParallelGraph(parent)
}

func buildParallelGraph(parent *schema.WorkflowStep) (*dependency.Graph, []string, error) {
	graph := dependency.NewGraph()
	order := make([]string, 0, len(parent.Steps))
	for i := range parent.Steps {
		child := &parent.Steps[i]
		if err := graph.AddNode(&dependency.Node{
			ID: child.Name,
			Metadata: map[string]any{
				"child": controlNode{step: *child, displayName: child.Name},
			},
		}); err != nil {
			return nil, nil, err
		}
		order = append(order, child.Name)
	}
	for i := range parent.Steps {
		child := &parent.Steps[i]
		for _, need := range child.Needs {
			if err := graph.AddDependency(child.Name, need); err != nil {
				return nil, nil, err
			}
		}
	}
	return graph, order, nil
}

func buildMatrixGraph(parent *schema.WorkflowStep) (*dependency.Graph, []string, error) {
	graph := dependency.NewGraph()
	order := make([]string, 0)
	rows := expandMatrix(parent.Matrix)
	for _, row := range rows {
		rowName := parent.Name + controlNameSep + matrixRowSuffix(row)
		childNames := make(map[string]string, len(parent.Steps))
		for i := range parent.Steps {
			child := &parent.Steps[i]
			childNames[child.Name] = rowName + controlNameSep + child.Name
		}
		if err := addMatrixRowNodes(graph, parent.Steps, childNames, row, &order); err != nil {
			return nil, nil, err
		}
		if err := addMatrixRowDependencies(graph, parent.Steps, childNames); err != nil {
			return nil, nil, err
		}
	}
	return graph, order, nil
}

func addMatrixRowNodes(graph *dependency.Graph, steps []schema.WorkflowStep, childNames map[string]string, row map[string]string, order *[]string) error {
	for i := range steps {
		child := &steps[i]
		expanded := *child
		expanded.Name = childNames[child.Name]
		expanded.Needs = nil
		if err := graph.AddNode(&dependency.Node{
			ID: expanded.Name,
			Metadata: map[string]any{
				"child": controlNode{step: expanded, matrix: row, displayName: child.Name},
			},
		}); err != nil {
			return err
		}
		*order = append(*order, expanded.Name)
	}
	return nil
}

func addMatrixRowDependencies(graph *dependency.Graph, steps []schema.WorkflowStep, childNames map[string]string) error {
	for idx := range steps {
		child := &steps[idx]
		expandedName := childNames[child.Name]
		if len(child.Needs) > 0 {
			for _, need := range child.Needs {
				if err := graph.AddDependency(expandedName, childNames[need]); err != nil {
					return err
				}
			}
			continue
		}
		if idx > 0 {
			previous := &steps[idx-1]
			if err := graph.AddDependency(expandedName, childNames[previous.Name]); err != nil {
				return err
			}
		}
	}
	return nil
}

// renderControlOutput renders any child blocks not already streamed live.
// Order is always the graph's definition order; skip marks node IDs already
// printed by the live WithNodeCompleteHook callback in ExecuteControlStep
// (order: completion mode) so they aren't printed twice. When nothing was
// streamed live (order: definition, or an empty/nil skip set), this
// reproduces a full replay in declared order - which also covers
// skipped/never-dispatched nodes, since the scheduler's node-complete hook
// never fires for them (they're synthesized directly by the scheduler
// without going through a worker).
func renderControlOutput(aggregate *scheduler.AggregateResult, order []string, skip map[string]bool, outputCfg controlOutputConfig) {
	if aggregate == nil || outputCfg.mode != ControlOutputGrouped {
		return
	}
	resultsByID := make(map[string]scheduler.Result, len(aggregate.Results))
	for i := range aggregate.Results {
		result := aggregate.Results[i]
		resultsByID[result.NodeID] = result
	}
	for _, nodeID := range order {
		if skip[nodeID] {
			continue
		}
		result, ok := resultsByID[nodeID]
		if !ok {
			continue
		}
		renderControlChildBlock(&result)
	}
}

// renderControlChildBlock reports a parallel/matrix child's completion status
// through the shared ui.Success/ui.Warning/ui.Error pipeline, matching the
// `AnnounceStepStart`/`AnnounceStepEnd` banner convention used by other
// workflow step engines instead of a raw "[nodeID] status" line. A child's
// own stdout/stderr is still buffered for the duration of its own execution
// (grouped mode doesn't stream a running child's output line-by-line - see
// `prefixed` mode for that), but the finished block is emitted live, the
// moment that child completes, for the default order: completion mode
// (called from ExecuteControlStep's WithNodeCompleteHook, under renderMu);
// for order: definition it's still emitted once for the whole group, in
// declared order, after every child has finished (called from
// renderControlOutput, which runs single-threaded after Run() returns so no
// lock is needed there). Either way, the buffered output prints before the
// completion banner so the log reads as "here's what happened, here's the
// verdict" instead of announcing completion before showing any output.
func renderControlChildBlock(result *scheduler.Result) {
	child, _ := result.Value.(*ControlResult)
	canceled := child != nil && child.Canceled
	name := controlNodeLabel(&result.Node)

	if child != nil {
		if child.Stdout != "" {
			_ = data.Write(child.Stdout)
		}
		if child.Stderr != "" {
			ui.Write(child.Stderr)
		}
	}

	switch {
	case canceled:
		ui.Warningf("`%s` canceled", name)
	case result.Status == scheduler.StatusFailed:
		ui.Errorf("`%s` failed", name)
	case result.Status == scheduler.StatusSkipped:
		ui.Warningf("`%s` skipped", name)
	default:
		ui.Successf("`%s` completed", name)
	}
}

func renderControlSummary(parent *schema.WorkflowStep, aggregate *scheduler.AggregateResult) {
	if aggregate == nil {
		return
	}
	counts := countControlResults(aggregate)
	format := "`%s` summary: %d succeeded, %d failed, %d skipped, %d canceled"
	args := []interface{}{parent.Name, counts.succeeded, counts.failed, counts.skipped, counts.canceled}
	if aggregate.Err != nil || counts.failed > 0 || counts.canceled > 0 {
		ui.Errorf(format, args...)
		return
	}
	if counts.skipped > 0 {
		ui.Warningf(format, args...)
		return
	}
	ui.Successf(format, args...)
}

type controlResultCounts struct {
	succeeded int
	failed    int
	skipped   int
	canceled  int
}

func countControlResults(aggregate *scheduler.AggregateResult) controlResultCounts {
	counts := controlResultCounts{}
	for i := range aggregate.Results {
		result := &aggregate.Results[i]
		child, _ := result.Value.(*ControlResult)
		if child != nil && child.Canceled {
			counts.canceled++
			continue
		}
		switch result.Status {
		case scheduler.StatusSucceeded:
			counts.succeeded++
		case scheduler.StatusFailed:
			counts.failed++
		case scheduler.StatusSkipped:
			counts.skipped++
		}
	}
	return counts
}

func resolveControlStep(step *schema.WorkflowStep, matrix map[string]string, dataFunc ControlTemplateDataFunc) (schema.WorkflowStep, error) {
	resolved := *step
	var err error
	resolved.Command, err = resolveControlTemplate(step.Command, step.Name, matrix, dataFunc)
	if err != nil {
		return resolved, err
	}
	resolved.Stack, err = resolveControlTemplate(step.Stack, step.Name, matrix, dataFunc)
	if err != nil {
		return resolved, err
	}
	resolved.Timeout, err = resolveControlTemplate(step.Timeout, step.Name, matrix, dataFunc)
	if err != nil {
		return resolved, err
	}
	resolved.WorkingDirectory, err = resolveControlTemplate(step.WorkingDirectory, step.Name, matrix, dataFunc)
	if err != nil {
		return resolved, err
	}
	if len(step.Env) > 0 {
		envMap := make(map[string]string, len(step.Env))
		for key, value := range step.Env {
			envMap[key], err = resolveControlTemplate(value, step.Name, matrix, dataFunc)
			if err != nil {
				return resolved, err
			}
		}
		resolved.Env = envMap
	}
	return resolved, nil
}

func resolveControlTemplate(input, stepName string, matrix map[string]string, dataFunc ControlTemplateDataFunc) (string, error) {
	if input == "" || (!strings.Contains(input, "{{") && !strings.Contains(input, "}}")) {
		return input, nil
	}
	tmpl, err := template.New("workflow-control").Parse(input)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, controlTemplateData(stepName, matrix, dataFunc)); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func controlTemplateData(stepName string, matrix map[string]string, dataFunc ControlTemplateDataFunc) map[string]any {
	dataMap := map[string]any{}
	if dataFunc != nil {
		for key, value := range dataFunc(stepName, matrix) {
			dataMap[key] = value
		}
	}
	dataMap["matrix"] = matrix
	dataMap["step"] = map[string]any{"name": stepName}
	return dataMap
}

func effectiveControlOutput(step *schema.WorkflowStep) controlOutputConfig {
	cfg := controlOutputConfig{
		mode:        ControlOutputGrouped,
		order:       ControlOutputCompletion,
		showSummary: true,
		prefix:      "{{ .step.name }}",
	}
	if strings.TrimSpace(step.Output) != "" {
		cfg.mode = strings.TrimSpace(step.Output)
	}
	if step.ParallelOutput == nil {
		return cfg
	}
	if strings.TrimSpace(step.ParallelOutput.Mode) != "" {
		cfg.mode = strings.TrimSpace(step.ParallelOutput.Mode)
	}
	if strings.TrimSpace(step.ParallelOutput.Order) != "" {
		cfg.order = strings.TrimSpace(step.ParallelOutput.Order)
	}
	if step.ParallelOutput.ShowSummary != nil {
		cfg.showSummary = *step.ParallelOutput.ShowSummary
	}
	if strings.TrimSpace(step.ParallelOutput.Prefix) != "" {
		cfg.prefix = step.ParallelOutput.Prefix
	}
	return cfg
}

func effectiveControlFail(step *schema.WorkflowStep) controlFailConfig {
	cfg := controlFailConfig{mode: ControlFailWaitAll}
	if step.Fail != nil {
		if strings.TrimSpace(step.Fail.Mode) != "" {
			cfg.mode = strings.TrimSpace(step.Fail.Mode)
		}
		cfg.maxFailures = step.Fail.MaxFailures
	}
	return cfg
}

func effectiveControlConcurrency(step *schema.WorkflowStep, total int) int {
	if step.MaxConcurrency > 0 && step.MaxConcurrency < total {
		return step.MaxConcurrency
	}
	return total
}

func shouldCancelControl(cfg controlFailConfig, failures int64) bool {
	if cfg.mode == ControlFailBestEffort {
		return false
	}
	threshold := cfg.maxFailures
	if cfg.mode == ControlFailFast && threshold <= 0 {
		threshold = 1
	}
	return threshold > 0 && failures >= int64(threshold)
}
