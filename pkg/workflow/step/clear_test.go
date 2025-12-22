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

var clearTestInitOnce sync.Once

// initClearTestIO initializes the I/O context for clear tests.
func initClearTestIO(t *testing.T) {
	t.Helper()
	clearTestInitOnce.Do(func() {
		ioCtx, err := iolib.NewContext()
		require.NoError(t, err)
		ui.InitFormatter(ioCtx)
	})
}

// ClearHandler registration and validation are tested in ui_handlers_test.go.
// This file tests the Execute method.

func TestClearHandler_Execute(t *testing.T) {
	initClearTestIO(t)

	handler, ok := Get("clear")
	require.True(t, ok)

	t.Run("executes successfully", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Type: "clear",
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Empty(t, result.Value)
	})

	t.Run("returns empty result value", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Type: "clear",
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.Equal(t, "", result.Value)
	})
}
