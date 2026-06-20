package workflow

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/cloudposse/atmos/pkg/scheduler"
	"github.com/cloudposse/atmos/pkg/schema"
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
