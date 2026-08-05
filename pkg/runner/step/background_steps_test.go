package step

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestBackgroundActionHandlers_Validate(t *testing.T) {
	wait := &WaitHandler{}
	cancel := &CancelHandler{}
	waitAll := &WaitAllHandler{}

	// wait/cancel require `for:` targets.
	require.NoError(t, wait.Validate(&schema.WorkflowStep{Name: "gate", For: []string{"svc"}}))
	require.NoError(t, cancel.Validate(&schema.WorkflowStep{Name: "drop", For: []string{"svc"}}))

	for _, tc := range []struct {
		name string
		err  error
	}{
		{"wait without for", wait.Validate(&schema.WorkflowStep{Name: "gate"})},
		{"cancel without for", cancel.Validate(&schema.WorkflowStep{Name: "drop"})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, tc.err)
			assert.ErrorIs(t, tc.err, schema.ErrWorkflowControlStepInvalid)
			assert.Contains(t, tc.err.Error(), "requires `for:`")
		})
	}

	// wait-all takes no targets.
	require.NoError(t, waitAll.Validate(&schema.WorkflowStep{Name: "gate"}))
	require.NoError(t, waitAll.Validate(&schema.WorkflowStep{Name: "gate", For: []string{"ignored"}}))
}

// TestBackgroundActionHandlers_Execute verifies the handlers refuse to run outside
// the workflow executor (which owns the run-scoped background registry).
func TestBackgroundActionHandlers_Execute(t *testing.T) {
	ctx := context.Background()
	step := &schema.WorkflowStep{Name: "gate", For: []string{"svc"}}

	for _, tc := range []struct {
		name string
		run  func() (*StepResult, error)
	}{
		{"wait", func() (*StepResult, error) { return (&WaitHandler{}).Execute(ctx, step, nil) }},
		{"wait-all", func() (*StepResult, error) { return (&WaitAllHandler{}).Execute(ctx, step, nil) }},
		{"cancel", func() (*StepResult, error) { return (&CancelHandler{}).Execute(ctx, step, nil) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tc.run()
			assert.Nil(t, res)
			require.Error(t, err)
			assert.ErrorIs(t, err, schema.ErrWorkflowControlStepInvalid)
			assert.Contains(t, err.Error(), "workflow executor context")
		})
	}
}

// TestBackgroundActionHandlers_Registered confirms the action step types resolve
// through the step registry (populated by init).
func TestBackgroundActionHandlers_Registered(t *testing.T) {
	for _, name := range []string{schema.TaskTypeWait, schema.TaskTypeWaitAll, schema.TaskTypeCancel} {
		h, ok := Get(name)
		require.True(t, ok, "step type %q must be registered", name)
		assert.Equal(t, name, h.GetName())
	}
}
