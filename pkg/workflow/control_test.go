package workflow

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/scheduler"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteControlStepWaitAllSkipsDependents(t *testing.T) {
	showSummary := false
	parent := &schema.WorkflowStep{
		Name: "checks",
		Type: schema.TaskTypeParallel,
		ParallelOutput: &schema.ParallelOutputConfig{
			Mode:        ControlOutputNone,
			ShowSummary: &showSummary,
		},
		Steps: []schema.WorkflowStep{
			{Name: "lint", Type: schema.TaskTypeShell, Command: "lint"},
			{Name: "test", Type: schema.TaskTypeShell, Command: "test"},
			{Name: "summary", Type: schema.TaskTypeShell, Command: "summary", Needs: []string{"lint", "test"}},
		},
	}

	var mu sync.Mutex
	executed := make([]string, 0)
	stored := make(map[string]scheduler.Status)
	err := ExecuteControlStep(context.Background(), parent, func(_ context.Context, child *ControlChild, _ ControlChildOutput) (*ControlChildResult, error) {
		mu.Lock()
		executed = append(executed, child.Step.Name)
		mu.Unlock()
		if child.Step.Name == "lint" {
			return &ControlChildResult{Stdout: "lint failed"}, errors.New("lint failed")
		}
		return &ControlChildResult{Stdout: child.Step.Name}, nil
	}, ControlExecutionOptions{
		StoreResult: func(result *scheduler.Result) {
			stored[result.NodeID] = result.Status
		},
	})

	require.Error(t, err)
	assert.ElementsMatch(t, []string{"lint", "test"}, executed)
	assert.Equal(t, scheduler.StatusFailed, stored["lint"])
	assert.Equal(t, scheduler.StatusSucceeded, stored["test"])
	assert.Equal(t, scheduler.StatusSkipped, stored["summary"])
}

func TestExecuteControlStepBestEffortReturnsSuccess(t *testing.T) {
	showSummary := false
	parent := &schema.WorkflowStep{
		Name: "checks",
		Type: schema.TaskTypeParallel,
		Fail: &schema.ParallelFailConfig{Mode: ControlFailBestEffort},
		ParallelOutput: &schema.ParallelOutputConfig{
			Mode:        ControlOutputNone,
			ShowSummary: &showSummary,
		},
		Steps: []schema.WorkflowStep{
			{Name: "lint", Type: schema.TaskTypeShell, Command: "lint"},
			{Name: "test", Type: schema.TaskTypeShell, Command: "test"},
		},
	}

	err := ExecuteControlStep(context.Background(), parent, func(_ context.Context, child *ControlChild, _ ControlChildOutput) (*ControlChildResult, error) {
		if child.Step.Name == "lint" {
			return &ControlChildResult{Stdout: "lint failed"}, errors.New("lint failed")
		}
		return &ControlChildResult{Stdout: child.Step.Name}, nil
	}, ControlExecutionOptions{})

	require.NoError(t, err)
}

func TestExecuteControlStepMatrixExpandsRows(t *testing.T) {
	showSummary := false
	parent := &schema.WorkflowStep{
		Name: "plans",
		Type: schema.TaskTypeMatrix,
		Matrix: map[string][]string{
			"component": {"vpc", "eks"},
			"stack":     {"dev"},
		},
		ParallelOutput: &schema.ParallelOutputConfig{
			Mode:        ControlOutputNone,
			ShowSummary: &showSummary,
		},
		Steps: []schema.WorkflowStep{{
			Name:    "plan",
			Type:    schema.TaskTypeShell,
			Command: "plan {{ .matrix.component }} {{ .matrix.stack }}",
		}},
	}

	var mu sync.Mutex
	commands := make([]string, 0)
	err := ExecuteControlStep(context.Background(), parent, func(_ context.Context, child *ControlChild, _ ControlChildOutput) (*ControlChildResult, error) {
		mu.Lock()
		commands = append(commands, child.Step.Command)
		mu.Unlock()
		return &ControlChildResult{Stdout: child.Step.Command}, nil
	}, ControlExecutionOptions{})

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"plan vpc dev", "plan eks dev"}, commands)
}

func TestMatrixRowSuffixIncludesAxisNames(t *testing.T) {
	assert.Contains(t, matrixRowSuffix(map[string]string{"axis": "a/b"}), "axis_a_b_")
	assert.Contains(t, matrixRowSuffix(map[string]string{"axis": "a b"}), "axis_a_b_")
	assert.NotEqual(
		t,
		matrixRowSuffix(map[string]string{"axis": "a/b"}),
		matrixRowSuffix(map[string]string{"axis": "a b"}),
	)
	assert.NotEqual(
		t,
		matrixRowSuffix(map[string]string{"component": "app", "stack": "dev"}),
		matrixRowSuffix(map[string]string{"component": "dev", "stack": "app"}),
	)
}

