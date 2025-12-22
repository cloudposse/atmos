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

var markdownTestInitOnce sync.Once

// initMarkdownTestIO initializes the I/O context for markdown tests.
func initMarkdownTestIO(t *testing.T) {
	t.Helper()
	markdownTestInitOnce.Do(func() {
		ioCtx, err := iolib.NewContext()
		require.NoError(t, err)
		ui.InitFormatter(ioCtx)
	})
}

// MarkdownHandler registration and validation are tested in ui_handlers_test.go.
// This file tests the Execute method.

func TestMarkdownHandler_Execute(t *testing.T) {
	initMarkdownTestIO(t)

	handler, ok := Get("markdown")
	require.True(t, ok)

	t.Run("executes with simple content", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "markdown",
			Content: "# Hello World",
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "# Hello World", result.Value)
	})

	t.Run("resolves template in content", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "markdown",
			Content: "# Hello {{ .steps.name.value }}",
		}
		vars := NewVariables()
		vars.Set("name", NewStepResult("World"))
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "# Hello World", result.Value)
	})

	t.Run("returns error for invalid template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "markdown",
			Content: "{{ .invalid",
		}
		vars := NewVariables()
		ctx := context.Background()

		_, err := handler.Execute(ctx, step, vars)
		assert.Error(t, err)
	})

	t.Run("handles complex markdown", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Type: "markdown",
			Content: `# Title
## Subtitle

- Item 1
- Item 2

**Bold** and *italic*`,
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Contains(t, result.Value, "# Title")
	})

	t.Run("handles empty content", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "markdown",
			Content: "",
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Empty(t, result.Value)
	})
}
