package workflow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/dependency"
	iolib "github.com/cloudposse/atmos/pkg/io"
	step "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/scheduler"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui/testreport"
)

func init() { step.RegisterTestRunner(testBridge{}) }

type (
	testBridge struct{}
	testGroup  struct {
		graph *dependency.Graph
		order []string
		ids   map[string]string
	}
)

type testRun struct {
	report   *testreport.Reporter
	groups   map[string]*testGroup
	workflow *schema.WorkflowDefinition
	all      bool
	cancel   context.CancelFunc
	fail     controlFailConfig
	failures atomic.Int64
}

// RunTest expands and runs an isolated test suite, then publishes its report and results.
func (testBridge) RunTest(ctx context.Context, parent *schema.WorkflowStep, vars *step.Variables, workflow *schema.WorkflowDefinition) (*step.StepResult, error) {
	// Copy the tree before assigning names or applying inherited defaults.
	s := *parent
	s.Steps = copyTestSteps(parent.Steps)
	generateWorkflowStepNames(s.Steps, s.Name)
	run := &testRun{groups: map[string]*testGroup{}, workflow: workflow, all: s.Output == "all", fail: effectiveControlFail(&s)}
	roots, err := run.buildTree(&s)
	if err != nil {
		return nil, err
	}
	title := s.Title
	if title == "" {
		title = s.Name
	}
	if title == "" {
		title = "Tests"
	}
	title, err = vars.Resolve(title)
	if err != nil {
		return nil, err
	}
	output, live := testReportOutput(ctx, vars)
	ctx, stopSignals := signal.NotifyContext(ctx, os.Interrupt)
	defer stopSignals()
	ctx, run.cancel = context.WithCancel(ctx)
	defer run.cancel()
	run.report = testreport.New(title, roots, output)
	run.report.Start(live, run.cancel)
	local := vars.Clone()
	inheritTestStep(&s, nil, workflow)
	if err = applyTestEnv(&s, local); err == nil {
		err = run.sequence(ctx, &s, local)
	} else {
		for i := range s.Steps {
			run.skipStep(&s.Steps[i], testreport.Skipped)
		}
		run.report.Update("", testreport.Failed, 0, err.Error())
	}
	renderErr := run.report.Finish()
	if err != nil && renderErr == nil {
		err = &step.TestFailureError{Err: err}
	}
	return testRunResult(run.report, vars, local), errors.Join(err, renderErr)
}

// testReportOutput selects the execution writer and enables live rendering only for an owned terminal.
func testReportOutput(ctx context.Context, vars *step.Variables) (io.Writer, bool) {
	output := vars.OutputWriters.Stderr
	live := output == nil && !step.OutputSuppressed(ctx) && !ci.IsCI() && terminal.New().IsTTY(terminal.Stderr)
	if live {
		return iolib.MaskWriter(os.Stderr), true
	}
	if output == nil {
		output = iolib.GetContext().UI()
	}
	return output, false
}

// testRunResult exposes leaf totals and completed step values to the calling execution.
func testRunResult(report *testreport.Reporter, vars, local *step.Variables) *step.StepResult {
	counts := report.Counts()
	result := step.NewStepResult(fmt.Sprintf("%d passed, %d failed", counts[testreport.Passed], counts[testreport.Failed]))
	for name, count := range counts {
		result.WithMetadata(name, count)
	}
	for name, value := range local.Steps {
		vars.Set(name, value)
	}
	return result
}

// copyTestSteps copies nested slices so naming and inherited defaults cannot mutate the source workflow.
func copyTestSteps(steps []schema.WorkflowStep) []schema.WorkflowStep {
	out := append([]schema.WorkflowStep(nil), steps...)
	for i := range out {
		out[i].Steps = copyTestSteps(out[i].Steps)
	}
	return out
}

// buildTree creates reporter nodes and expands concurrent groups before execution starts.
func (r *testRun) buildTree(s *schema.WorkflowStep) ([]*testreport.Node, error) {
	roots := make([]*testreport.Node, 0, len(s.Steps))
	for i := range s.Steps {
		child := &s.Steps[i]
		n := &testreport.Node{ID: "step:" + child.Name, Name: testLabel(child)}
		if isTestControl(child) {
			if err := r.buildGroupTree(child, n); err != nil {
				return nil, err
			}
		}
		roots = append(roots, n)
	}
	return roots, nil
}

// isTestControl identifies groups whose children use the dependency scheduler.
func isTestControl(s *schema.WorkflowStep) bool {
	return s.Type == schema.TaskTypeParallel || s.Type == schema.TaskTypeMatrix
}

