package step

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TitleHandler registration and validation are tested in ui_handlers_test.go.
// This file tests the Execute method.

func TestTitleHandler_Execute(t *testing.T) {
	handler, ok := Get("title")
	require.True(t, ok)

	t.Run("sets title with content", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "title",
			Content: "My Window Title",
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "My Window Title", result.Value)
	})

	t.Run("restores title when content is empty", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "title",
			Content: "",
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Empty(t, result.Value)
	})

	t.Run("resolves template in content", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "title",
			Content: "Atmos - {{ .steps.env.value }}",
		}
		vars := NewVariables()
		vars.Set("env", NewStepResult("production"))
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.Equal(t, "Atmos - production", result.Value)
	})

	t.Run("returns error for invalid template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "title",
			Content: "{{ .invalid",
		}
		vars := NewVariables()
		ctx := context.Background()

		_, err := handler.Execute(ctx, step, vars)
		assert.Error(t, err)
	})
}
