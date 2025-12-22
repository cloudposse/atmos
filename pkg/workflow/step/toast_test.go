package step

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

var toastTestInitOnce sync.Once

// initToastTestIO initializes the I/O context for toast tests.
func initToastTestIO(t *testing.T) {
	t.Helper()
	toastTestInitOnce.Do(func() {
		ioCtx, err := iolib.NewContext()
		require.NoError(t, err)
		ui.InitFormatter(ioCtx)
	})
}

// ToastHandler registration and validation are tested in ui_handlers_test.go.
// This file tests the Execute method with different levels.

func TestToastHandler_Execute(t *testing.T) {
	initToastTestIO(t)

	handler, ok := Get("toast")
	require.True(t, ok)

	t.Run("executes with success level", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "toast",
			Content: "Operation successful",
			Level:   "success",
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "Operation successful", result.Value)
	})

	t.Run("executes with warning level", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "toast",
			Content: "This is a warning",
			Level:   "warning",
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "This is a warning", result.Value)
	})

	t.Run("executes with warn level alias", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "toast",
			Content: "This is also a warning",
			Level:   "warn",
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "This is also a warning", result.Value)
	})

	t.Run("executes with error level", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "toast",
			Content: "An error occurred",
			Level:   "error",
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "An error occurred", result.Value)
	})

	t.Run("executes with info level", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "toast",
			Content: "Information message",
			Level:   "info",
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "Information message", result.Value)
	})

	t.Run("defaults to info when level is empty", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "toast",
			Content: "Default level message",
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "Default level message", result.Value)
	})

	t.Run("defaults to info for unknown level", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "toast",
			Content: "Unknown level message",
			Level:   "unknown",
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "Unknown level message", result.Value)
	})

	t.Run("resolves template in content", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "toast",
			Content: "Hello {{ .steps.name.value }}",
			Level:   "success",
		}
		vars := NewVariables()
		vars.Set("name", NewStepResult("World"))
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.Equal(t, "Hello World", result.Value)
	})

	t.Run("returns error for invalid template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "toast",
			Content: "{{ .invalid",
		}
		vars := NewVariables()
		ctx := context.Background()

		_, err := handler.Execute(ctx, step, vars)
		assert.Error(t, err)
	})

	t.Run("handles case insensitive level", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "toast",
			Content: "Uppercase level",
			Level:   "SUCCESS",
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.Equal(t, "Uppercase level", result.Value)
	})
}
