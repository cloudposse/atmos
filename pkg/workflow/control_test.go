package workflow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	stdio "io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/dependency"
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

// TestExecuteControlStepGroupedModeRendersSkippedChildBanner is a regression
// test: the scheduler never dispatches skipped/never-run nodes, so
// WithNodeCompleteHook never fires for them - before renderControlOutput's
// skip-set flush, order: completion mode silently dropped their banner
// entirely (they were absent from the old completionOrder list) even though
// renderControlSummary's counts included them.
func TestExecuteControlStepGroupedModeRendersSkippedChildBanner(t *testing.T) {
	_, stderr := initControlTestIOCapture(t)
	parent := &schema.WorkflowStep{
		Name: "checks",
		Type: schema.TaskTypeParallel,
		ParallelOutput: &schema.ParallelOutputConfig{
			Mode:        ControlOutputGrouped,
			ShowSummary: boolPtr(false),
		},
		Steps: []schema.WorkflowStep{
			{Name: "lint", Type: schema.TaskTypeShell, Command: "lint"},
			{Name: "summary", Type: schema.TaskTypeShell, Command: "summary", Needs: []string{"lint"}},
		},
	}

	err := ExecuteControlStep(context.Background(), parent, func(_ context.Context, child *ControlChild, _ ControlChildOutput) (*ControlChildResult, error) {
		if child.Step.Name == "lint" {
			return &ControlChildResult{}, errors.New("lint failed")
		}
		return &ControlChildResult{}, nil
	}, ControlExecutionOptions{})

	require.Error(t, err)
	assert.Contains(t, stderr.String(), "summary skipped")
}

// TestExecuteControlStepLiveCompletionOrderStreamsImmediately proves that
// order: completion (the default) renders each child's block the instant
// that specific child finishes, instead of waiting for the whole group.
func TestExecuteControlStepLiveCompletionOrderStreamsImmediately(t *testing.T) {
	stdout, stderr := initControlTestIOCapture(t)
	slowRelease := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(slowRelease) }) }
	// Guarantees the slow child unblocks (and its goroutine exits) even if an
	// assertion below fails and returns early, so a failure here can't leak a
	// goroutine that keeps touching global ui/terminal state into later tests.
	defer release()
	parent := &schema.WorkflowStep{
		Name: "checks",
		Type: schema.TaskTypeParallel,
		ParallelOutput: &schema.ParallelOutputConfig{
			Mode:  ControlOutputGrouped,
			Order: ControlOutputCompletion,
		},
		Steps: []schema.WorkflowStep{
			{Name: "fast", Type: schema.TaskTypeShell, Command: "fast"},
			{Name: "slow", Type: schema.TaskTypeShell, Command: "slow"},
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- ExecuteControlStep(context.Background(), parent, func(_ context.Context, child *ControlChild, _ ControlChildOutput) (*ControlChildResult, error) {
			if child.Step.Name == "slow" {
				<-slowRelease
				return &ControlChildResult{Stdout: "slow output\n"}, nil
			}
			return &ControlChildResult{Stdout: "fast output\n"}, nil
		}, ControlExecutionOptions{})
	}()

	require.Eventually(t, func() bool {
		return strings.Contains(stdout.String(), "fast output")
	}, time.Second, 5*time.Millisecond, "fast child's output should stream before the slow child finishes")
	assert.NotContains(t, stdout.String(), "slow output", "slow child hasn't been released yet")
	assert.Contains(t, stderr.String(), "fast completed")

	release()
	require.Eventually(t, func() bool {
		return strings.Contains(stdout.String(), "slow output")
	}, time.Second, 5*time.Millisecond, "slow child's output should appear once released")

	require.NoError(t, <-done)
}

