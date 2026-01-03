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

var styleTestInitOnce sync.Once

// initStyleTestIO initializes the I/O context for style tests.
func initStyleTestIO(t *testing.T) {
	t.Helper()
	styleTestInitOnce.Do(func() {
		ioCtx, err := iolib.NewContext()
		require.NoError(t, err)
		ui.InitFormatter(ioCtx)
	})
}

// StyleHandler registration and validation are tested in output_handlers_test.go.
// This file tests helper methods.

func TestStyleHandler_BuildStyle(t *testing.T) {
	handler, ok := Get("style")
	require.True(t, ok)
	styleHandler := handler.(*StyleHandler)

	t.Run("empty step returns default style", func(t *testing.T) {
		step := &schema.WorkflowStep{Name: "test"}
		style := styleHandler.buildStyle(step)
		// Default style should exist but have no specific attributes.
		assert.NotNil(t, style)
	})

	t.Run("foreground color", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:       "test",
			Foreground: "#FF0000",
		}
		style := styleHandler.buildStyle(step)
		rendered := style.Render("test")
		// Style should be applied (ANSI codes present).
		assert.NotEmpty(t, rendered)
	})

	t.Run("background color", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:       "test",
			Background: "#00FF00",
		}
		style := styleHandler.buildStyle(step)
		rendered := style.Render("test")
		assert.NotEmpty(t, rendered)
	})

	t.Run("border styles", func(t *testing.T) {
		borders := []string{"normal", "thick", "double", "hidden", "rounded"}
		for _, border := range borders {
			step := &schema.WorkflowStep{
				Name:   "test",
				Border: border,
			}
			style := styleHandler.buildStyle(step)
			rendered := style.Render("content")
			assert.NotEmpty(t, rendered, "border style %s", border)
		}
	})

	t.Run("border with colors", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:             "test",
			Border:           "rounded",
			BorderForeground: "#FF0000",
			BorderBackground: "#0000FF",
		}
		style := styleHandler.buildStyle(step)
		rendered := style.Render("test")
		assert.NotEmpty(t, rendered)
	})

	t.Run("border none is ignored", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:   "test",
			Border: "none",
		}
		style := styleHandler.buildStyle(step)
		rendered := style.Render("test")
		// Should just have the text without border.
		assert.Contains(t, rendered, "test")
	})

	t.Run("padding single value", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Padding: "2",
		}
		style := styleHandler.buildStyle(step)
		rendered := style.Render("x")
		// Padding adds spaces around content.
		assert.NotEmpty(t, rendered)
	})

	t.Run("margin single value", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:   "test",
			Margin: "1",
		}
		style := styleHandler.buildStyle(step)
		rendered := style.Render("x")
		assert.NotEmpty(t, rendered)
	})

	t.Run("dimensions", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:   "test",
			Width:  40,
			Height: 5,
		}
		style := styleHandler.buildStyle(step)
		rendered := style.Render("x")
		assert.NotEmpty(t, rendered)
	})

	t.Run("alignment center", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:  "test",
			Align: "center",
			Width: 20,
		}
		style := styleHandler.buildStyle(step)
		rendered := style.Render("x")
		assert.NotEmpty(t, rendered)
	})

	t.Run("alignment right", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:  "test",
			Align: "right",
			Width: 20,
		}
		style := styleHandler.buildStyle(step)
		rendered := style.Render("x")
		assert.NotEmpty(t, rendered)
	})

	t.Run("alignment left", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:  "test",
			Align: "left",
			Width: 20,
		}
		style := styleHandler.buildStyle(step)
		rendered := style.Render("x")
		assert.NotEmpty(t, rendered)
	})

	t.Run("text decorations", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:          "test",
			Bold:          true,
			Italic:        true,
			Underline:     true,
			Strikethrough: true,
			Faint:         true,
		}
		style := styleHandler.buildStyle(step)
		rendered := style.Render("decorated")
		assert.NotEmpty(t, rendered)
	})

	t.Run("bold only", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Bold: true,
		}
		style := styleHandler.buildStyle(step)
		rendered := style.Render("bold text")
		assert.NotEmpty(t, rendered)
	})
}

// TestGetBorderStyle is defined in output_handlers_test.go.
// TestParseSpacing is defined in output_handlers_test.go.

func TestStyleHandler_RenderMarkdown(t *testing.T) {
	handler, ok := Get("style")
	require.True(t, ok)
	styleHandler := handler.(*StyleHandler)

	t.Run("renders simple markdown", func(t *testing.T) {
		result, err := styleHandler.renderMarkdown("# Hello", 0)
		require.NoError(t, err)
		assert.Contains(t, result, "Hello")
	})

	t.Run("renders with width constraint", func(t *testing.T) {
		longText := "This is a very long line that should be wrapped when width is constrained"
		result, err := styleHandler.renderMarkdown(longText, 20)
		require.NoError(t, err)
		assert.NotEmpty(t, result)
	})

	t.Run("renders bold text", func(t *testing.T) {
		result, err := styleHandler.renderMarkdown("**bold**", 0)
		require.NoError(t, err)
		assert.Contains(t, result, "bold")
	})

	t.Run("renders list", func(t *testing.T) {
		result, err := styleHandler.renderMarkdown("- item1\n- item2", 0)
		require.NoError(t, err)
		assert.Contains(t, result, "item1")
		assert.Contains(t, result, "item2")
	})

	t.Run("renders empty content", func(t *testing.T) {
		result, err := styleHandler.renderMarkdown("", 0)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("zero width uses default", func(t *testing.T) {
		result, err := styleHandler.renderMarkdown("test content", 0)
		require.NoError(t, err)
		// Glamour may split "test" and "content" with ANSI codes between words.
		assert.Contains(t, result, "test")
		assert.Contains(t, result, "content")
	})
}

func TestStyleHandler_Execute(t *testing.T) {
	initStyleTestIO(t)

	handler, ok := Get("style")
	require.True(t, ok)

	t.Run("executes with simple content", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "style",
			Content: "Hello World",
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "Hello World", result.Value)
	})

	t.Run("executes with template content", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "style",
			Content: "Hello {{ .steps.name.value }}",
		}
		vars := NewVariables()
		vars.Set("name", NewStepResult("World"))
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.Equal(t, "Hello World", result.Value)
	})

	t.Run("executes with markdown enabled", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:     "test",
			Type:     "style",
			Content:  "# Hello **World**",
			Markdown: true,
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		// Result should contain the rendered text (without markdown syntax).
		assert.Contains(t, result.Value, "Hello")
		assert.Contains(t, result.Value, "World")
	})

	t.Run("executes with markdown and width", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:     "test",
			Type:     "style",
			Content:  "Short text",
			Markdown: true,
			Width:    40,
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.Contains(t, result.Value, "Short")
	})

	t.Run("executes with markdown, border, and padding", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:     "test",
			Type:     "style",
			Content:  "Content",
			Markdown: true,
			Width:    50,
			Border:   "rounded",
			Padding:  "1 2",
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.Contains(t, result.Value, "Content")
	})

	t.Run("executes with styling options", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:       "test",
			Type:       "style",
			Content:    "Styled text",
			Foreground: "#FF0000",
			Bold:       true,
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.Equal(t, "Styled text", result.Value)
	})

	t.Run("returns error for invalid template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "style",
			Content: "{{ .invalid",
		}
		vars := NewVariables()
		ctx := context.Background()

		_, err := handler.Execute(ctx, step, vars)
		assert.Error(t, err)
	})

	t.Run("executes with border and no markdown", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "style",
			Content: "Bordered content",
			Border:  "double",
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.Equal(t, "Bordered content", result.Value)
	})
}
