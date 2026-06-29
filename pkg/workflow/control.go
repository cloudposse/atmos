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
	step   schema.WorkflowStep
	matrix map[string]string
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
	completionOrder := make([]string, 0, len(order))
	var completionMu sync.Mutex

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
		scheduler.WithNodeCompleteHook(func(node *dependency.Node, _ scheduler.Result) {
			completionMu.Lock()
			completionOrder = append(completionOrder, node.ID)
			completionMu.Unlock()
		}),
	}
	if failCfg.mode == ControlFailFast && failCfg.maxFailures <= 1 {
		schedOpts = append(schedOpts, scheduler.WithFailFast(true))
	}

	aggregate := scheduler.New(graph, dispatcher, schedOpts...).Run(runCtx)
	renderControlOutput(aggregate, order, completionOrder, outputCfg)
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
		resolved, err := resolveControlStep(&child.step, child.matrix, cfg.dataFunc)
		if err != nil {
			return scheduler.Result{Value: &ControlResult{Name: child.step.Name, Err: err, Status: string(scheduler.StatusFailed)}}, err
		}
		child.step = resolved
		childOutput := ControlChildOutput{
			Mode:   cfg.outputCfg.mode,
			Prefix: controlPrefix(cfg.outputCfg, child.step.Name, child.matrix, cfg.dataFunc),
		}
		execResult, dispatchErr := cfg.executor(ctx, &ControlChild{Step: child.step, Matrix: child.matrix}, childOutput)
		nodeResult := controlNodeResult(child.step.Name, cfg.completedSeq.Add(1), execResult, dispatchErr)
		if dispatchErr != nil && shouldCancelControl(cfg.failCfg, cfg.failures.Add(1)) {
			nodeResult.Canceled = nodeResult.Canceled || errors.Is(dispatchErr, context.Canceled)
			cfg.cancel()
		}
		return scheduler.Result{Value: nodeResult}, dispatchErr
	})
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
				"child": controlNode{step: *child},
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
				"child": controlNode{step: expanded, matrix: row},
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

func renderControlOutput(aggregate *scheduler.AggregateResult, definitionOrder []string, completionOrder []string, outputCfg controlOutputConfig) {
	if aggregate == nil || outputCfg.mode != ControlOutputGrouped {
		return
	}
	order := definitionOrder
	if outputCfg.order == ControlOutputCompletion {
		order = completionOrder
	}
	resultsByID := make(map[string]scheduler.Result, len(aggregate.Results))
	for i := range aggregate.Results {
		result := aggregate.Results[i]
		resultsByID[result.NodeID] = result
	}
	for _, nodeID := range order {
		result, ok := resultsByID[nodeID]
		if !ok {
			continue
		}
		renderControlChildBlock(&result)
	}
}

func renderControlChildBlock(result *scheduler.Result) {
	child, _ := result.Value.(*ControlResult)
	status := string(result.Status)
	if child != nil && child.Canceled {
		status = "canceled"
	}
	ui.Writeln(fmt.Sprintf("[%s] %s", result.NodeID, status))
	if child != nil {
		if child.Stdout != "" {
			_ = data.Write(child.Stdout)
		}
		if child.Stderr != "" {
			ui.Write(child.Stderr)
		}
	}
}

func renderControlSummary(parent *schema.WorkflowStep, aggregate *scheduler.AggregateResult) {
	if aggregate == nil {
		return
	}
	counts := countControlResults(aggregate)
	format := "[%s] summary: %d succeeded, %d failed, %d skipped, %d canceled"
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