// TestExecuteControlStepDefinitionOrderBatchesUntilGroupFinishes proves
// order: definition intentionally keeps completion blocks batched until the
// whole group finishes (unlike order: completion), even though start banners
// still stream live for both order settings.
func TestExecuteControlStepDefinitionOrderBatchesUntilGroupFinishes(t *testing.T) {
	stdout, stderr := initControlTestIOCapture(t)
	slowRelease := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(slowRelease) }) }
	defer release()
	parent := &schema.WorkflowStep{
		Name: "checks",
		Type: schema.TaskTypeParallel,
		ParallelOutput: &schema.ParallelOutputConfig{
			Mode:  ControlOutputGrouped,
			Order: ControlOutputDefinition,
		},
		Steps: []schema.WorkflowStep{
			{Name: "fast", Type: schema.TaskTypeShell, Command: "fast"},
			{Name: "slow", Type: schema.TaskTypeShell, Command: "slow"},
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- ExecuteControlStep(context.Background(), parent, func(_ context.Context, child *ControlChild, _ ControlChildOutput) (*ControlChildResult, error) {
			if child.Step.Name == "slow" {
				<-slowRelease
				return &ControlChildResult{Stdout: "slow output\n"}, nil
			}
			return &ControlChildResult{Stdout: "fast output\n"}, nil
		}, ControlExecutionOptions{})
	}()

	// Start banners stream live regardless of order mode.
	require.Eventually(t, func() bool {
		return strings.Contains(stderr.String(), "Running fast") && strings.Contains(stderr.String(), "Running slow")
	}, time.Second, 5*time.Millisecond)

	// fast has nothing blocking it and should have long since returned, but
	// order: definition must not print its completion block until slow also
	// finishes.
	time.Sleep(50 * time.Millisecond)
	assert.NotContains(t, stdout.String(), "fast output")

	release()
	require.NoError(t, <-done)
	assert.Contains(t, stdout.String(), "fast output")
	assert.Contains(t, stdout.String(), "slow output")
}