// buildGroupTree assigns stable report IDs to each expanded scheduler case.
func (r *testRun) buildGroupTree(s *schema.WorkflowStep, root *testreport.Node) error {
	graph, order, err := buildControlGraph(s)
	if err != nil {
		return err
	}
	group := &testGroup{graph: graph, order: order, ids: map[string]string{}}
	rows := map[string]*testreport.Node{}
	for _, id := range order {
		node, _ := graph.GetNode(id)
		cn := node.Metadata["child"].(controlNode)
		leaf := &testreport.Node{ID: fmt.Sprintf("case:%d:%s%s", len(s.Name), s.Name, id), Name: testLabel(&cn.step)}
		group.ids[id] = leaf.ID
		if len(cn.matrix) == 0 {
			root.Children = append(root.Children, leaf)
			continue
		}
		addMatrixTestLeaf(s, root, leaf, rows, &cn)
	}
	r.groups[s.Name] = group
	return nil
}

// addMatrixTestLeaf groups expanded cases beneath readable matrix-axis labels.
func addMatrixTestLeaf(s *schema.WorkflowStep, root, leaf *testreport.Node, rows map[string]*testreport.Node, cn *controlNode) {
	label := matrixTestLabel(cn.matrix)
	row := rows[label]
	if row == nil {
		row = &testreport.Node{ID: fmt.Sprintf("row:%d:%s%s", len(s.Name), s.Name, label), Name: label}
		rows[label] = row
		root.Children = append(root.Children, row)
	}
	original := strings.TrimPrefix(cn.step.Name, s.Name+"_"+matrixRowSuffix(cn.matrix)+"_")
	for i := range s.Steps {
		if s.Steps[i].Name == original {
			leaf.Name = testLabel(&s.Steps[i])
			break
		}
	}
	row.Children = append(row.Children, leaf)
}

// testLabel prefers an explicit display title over the execution name.
func testLabel(s *schema.WorkflowStep) string {
	if s.Title != "" {
		return s.Title
	}
	return s.Name
}

// matrixTestLabel sorts axis names to keep row labels deterministic.
func matrixTestLabel(row map[string]string) string {
	keys := make([]string, 0, len(row))
	for k := range row {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	values := make([]string, 0, len(keys))
	for _, k := range keys {
		values = append(values, k+"="+row[k])
	}
	return strings.Join(values, ", ")
}

// sequence runs direct children in order, retaining failures while honoring suite cancellation.
func (r *testRun) sequence(ctx context.Context, parent *schema.WorkflowStep, vars *step.Variables) error {
	var combined error
	status := schema.ConditionPredicateSuccess
	for i := range parent.Steps {
		s := parent.Steps[i]
		inheritTestStep(&s, parent, r.workflow)
		if ctx.Err() != nil {
			r.skipStep(&s, testreport.Canceled)
			continue
		}
		err := r.runChild(ctx, &s, vars, status)
		if err != nil {
			combined = errors.Join(combined, err)
			status = schema.ConditionPredicateFailure
		}
	}
	if ctx.Err() != nil {
		return errors.Join(combined, ctx.Err())
	}
	if r.fail.mode == ControlFailBestEffort {
		return nil
	}
	return combined
}

// runChild evaluates group conditions or runs a leaf and applies its continuation policy.
func (r *testRun) runChild(ctx context.Context, s *schema.WorkflowStep, vars *step.Variables, status string) error {
	group := r.groups[s.Name]
	if group == nil {
		_, err := r.leaf(ctx, s, vars, testLeafOptions{id: "step:" + s.Name, status: status, accountFailure: true})
		return err
	}
	conditionCtx := testCondition(s, vars, nil, status)
	allowed, err := s.When.EvaluateE(conditionCtx)
	if err != nil {
		r.skipStep(s, testreport.Skipped)
		r.report.Update("step:"+s.Name, testreport.Failed, 0, err.Error())
		return err
	}
	if !allowed {
		r.skipStep(s, testreport.Skipped)
		return nil
	}
	started := time.Now()
	r.report.Update("step:"+s.Name, testreport.Running, 0, "")
	err = r.group(ctx, s, group, vars)
	r.report.Update("step:"+s.Name, "", time.Since(started), "")
	if err == nil {
		return nil
	}
	conditionCtx.Status = schema.ConditionPredicateFailure
	forgiven, continueErr := s.Continue.EvaluateContinueE(conditionCtx)
	if continueErr != nil {
		return errors.Join(err, continueErr)
	}
	if forgiven {
		return nil
	}
	return err
}

// skipStep marks every leaf in a skipped or canceled group without changing progress totals.
func (r *testRun) skipStep(s *schema.WorkflowStep, status string) {
	if g := r.groups[s.Name]; g != nil {
		for _, id := range g.order {
			r.report.Update(g.ids[id], status, 0, "")
		}
		return
	}
	r.report.Update("step:"+s.Name, status, 0, "")
}

type testGroupExecutor struct {
	run      *testRun
	parent   *schema.WorkflowStep
	group    *testGroup
	vars     *step.Variables
	mu       sync.Mutex
	failures atomic.Int64
	policy   controlFailConfig
	cancel   context.CancelFunc
}

// group schedules isolated children with the group concurrency and failure policies.
func (r *testRun) group(ctx context.Context, s *schema.WorkflowStep, g *testGroup, vars *step.Variables) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	groupVars := vars.Clone()
	if err := applyTestEnv(s, groupVars); err != nil {
		r.skipStep(s, testreport.Skipped)
		r.report.Update("step:"+s.Name, testreport.Failed, 0, err.Error())
		return err
	}
	executor := &testGroupExecutor{run: r, parent: s, group: g, vars: groupVars, policy: effectiveControlFail(s), cancel: cancel}
	aggregate := scheduler.New(g.graph, executor, scheduler.WithMaxConcurrency(effectiveControlConcurrency(s, len(g.order)))).Run(ctx)
	r.reportSkipped(g, aggregate)
	for name, result := range groupVars.Steps {
		vars.Set(name, result)
	}
	if executor.policy.mode == ControlFailBestEffort && ctx.Err() == nil {
		return nil
	}
	// Explicit skips affect dependents but are not failures of the test suite.
	var combined error
	for i := range aggregate.Results {
		result := &aggregate.Results[i]
		if result.Status == scheduler.StatusFailed {
			combined = errors.Join(combined, result.Err)
		}
	}
	if ctx.Err() != nil {
		return errors.Join(combined, ctx.Err())
	}
	return combined
}

