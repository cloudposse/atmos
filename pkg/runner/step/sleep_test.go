package step

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// SleepHandler registration is tested in ui_handlers_test.go.
// This file tests the Execute method.

func TestSleepHandler_Validate(t *testing.T) {
	handler, ok := Get("sleep")
	require.True(t, ok)

	t.Run("validates with no timeout", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Type: "sleep",
		}
		err := handler.Validate(step)
		assert.NoError(t, err)
	})

	t.Run("validates with timeout", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "sleep",
			Timeout: "1s",
		}
		err := handler.Validate(step)
		assert.NoError(t, err)
	})
}

func TestSleepHandler_Execute(t *testing.T) {
	handler, ok := Get("sleep")
	require.True(t, ok)

	t.Run("sleeps for default duration", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Type: "sleep",
		}
		vars := NewVariables()
		ctx := context.Background()

		start := time.Now()
		result, err := handler.Execute(ctx, step, vars)
		elapsed := time.Since(start)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "1s", result.Value)
		// Should take at least 1 second.
		assert.GreaterOrEqual(t, elapsed, 900*time.Millisecond)
	})

	t.Run("sleeps for specified duration", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "sleep",
			Timeout: "100ms",
		}
		vars := NewVariables()
		ctx := context.Background()

		start := time.Now()
		result, err := handler.Execute(ctx, step, vars)
		elapsed := time.Since(start)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "100ms", result.Value)
		// Should take at least 100ms but less than 500ms.
		assert.GreaterOrEqual(t, elapsed, 90*time.Millisecond)
		assert.Less(t, elapsed, 500*time.Millisecond)
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "sleep",
			Timeout: "10s",
		}
		vars := NewVariables()
		ctx, cancel := context.WithCancel(context.Background())

		// Cancel after a short delay.
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		start := time.Now()
		_, err := handler.Execute(ctx, step, vars)
		elapsed := time.Since(start)

		// Should be cancelled, not complete the full sleep.
		assert.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Less(t, elapsed, 1*time.Second)
	})

	t.Run("respects context deadline", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "sleep",
			Timeout: "10s",
		}
		vars := NewVariables()
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		start := time.Now()
		_, err := handler.Execute(ctx, step, vars)
		elapsed := time.Since(start)

		// Should hit deadline, not complete the full sleep.
		assert.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Less(t, elapsed, 1*time.Second)
	})

	t.Run("handles template in timeout", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "sleep",
			Timeout: "{{ .steps.duration.value }}",
		}
		vars := NewVariables()
		vars.Set("duration", NewStepResult("50ms"))
		ctx := context.Background()

		start := time.Now()
		result, err := handler.Execute(ctx, step, vars)
		elapsed := time.Since(start)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "50ms", result.Value)
		assert.GreaterOrEqual(t, elapsed, 40*time.Millisecond)
		assert.Less(t, elapsed, 500*time.Millisecond)
	})

	t.Run("returns error for invalid template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "sleep",
			Timeout: "{{ .steps.invalid.value",
		}
		vars := NewVariables()
		ctx := context.Background()

		_, err := handler.Execute(ctx, step, vars)
		assert.Error(t, err)
	})

	t.Run("returns error for invalid duration", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "sleep",
			Timeout: "not-a-duration",
		}
		vars := NewVariables()
		ctx := context.Background()

		_, err := handler.Execute(ctx, step, vars)
		assert.Error(t, err)
	})
}
