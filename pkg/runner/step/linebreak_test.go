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

var linebreakTestInitOnce sync.Once

// initLinebreakTestIO initializes the I/O context for linebreak tests.
func initLinebreakTestIO(t *testing.T) {
	t.Helper()
	linebreakTestInitOnce.Do(func() {
		ioCtx, err := iolib.NewContext()
		require.NoError(t, err)
		ui.InitFormatter(ioCtx)
	})
}

// LinebreakHandler registration is tested in ui_handlers_test.go.
// This file tests the Validate and Execute methods.

func TestLinebreakHandler_Validate(t *testing.T) {
	handler, ok := Get("linebreak")
	require.True(t, ok)

	t.Run("validates with no count", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Type: "linebreak",
		}
		err := handler.Validate(step)
		assert.NoError(t, err)
	})

	t.Run("validates with count", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:  "test",
			Type:  "linebreak",
			Count: 5,
		}
		err := handler.Validate(step)
		assert.NoError(t, err)
	})

	t.Run("validates with zero count", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:  "test",
			Type:  "linebreak",
			Count: 0,
		}
		err := handler.Validate(step)
		assert.NoError(t, err)
	})

	t.Run("validates with negative count", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:  "test",
			Type:  "linebreak",
			Count: -1,
		}
		err := handler.Validate(step)
		// Negative count should still pass validation - it defaults to 1 in Execute.
		assert.NoError(t, err)
	})
}

func TestLinebreakHandler_Execute(t *testing.T) {
	initLinebreakTestIO(t)

	handler, ok := Get("linebreak")
	require.True(t, ok)

	t.Run("executes with default count", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Type: "linebreak",
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Empty(t, result.Value)
	})

	t.Run("executes with specified count", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:  "test",
			Type:  "linebreak",
			Count: 3,
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Empty(t, result.Value)
	})

	t.Run("executes with zero count defaults to 1", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:  "test",
			Type:  "linebreak",
			Count: 0,
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Empty(t, result.Value)
	})

	t.Run("executes with negative count defaults to 1", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:  "test",
			Type:  "linebreak",
			Count: -5,
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Empty(t, result.Value)
	})

	t.Run("executes with large count", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:  "test",
			Type:  "linebreak",
			Count: 10,
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Empty(t, result.Value)
	})
}
