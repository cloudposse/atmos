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

var formatTestInitOnce sync.Once

// initFormatTestIO initializes the I/O context for format tests.
func initFormatTestIO(t *testing.T) {
	t.Helper()
	formatTestInitOnce.Do(func() {
		ioCtx, err := iolib.NewContext()
		require.NoError(t, err)
		ui.InitFormatter(ioCtx)
	})
}

// FormatHandler registration and validation are tested in output_handlers_test.go.
// This file tests the Execute method.

func TestFormatHandler_Execute(t *testing.T) {
	initFormatTestIO(t)

	handler, ok := Get("format")
	require.True(t, ok)

	t.Run("executes with simple content", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "format",
			Content: "Hello World",
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "Hello World", result.Value)
	})

	t.Run("resolves template in content", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "format",
			Content: "Hello {{ .steps.name.value }}",
		}
		vars := NewVariables()
		vars.Set("name", NewStepResult("World"))
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "Hello World", result.Value)
	})

	t.Run("returns error for invalid template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "format",
			Content: "{{ .invalid",
		}
		vars := NewVariables()
		ctx := context.Background()

		_, err := handler.Execute(ctx, step, vars)
		assert.Error(t, err)
	})

	t.Run("handles multiline content", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Type: "format",
			Content: `Line 1
Line 2
Line 3`,
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.Contains(t, result.Value, "Line 1")
		assert.Contains(t, result.Value, "Line 2")
		assert.Contains(t, result.Value, "Line 3")
	})

	t.Run("resolves env variables", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "format",
			Content: "Env: {{ .env.CUSTOM_VAR }}",
		}
		vars := NewVariables()
		vars.SetEnv("CUSTOM_VAR", "custom_value")
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.Equal(t, "Env: custom_value", result.Value)
	})

	t.Run("handles empty content", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "format",
			Content: "",
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.Empty(t, result.Value)
	})
}