// TestExecuteControlStepGroupedOutputDoesNotInterleaveUnderConcurrency runs
// several children concurrently, each writing a distinguishable multi-line
// block, and asserts no other child's lines appear inside another child's
// block. Run with -race: without ExecuteControlStep's renderMu serializing
// concurrent prints, this also triggers a data race on the shared capture
// buffers.
func TestExecuteControlStepGroupedOutputDoesNotInterleaveUnderConcurrency(t *testing.T) {
	stdout, _ := initControlTestIOCapture(t)
	const childCount = 8
	const linesPerChild = 20

	steps := make([]schema.WorkflowStep, childCount)
	for i := range steps {
		name := fmt.Sprintf("child-%d", i)
		steps[i] = schema.WorkflowStep{Name: name, Type: schema.TaskTypeShell, Command: name}
	}

	parent := &schema.WorkflowStep{
		Name:           "checks",
		Type:           schema.TaskTypeParallel,
		MaxConcurrency: childCount,
		ParallelOutput: &schema.ParallelOutputConfig{Mode: ControlOutputGrouped, ShowSummary: boolPtr(false)},
		Steps:          steps,
	}

	err := ExecuteControlStep(context.Background(), parent, func(_ context.Context, child *ControlChild, _ ControlChildOutput) (*ControlChildResult, error) {
		var b strings.Builder
		for line := 0; line < linesPerChild; line++ {
			fmt.Fprintf(&b, "%s-line-%d\n", child.Step.Name, line)
		}
		return &ControlChildResult{Stdout: b.String()}, nil
	}, ControlExecutionOptions{})
	require.NoError(t, err)

	combined := stdout.String()
	for i := range steps {
		name := steps[i].Name
		start := fmt.Sprintf("%s-line-0", name)
		end := fmt.Sprintf("%s-line-%d", name, linesPerChild-1)
		startIdx := strings.Index(combined, start)
		endIdx := strings.Index(combined, end)
		require.NotEqualf(t, -1, startIdx, "missing start of %s block", name)
		require.NotEqualf(t, -1, endIdx, "missing end of %s block", name)
		block := combined[startIdx : endIdx+len(end)]
		for j := range steps {
			if j == i {
				continue
			}
			assert.NotContainsf(t, block, steps[j].Name+"-line-", "%s's block contains a line from %s", name, steps[j].Name)
		}
	}
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
	_, stderr := initControlTestIOCapture(t)
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
	// prefixed mode's contract is "raw stream, no banners" - the live start
	// hook is only wired for grouped mode, so no "Running ..." banner should
	// ever appear here.
	assert.NotContains(t, stderr.String(), "Running ")
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
	stdout, stderr := initControlTestIOCapture(t)
	failedErr := errors.New("failed")
	// Node is populated on each Result (as the real scheduler always does -
	// see skippedResult in pkg/scheduler) since renderControlChildBlock
	// resolves its display label from Result.Node, not Result.Value.
	aggregate := &scheduler.AggregateResult{
		Err: failedErr,
		Results: []scheduler.Result{
			{NodeID: "ok", Node: dependency.Node{ID: "ok"}, Status: scheduler.StatusSucceeded, Value: &ControlResult{Name: "ok", Stdout: "stdout\n", Stderr: "stderr\n"}},
			{NodeID: "failed", Node: dependency.Node{ID: "failed"}, Status: scheduler.StatusFailed, Value: &ControlResult{Name: "failed", Err: failedErr}},
			{NodeID: "skipped", Node: dependency.Node{ID: "skipped"}, Status: scheduler.StatusSkipped},
			{NodeID: "canceled", Node: dependency.Node{ID: "canceled"}, Status: scheduler.StatusFailed, Value: &ControlResult{Name: "canceled", Canceled: true}},
		},
	}

	// "ok" and "canceled" are pre-marked as already rendered (e.g. streamed
	// live by ExecuteControlStep's WithNodeCompleteHook) - renderControlOutput
	// must skip them and only flush "failed" and "skipped" ("missing" isn't in
	// resultsByID at all, so it's silently ignored either way).
	renderControlOutput(aggregate, []string{"missing", "ok", "failed", "skipped", "canceled"}, map[string]bool{"canceled": true, "ok": true}, controlOutputConfig{
		mode: ControlOutputGrouped,
	})
	assert.NotContains(t, stdout.String(), "stdout\n", "pre-rendered \"ok\" must not be printed again")
	// Banner text is markdown-rendered, so backticks around the name are
	// styling only and don't appear literally in the plain-rendered output.
	assert.Contains(t, stderr.String(), "failed failed")
	assert.Contains(t, stderr.String(), "skipped skipped")

	// A nil/empty skip set replays everything present in resultsByID.
	renderControlOutput(aggregate, []string{"ok"}, nil, controlOutputConfig{mode: ControlOutputGrouped})
	assert.Contains(t, stdout.String(), "stdout\n")

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

// controlTestStreams is a minimal iolib.Streams implementation that captures
// output into buffers instead of the real os.Stdout/os.Stderr, mirroring the
// pattern in pkg/data/data_test.go's testStreams.
type controlTestStreams struct {
	stdin  stdio.Reader
	stdout stdio.Writer
	stderr stdio.Writer
}

func (ts *controlTestStreams) Input() stdio.Reader     { return ts.stdin }
func (ts *controlTestStreams) Output() stdio.Writer    { return ts.stdout }
func (ts *controlTestStreams) Error() stdio.Writer     { return ts.stderr }
func (ts *controlTestStreams) RawOutput() stdio.Writer { return ts.stdout }
func (ts *controlTestStreams) RawError() stdio.Writer  { return ts.stderr }

// syncBuffer is a concurrency-safe bytes.Buffer wrapper. Tests that run
// ExecuteControlStep exercise worker goroutines writing live (serialized in
// production by ExecuteControlStep's renderMu), while the test goroutine
// polls the same buffer via String() - a plain bytes.Buffer isn't safe for
// that concurrent read/write pattern, so both sides share this lock instead.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// initControlTestIOCapture initializes the global data/ui writers against
// captured buffers instead of the real terminal, so tests can assert on
// rendered banner/output text.
func initControlTestIOCapture(t *testing.T) (stdout, stderr *syncBuffer) {
	t.Helper()
	stdout, stderr = &syncBuffer{}, &syncBuffer{}
	streams := &controlTestStreams{stdin: &bytes.Buffer{}, stdout: stdout, stderr: stderr}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)
	return stdout, stderr
}
