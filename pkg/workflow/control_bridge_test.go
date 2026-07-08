package workflow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestControlBridge_ParallelThroughRegistry proves that a parallel step runs its
// children when dispatched through the step registry (as custom commands and
// lifecycle hooks do), not only through the legacy `atmos workflow` executor.
// The registration happens in control_bridge.go's init, so simply linking
// pkg/workflow wires the seam.
func TestControlBridge_ParallelThroughRegistry(t *testing.T) {
	initControlTestIO(t)

	handler, ok := stepPkg.Get(schema.TaskTypeParallel)
	require.True(t, ok)

	vars := stepPkg.NewVariables()
	step := &schema.WorkflowStep{
		Name: "fanout",
		Type: schema.TaskTypeParallel,
		Steps: []schema.WorkflowStep{
			{Name: "a", Type: schema.TaskTypeShell, Command: "echo aaa"},
			{Name: "b", Type: schema.TaskTypeShell, Command: "echo bbb"},
		},
	}
	require.NoError(t, handler.Validate(step))

	res, err := handler.Execute(context.Background(), step, vars)
	require.NoError(t, err)
	require.NotNil(t, res)

	// Both children executed and their captured output was stored for downstream
	// reference (default output mode is grouped, which captures stdout).
	a, okA := vars.GetValue("a")
	require.True(t, okA, "child 'a' result must be stored")
	assert.Equal(t, "aaa", a)
	b, okB := vars.GetValue("b")
	require.True(t, okB, "child 'b' result must be stored")
	assert.Equal(t, "bbb", b)
}

// TestControlBridge_ParallelFailurePropagates confirms a failing child surfaces
// as an error from the registry-dispatched parallel step.
func TestControlBridge_ParallelFailurePropagates(t *testing.T) {
	initControlTestIO(t)

	handler, ok := stepPkg.Get(schema.TaskTypeParallel)
	require.True(t, ok)

	vars := stepPkg.NewVariables()
	step := &schema.WorkflowStep{
		Name: "fanout",
		Type: schema.TaskTypeParallel,
		Steps: []schema.WorkflowStep{
			{Name: "ok", Type: schema.TaskTypeShell, Command: "echo fine"},
			{Name: "boom", Type: schema.TaskTypeShell, Command: "exit 7"},
		},
	}

	_, err := handler.Execute(context.Background(), step, vars)
	require.Error(t, err)
}

// TestControlBridge_MatrixThroughRegistry proves a matrix step expands and runs
// its rows through the registry seam.
func TestControlBridge_MatrixThroughRegistry(t *testing.T) {
	initControlTestIO(t)

	handler, ok := stepPkg.Get(schema.TaskTypeMatrix)
	require.True(t, ok)

	vars := stepPkg.NewVariables()
	step := &schema.WorkflowStep{
		Name:   "grid",
		Type:   schema.TaskTypeMatrix,
		Matrix: map[string][]string{"env": {"dev", "prod"}},
		Steps: []schema.WorkflowStep{
			{Name: "greet", Type: schema.TaskTypeShell, Command: "echo hi"},
		},
	}
	require.NoError(t, handler.Validate(step))

	res, err := handler.Execute(context.Background(), step, vars)
	require.NoError(t, err)
	require.NotNil(t, res)
}

// TestControlBridge_InteractiveChildRejected confirms the RequiresTTY gate: an
// interactive child cannot run inside a parallel step.
func TestControlBridge_InteractiveChildRejected(t *testing.T) {
	handler, ok := stepPkg.Get(schema.TaskTypeParallel)
	require.True(t, ok)

	// `input` is a registered interactive (TTY-requiring) step type.
	if h, ok := stepPkg.Get("input"); !ok || !h.RequiresTTY() {
		t.Skip("input step type not registered as interactive in this build")
	}

	step := &schema.WorkflowStep{
		Name: "fanout",
		Type: schema.TaskTypeParallel,
		Steps: []schema.WorkflowStep{
			{Name: "ask", Type: "input", Prompt: "name?"},
		},
	}
	err := handler.Validate(step)
	require.Error(t, err)
	assert.ErrorIs(t, err, schema.ErrWorkflowControlStepInvalid)
}