func TestSanitizeControlNameEmptyFallback(t *testing.T) {
	assert.Equal(t, "empty", sanitizeControlName(""))
	assert.Equal(t, "empty", sanitizeControlName("///"))
	assert.Regexp(t, `^empty_empty_[a-z0-9]+$`, matrixRowSuffix(map[string]string{"": "///"}))
}

func TestExecuteControlStepFailFastStopsAfterFirstFailure(t *testing.T) {
	showSummary := false
	parent := &schema.WorkflowStep{
		Name: "checks",
		Type: schema.TaskTypeParallel,
		Fail: &schema.ParallelFailConfig{Mode: ControlFailFast},
		ParallelOutput: &schema.ParallelOutputConfig{
			Mode:        ControlOutputNone,
			ShowSummary: &showSummary,
		},
		Steps: []schema.WorkflowStep{
			{Name: "lint", Type: schema.TaskTypeShell, Command: "lint"},
			{Name: "test", Type: schema.TaskTypeShell, Command: "test", Needs: []string{"lint"}},
		},
	}

	executed := make([]string, 0)
	err := ExecuteControlStep(context.Background(), parent, func(_ context.Context, child *ControlChild, _ ControlChildOutput) (*ControlChildResult, error) {
		executed = append(executed, child.Step.Name)
		return &ControlChildResult{Stdout: "lint failed"}, errors.New("lint failed")
	}, ControlExecutionOptions{})

	require.Error(t, err)
	assert.Equal(t, []string{"lint"}, executed)
}

