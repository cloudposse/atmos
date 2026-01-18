package step

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// JoinHandler registration and validation are tested in output_handlers_test.go.
// This file tests the Execute method.

func TestJoinHandler_Execute(t *testing.T) {
	handler, ok := Get("join")
	require.True(t, ok)

	t.Run("joins options with default separator", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "join",
			Options: []string{"line1", "line2", "line3"},
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.Equal(t, "line1\nline2\nline3", result.Value)
	})

	t.Run("joins options with custom separator", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:      "test",
			Type:      "join",
			Options:   []string{"a", "b", "c"},
			Separator: ", ",
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.Equal(t, "a, b, c", result.Value)
	})

	t.Run("joins options with empty separator", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:      "test",
			Type:      "join",
			Options:   []string{"a", "b", "c"},
			Separator: "",
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		// Empty separator means default newline.
		assert.Equal(t, "a\nb\nc", result.Value)
	})

	t.Run("resolves templates in options", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "join",
			Options: []string{"{{ .steps.first.value }}", "middle", "{{ .steps.last.value }}"},
		}
		vars := NewVariables()
		vars.Set("first", NewStepResult("start"))
		vars.Set("last", NewStepResult("end"))
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.Equal(t, "start\nmiddle\nend", result.Value)
	})

	t.Run("uses content when no options", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "join",
			Content: "direct content",
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.Equal(t, "direct content", result.Value)
	})

	t.Run("resolves template in content", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "join",
			Content: "Value is {{ .steps.val.value }}",
		}
		vars := NewVariables()
		vars.Set("val", NewStepResult("42"))
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.Equal(t, "Value is 42", result.Value)
	})

	t.Run("returns error for invalid option template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "join",
			Options: []string{"valid", "{{ .invalid"},
		}
		vars := NewVariables()
		ctx := context.Background()

		_, err := handler.Execute(ctx, step, vars)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
	})

	t.Run("returns error for invalid content template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "join",
			Content: "{{ .invalid",
		}
		vars := NewVariables()
		ctx := context.Background()

		_, err := handler.Execute(ctx, step, vars)
		assert.Error(t, err)
	})

	t.Run("handles single option", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "join",
			Options: []string{"single"},
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.Equal(t, "single", result.Value)
	})

	t.Run("handles space separator", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:      "test",
			Type:      "join",
			Options:   []string{"hello", "world"},
			Separator: " ",
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.Equal(t, "hello world", result.Value)
	})
}