// Dispatch executes one scheduled case and records its result before applying group cancellation.
func (e *testGroupExecutor) Dispatch(ctx context.Context, node *dependency.Node) (scheduler.Result, error) {
	cn := node.Metadata["child"].(controlNode)
	local := e.branchVariables(cn.matrix)
	child := cn.step
	if len(cn.matrix) > 0 {
		child.Name = strings.TrimPrefix(child.Name, e.parent.Name+"_"+matrixRowSuffix(cn.matrix)+"_")
	}
	inheritTestStep(&child, e.parent, e.run.workflow)
	result, err := e.run.leaf(ctx, &child, local, testLeafOptions{id: e.group.ids[node.ID], matrix: cn.matrix, status: schema.ConditionPredicateSuccess, accountFailure: e.accountFailure(local)})
	e.mu.Lock()
	if result != nil {
		e.vars.Set(node.ID, result)
	}
	e.mu.Unlock()
	if err != nil && shouldCancelControl(e.policy, e.failures.Add(1)) {
		e.cancel()
	}
	scheduled := scheduler.Result{Value: result}
	if err == nil && result != nil && result.Skipped {
		scheduled.Status = scheduler.StatusSkipped
	}
	return scheduled, err
}

// Only failures the parent group will propagate count toward the suite policy.
func (e *testGroupExecutor) accountFailure(vars *step.Variables) bool {
	if e.policy.mode == ControlFailBestEffort {
		return false
	}
	condition := testCondition(e.parent, vars, nil, schema.ConditionPredicateFailure)
	forgiven, err := e.parent.Continue.EvaluateContinueE(condition)
	return err != nil || !forgiven
}

// branchVariables isolates mutable state and exposes prior results from the same matrix row.
func (e *testGroupExecutor) branchVariables(matrix map[string]string) *step.Variables {
	e.mu.Lock()
	local := e.vars.Clone()
	e.mu.Unlock()
	local.SetTemplateData(map[string]any{"matrix": matrix})
	if len(matrix) == 0 {
		return local
	}
	prefix := e.parent.Name + "_" + matrixRowSuffix(matrix) + "_"
	for i := range e.parent.Steps {
		original := &e.parent.Steps[i]
		if value, ok := local.Steps[prefix+original.Name]; ok {
			local.Set(original.Name, value)
		}
	}
	return local
}

// reportSkipped distinguishes dependency skips from cancellation in scheduler results.
func (r *testRun) reportSkipped(g *testGroup, aggregate *scheduler.AggregateResult) {
	for i := range aggregate.Results {
		result := &aggregate.Results[i]
		if result.Status != scheduler.StatusSkipped {
			continue
		}
		status := testreport.Skipped
		if errors.Is(result.Err, context.Canceled) || errors.Is(result.Err, context.DeadlineExceeded) {
			status = testreport.Canceled
		}
		r.report.Update(g.ids[result.NodeID], status, 0, "")
	}
}

// inheritTestStep fills unset execution defaults without replacing explicit child values.
func inheritTestStep(s, parent *schema.WorkflowStep, workflow *schema.WorkflowDefinition) {
	if parent != nil {
		if s.WorkingDirectory == "" {
			s.WorkingDirectory = parent.WorkingDirectory
		}
		if s.Stack == "" {
			s.Stack = parent.Stack
		}
		s.DryRun = s.DryRun || parent.DryRun
		if s.Identity == "" {
			s.Identity = parent.Identity
		}
	}
	if workflow != nil {
		if s.WorkingDirectory == "" {
			s.WorkingDirectory = workflow.WorkingDirectory
		}
		if s.Stack == "" {
			s.Stack = workflow.Stack
		}
	}
}

// applyTestEnv resolves suite or group environment entries into its local variables.
func applyTestEnv(s *schema.WorkflowStep, vars *step.Variables) error {
	for k, v := range s.Env {
		resolved, err := vars.Resolve(v)
		if err != nil {
			return err
		}
		vars.SetEnv(k, resolved)
	}
	return nil
}