func TestExecuteControlStepUsesPrefixedOutputAndCustomTemplateData(t *testing.T) {
	parent := &schema.WorkflowStep{
		Name: "checks",
		Type: schema.TaskTypeParallel,
		ParallelOutput: &schema.ParallelOutputConfig{
			Mode:        ControlOutputPrefixed,
			Prefix:      "{{ .custom }}:{{ .step.name }}",
			ShowSummary: boolPtr(false),
		},
		Steps: []schema.WorkflowStep{{
			Name:    "echo",
			Type:    schema.TaskTypeShell,
			Command: "echo {{ .custom }}",
			Stack:   "{{ .custom }}-stack",
			Timeout: "{{ .custom }}s",
			Env:     map[string]string{"CUSTOM": "{{ .custom }}"},
		}},
	}

	var gotOutput ControlChildOutput
	var gotStep schema.WorkflowStep
	err := ExecuteControlStep(context.Background(), parent, func(_ context.Context, child *ControlChild, output ControlChildOutput) (*ControlChildResult, error) {
		gotOutput = output
		gotStep = child.Step
		return &ControlChildResult{}, nil
	}, ControlExecutionOptions{
		TemplateData: func(string, map[string]string) map[string]any {
			return map[string]any{"custom": "value"}
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ControlOutputPrefixed, gotOutput.Mode)
	assert.Equal(t, "value:echo", gotOutput.Prefix)
	assert.Equal(t, "echo value", gotStep.Command)
	assert.Equal(t, "value-stack", gotStep.Stack)
	assert.Equal(t, "values", gotStep.Timeout)
	assert.Equal(t, map[string]string{"CUSTOM": "value"}, gotStep.Env)
}

func TestExecuteControlStepTemplateError(t *testing.T) {
	parent := &schema.WorkflowStep{
		Name: "checks",
		Type: schema.TaskTypeParallel,
		ParallelOutput: &schema.ParallelOutputConfig{
			Mode:        ControlOutputNone,
			ShowSummary: boolPtr(false),
		},
		Steps: []schema.WorkflowStep{{
			Name:    "bad-template",
			Type:    schema.TaskTypeShell,
			Command: "{{ .missing",
		}},
	}

	err := ExecuteControlStep(context.Background(), parent, func(context.Context, *ControlChild, ControlChildOutput) (*ControlChildResult, error) {
		t.Fatal("executor should not be called when template resolution fails")
		return nil, nil
	}, ControlExecutionOptions{})

	require.Error(t, err)
}

func TestBuildMatrixGraphUsesSequentialDependenciesWithoutNeeds(t *testing.T) {
	parent := &schema.WorkflowStep{
		Name:   "plans",
		Type:   schema.TaskTypeMatrix,
		Matrix: map[string][]string{"stack": {"dev"}},
		Steps: []schema.WorkflowStep{
			{Name: "init", Type: schema.TaskTypeShell, Command: "init"},
			{Name: "plan", Type: schema.TaskTypeShell, Command: "plan"},
			{Name: "apply", Type: schema.TaskTypeShell, Command: "apply", Needs: []string{"init"}},
		},
	}

	graph, order, err := buildMatrixGraph(parent)
	require.NoError(t, err)
	require.Len(t, order, 3)

	initID := order[0]
	planID := order[1]
	applyID := order[2]
	assert.Contains(t, graph.Nodes[planID].Dependencies, initID)
	assert.Contains(t, graph.Nodes[applyID].Dependencies, initID)
}

func TestRenderControlOutputAndSummaryBranches(t *testing.T) {
	initControlTestIO(t)
	failedErr := errors.New("failed")
	aggregate := &scheduler.AggregateResult{
		Err: failedErr,
		Results: []scheduler.Result{
			{NodeID: "ok", Status: scheduler.StatusSucceeded, Value: &ControlResult{Name: "ok", Stdout: "stdout\n", Stderr: "stderr\n"}},
			{NodeID: "failed", Status: scheduler.StatusFailed, Value: &ControlResult{Name: "failed", Err: failedErr}},
			{NodeID: "skipped", Status: scheduler.StatusSkipped},
			{NodeID: "canceled", Status: scheduler.StatusFailed, Value: &ControlResult{Name: "canceled", Canceled: true}},
		},
	}

	renderControlOutput(aggregate, []string{"missing", "ok", "failed", "skipped", "canceled"}, []string{"canceled", "ok"}, controlOutputConfig{
		mode:  ControlOutputGrouped,
		order: ControlOutputDefinition,
	})
	renderControlOutput(aggregate, []string{"ok"}, []string{"canceled", "ok"}, controlOutputConfig{
		mode:  ControlOutputGrouped,
		order: ControlOutputCompletion,
	})
	renderControlOutput(nil, nil, nil, controlOutputConfig{mode: ControlOutputGrouped})
	renderControlOutput(aggregate, []string{"ok"}, nil, controlOutputConfig{mode: ControlOutputNone})

	counts := countControlResults(aggregate)
	assert.Equal(t, controlResultCounts{succeeded: 1, failed: 1, skipped: 1, canceled: 1}, counts)
	renderControlSummary(&schema.WorkflowStep{Name: "checks"}, aggregate)
	renderControlSummary(&schema.WorkflowStep{Name: "checks"}, &scheduler.AggregateResult{
		Results: []scheduler.Result{{NodeID: "skipped", Status: scheduler.StatusSkipped}},
	})
	renderControlSummary(&schema.WorkflowStep{Name: "checks"}, &scheduler.AggregateResult{
		Results: []scheduler.Result{{NodeID: "ok", Status: scheduler.StatusSucceeded}},
	})
	renderControlSummary(&schema.WorkflowStep{Name: "checks"}, nil)
}

func TestControlConfigurationDefaultsAndCancellationThresholds(t *testing.T) {
	defaultOutput := effectiveControlOutput(&schema.WorkflowStep{})
	assert.Equal(t, controlOutputConfig{
		mode:        ControlOutputGrouped,
		order:       ControlOutputCompletion,
		showSummary: true,
		prefix:      "{{ .step.name }}",
	}, defaultOutput)

	showSummary := false
	customOutput := effectiveControlOutput(&schema.WorkflowStep{
		Output: ControlOutputNone,
		ParallelOutput: &schema.ParallelOutputConfig{
			Mode:        ControlOutputPrefixed,
			Order:       ControlOutputDefinition,
			ShowSummary: &showSummary,
			Prefix:      "prefix",
		},
	})
	assert.Equal(t, controlOutputConfig{
		mode:        ControlOutputPrefixed,
		order:       ControlOutputDefinition,
		showSummary: false,
		prefix:      "prefix",
	}, customOutput)

	assert.Equal(t, controlFailConfig{mode: ControlFailWaitAll}, effectiveControlFail(&schema.WorkflowStep{}))
	assert.Equal(t, controlFailConfig{mode: ControlFailFast, maxFailures: 2}, effectiveControlFail(&schema.WorkflowStep{
		Fail: &schema.ParallelFailConfig{Mode: ControlFailFast, MaxFailures: 2},
	}))
	assert.Equal(t, 2, effectiveControlConcurrency(&schema.WorkflowStep{MaxConcurrency: 2}, 5))
	assert.Equal(t, 5, effectiveControlConcurrency(&schema.WorkflowStep{MaxConcurrency: 7}, 5))
	assert.False(t, shouldCancelControl(controlFailConfig{mode: ControlFailBestEffort}, 99))
	assert.True(t, shouldCancelControl(controlFailConfig{mode: ControlFailFast}, 1))
	assert.False(t, shouldCancelControl(controlFailConfig{mode: ControlFailWaitAll, maxFailures: 2}, 1))
	assert.True(t, shouldCancelControl(controlFailConfig{mode: ControlFailWaitAll, maxFailures: 2}, 2))
}

func initControlTestIO(t *testing.T) {
	t.Helper()
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)
}

func boolPtr(v bool) *bool {
	return &v
}
