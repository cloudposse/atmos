package workflow

import (
	"bytes"
	"context"
	"errors"
	"maps"
	"runtime"
	"strings"
	"time"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/testreport"
)

type testLeafExecution struct {
	step           *schema.WorkflowStep
	vars           *step.Variables
	condition      schema.ConditionContext
	stdout, stderr bytes.Buffer
	result         *step.StepResult
	err            error
}

type testLeafOptions struct {
	id             string
	matrix         map[string]string
	status         string
	accountFailure bool
}

// leaf captures one test case, publishes its actual outcome, and accounts for unhandled failures.
func (r *testRun) leaf(ctx context.Context, s *schema.WorkflowStep, vars *step.Variables, opts testLeafOptions) (*step.StepResult, error) {
	started := time.Now()
	execution := newTestLeaf(s, vars, opts.matrix, opts.status)
	if execution.result != nil && execution.result.Skipped {
		vars.Set(s.Name, execution.result)
		r.report.Update(opts.id, testreport.Skipped, 0, "")
		return execution.result, nil
	}
	r.report.Update(opts.id, testreport.Running, 0, "")
	oldWriters := vars.OutputWriters
	vars.OutputWriters = step.OutputWriters{Stdout: &execution.stdout, Stderr: &execution.stderr}
	defer func() { vars.OutputWriters = oldWriters }()
	execution.execute(ctx)
	actual := execution.outcome(ctx)
	r.report.Update(opts.id, actual, time.Since(started), execution.logs(r.all))
	vars.Set(s.Name, execution.result)
	err := execution.unhandledError(ctx)
	if err != nil && ctx.Err() == nil && opts.accountFailure && shouldCancelControl(r.fail, r.failures.Add(1)) {
		r.cancel()
	}
	return execution.result, err
}

// newTestLeaf resolves deferred hook parameters before evaluating the case condition.
func newTestLeaf(s *schema.WorkflowStep, vars *step.Variables, matrix map[string]string, status string) *testLeafExecution {
	execution := &testLeafExecution{step: s, vars: vars}
	if vars.ResolveTestStep != nil {
		resolved, err := vars.ResolveTestStep(s, vars)
		if err != nil {
			execution.err = err
			return execution
		}
		execution.step = resolved
	}
	execution.condition = testCondition(execution.step, vars, matrix, status)
	allowed, err := execution.step.When.EvaluateE(execution.condition)
	execution.err = err
	if err == nil && !allowed {
		execution.result = step.NewStepResult("").WithSkipped()
	}
	return execution
}

// prepareCommand builds a capture-only child with the effective directory, environment, and identity.
func (e *testLeafExecution) prepareCommand() (*schema.WorkflowStep, error) {
	child := *e.step
	base := step.NewBaseHandler("test", step.CategoryCommand, false)
	dir, err := base.ResolveInWorkingDirectory(&child, e.vars, ".", "working_directory")
	if err != nil {
		return nil, err
	}
	child.WorkingDirectory = dir
	child.Output = "none"
	off := false
	child.Show = &schema.ShowConfig{Command: &off, Labels: &off, Progress: &off}
	if child.Type == "" {
		child.Type = schema.TaskTypeShell
	}
	child.Env = maps.Clone(e.vars.Env)
	for k, v := range e.step.Env {
		child.Env[k] = v
	}
	if child.Identity != "" {
		child.Env["ATMOS_IDENTITY"] = child.Identity
	}
	return &child, nil
}

// execute runs a prepared leaf, leaving HTTP retries to their handler and retrying other types here.
func (e *testLeafExecution) execute(ctx context.Context) {
	if e.err != nil || e.step.DryRun {
		return
	}
	child, err := e.prepareCommand()
	if err != nil {
		e.err = err
		return
	}
	executor := step.NewStepExecutorWithVars(e.vars)
	run := func() error {
		outLen, errLen := e.stdout.Len(), e.stderr.Len()
		result, runErr := executor.Execute(step.WithOutputSuppressed(ctx), child)
		e.result = result
		e.captureResult(outLen, errLen)
		return runErr
	}
	if child.Type == "http" || child.Type == "webhook" {
		e.err = run()
		return
	}
	child.Retry = nil
	if e.step.Retry == nil {
		e.err = run()
		return
	}
	e.err = retry.Do(ctx, e.step.Retry, run)
}

// captureResult copies returned streams only when the handler has not already written them.
func (e *testLeafExecution) captureResult(outLen, errLen int) {
	if e.result == nil {
		return
	}
	if v, ok := e.result.Metadata["stdout"].(string); ok && e.stdout.Len() == outLen {
		e.stdout.WriteString(v)
	}
	if v, ok := e.result.Metadata["stderr"].(string); ok && e.stderr.Len() == errLen {
		e.stderr.WriteString(v)
	}
}

// outcome records the actual result before any continuation policy tolerates its failure.
func (e *testLeafExecution) outcome(ctx context.Context) string {
	if e.result == nil {
		e.result = step.NewStepResult("")
	}
	if e.step.DryRun {
		e.result.WithSkipped()
	}
	if ctx.Err() != nil {
		e.err = errors.Join(e.err, ctx.Err())
		e.result.WithError(e.err.Error())
		return testreport.Canceled
	}
	if e.err != nil {
		e.result.WithError(e.err.Error())
		return testreport.Failed
	}
	if e.result.Skipped {
		return testreport.Skipped
	}
	return testreport.Passed
}

// logs selects buffered output and error details according to the suite output policy.
func (e *testLeafExecution) logs(all bool) string {
	if e.err == nil && !all {
		return ""
	}
	logs := e.stdout.String() + e.stderr.String()
	if logs == "" && e.result != nil {
		logs = e.result.Value
	}
	if e.err == nil {
		return logs
	}
	if logs != "" && !strings.HasSuffix(logs, "\n") {
		logs += "\n"
	}
	return logs + e.err.Error()
}

// unhandledError applies continuation policies without hiding cancellation or evaluation failures.
func (e *testLeafExecution) unhandledError(ctx context.Context) error {
	if e.err == nil || ctx.Err() != nil {
		return e.err
	}
	e.condition.Status = schema.ConditionPredicateFailure
	forgiven, err := e.step.Continue.EvaluateContinueE(e.condition)
	if err != nil {
		return errors.Join(e.err, err)
	}
	if forgiven {
		return nil
	}
	return e.err
}

// testCondition exposes the current case, matrix row, flags, and environment to conditions.
func testCondition(s *schema.WorkflowStep, vars *step.Variables, matrix map[string]string, status string) schema.ConditionContext {
	flags := map[string]any{}
	for k, v := range vars.Flags {
		flags[k] = v
	}
	return schema.ConditionContext{CI: ci.IsCI(), Status: status, Step: s.Name, Stack: s.Stack, Env: vars.Env, Flags: flags, Matrix: matrix, OS: runtime.GOOS, Arch: runtime.GOARCH, Platform: runtime.GOOS + "/" + runtime.GOARCH}
}
