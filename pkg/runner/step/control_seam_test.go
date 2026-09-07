package step

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestValidateControlChildrenNonInteractive(t *testing.T) {
	t.Run("empty child type defaults to shell and is accepted", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "fanout",
			Type: schema.TaskTypeParallel,
			Steps: []schema.WorkflowStep{
				{Name: "child", Command: "echo hi"},
			},
		}
		require.NoError(t, validateControlChildrenNonInteractive(step))
	})

	t.Run("unregistered child type is accepted (no handler to check RequiresTTY)", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "fanout",
			Type: schema.TaskTypeParallel,
			Steps: []schema.WorkflowStep{
				{Name: "child", Type: "not-a-registered-step-type"},
			},
		}
		require.NoError(t, validateControlChildrenNonInteractive(step))
	})

	t.Run("interactive child type is rejected", func(t *testing.T) {
		handler, ok := Get("input")
		require.True(t, ok, "input step type must be registered")
		require.True(t, handler.RequiresTTY(), "input step type must require TTY")

		step := &schema.WorkflowStep{
			Name: "fanout",
			Type: schema.TaskTypeParallel,
			Steps: []schema.WorkflowStep{
				{Name: "ask", Type: "input", Prompt: "name?"},
			},
		}
		err := validateControlChildrenNonInteractive(step)
		require.Error(t, err)
		assert.ErrorIs(t, err, schema.ErrWorkflowControlStepInvalid)
	})

	t.Run("no children is accepted", func(t *testing.T) {
		step := &schema.WorkflowStep{Name: "fanout", Type: schema.TaskTypeParallel}
		require.NoError(t, validateControlChildrenNonInteractive(step))
	})
}
