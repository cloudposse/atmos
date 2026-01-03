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

var alertTestInitOnce sync.Once

// initAlertTestIO initializes the I/O context for alert tests.
func initAlertTestIO(t *testing.T) {
	t.Helper()
	alertTestInitOnce.Do(func() {
		ioCtx, err := iolib.NewContext()
		require.NoError(t, err)
		ui.InitFormatter(ioCtx)
	})
}

// AlertHandler registration and validation are tested in ui_handlers_test.go.
// This file tests the Execute method.

func TestAlertHandler_Execute(t *testing.T) {
	initAlertTestIO(t)

	handler, ok := Get("alert")
	require.True(t, ok)

	t.Run("executes without content", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Type: "alert",
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Empty(t, result.Value)
	})

	t.Run("executes with content", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "alert",
			Content: "Alert message",
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "Alert message", result.Value)
	})

	t.Run("resolves template in content", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "alert",
			Content: "Alert: {{ .steps.status.value }}",
		}
		vars := NewVariables()
		vars.Set("status", NewStepResult("Complete"))
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.Equal(t, "Alert: Complete", result.Value)
	})

	t.Run("returns error for invalid template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "alert",
			Content: "{{ .invalid",
		}
		vars := NewVariables()
		ctx := context.Background()

		_, err := handler.Execute(ctx, step, vars)
		assert.Error(t, err)
	})
}
