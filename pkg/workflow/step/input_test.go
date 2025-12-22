package step

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// InputHandler registration and basic validation are tested in interactive_handlers_test.go.
// This file tests helper methods.

func TestInputHandler_ResolveOptionalValue(t *testing.T) {
	handler, ok := Get("input")
	require.True(t, ok)
	inputHandler := handler.(*InputHandler)

	t.Run("empty value returns empty", func(t *testing.T) {
		step := &schema.WorkflowStep{Name: "test"}
		vars := NewVariables()
		ctx := context.Background()

		result, err := inputHandler.resolveOptionalValue(ctx, step, vars, "", "default")
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("static value", func(t *testing.T) {
		step := &schema.WorkflowStep{Name: "test"}
		vars := NewVariables()
		ctx := context.Background()

		result, err := inputHandler.resolveOptionalValue(ctx, step, vars, "static_value", "default")
		require.NoError(t, err)
		assert.Equal(t, "static_value", result)
	})

	t.Run("template value", func(t *testing.T) {
		step := &schema.WorkflowStep{Name: "test"}
		vars := NewVariables()
		vars.Set("env", NewStepResult("production"))
		ctx := context.Background()

		result, err := inputHandler.resolveOptionalValue(ctx, step, vars, "{{ .steps.env.value }}", "default")
		require.NoError(t, err)
		assert.Equal(t, "production", result)
	})

	t.Run("invalid template returns error", func(t *testing.T) {
		step := &schema.WorkflowStep{Name: "test"}
		vars := NewVariables()
		ctx := context.Background()

		_, err := inputHandler.resolveOptionalValue(ctx, step, vars, "{{ .steps.invalid.value", "default")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to resolve default")
	})
}
