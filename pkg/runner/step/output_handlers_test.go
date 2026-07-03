package step

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

var initOnce sync.Once

// initTestIO initializes the I/O context and data writer for tests that need it.
func initTestIO(t *testing.T) {
	t.Helper()
	initOnce.Do(func() {
		ioCtx, err := iolib.NewContext()
		require.NoError(t, err)
		data.InitWriter(ioCtx)
		ui.InitFormatter(ioCtx)
	})
}

func TestOutputHandlersRegistration(t *testing.T) {
	// Verify all output handlers are registered.
	tests := []struct {
		name        string
		category    StepCategory
		requiresTTY bool
	}{
		{"spin", CategoryOutput, false},
		{"table", CategoryOutput, false},
		{"pager", CategoryOutput, false},
		{"format", CategoryOutput, false},
		{"join", CategoryOutput, false},
		{"style", CategoryOutput, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, ok := Get(tt.name)
			require.True(t, ok, "handler %s should be registered", tt.name)
			assert.Equal(t, tt.name, handler.GetName())
			assert.Equal(t, tt.category, handler.GetCategory())
			assert.Equal(t, tt.requiresTTY, handler.RequiresTTY(), "output handlers should not require TTY")
		})
	}
}

func TestSpinHandlerValidation(t *testing.T) {
	handler, ok := Get("spin")
	require.True(t, ok)

	tests := []struct {
		name      string
		step      *schema.WorkflowStep
		expectErr bool
	}{
		{
			name: "valid with title and command",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "spin",
				Title:   "Building...",
				Command: "make build",
			},
			expectErr: false,
		},
		{
			name: "missing title",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "spin",
				Command: "make build",
			},
			expectErr: true,
		},
		{
			name: "missing command",
			step: &schema.WorkflowStep{
				Name:  "test",
				Type:  "spin",
				Title: "Building...",
			},
			expectErr: true,
		},
		{
			name: "with timeout",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "spin",
				Title:   "Building...",
				Command: "make build",
				Timeout: "5m",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.Validate(tt.step)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTableHandlerValidation(t *testing.T) {
	handler, ok := Get("table")
	require.True(t, ok)

	tests := []struct {
		name      string
		step      *schema.WorkflowStep
		expectErr bool
	}{
		{
			name: "valid with data",
			step: &schema.WorkflowStep{
				Name: "test",
				Type: "table",
				Data: []map[string]any{
					{"Name": "vpc", "Status": "deployed"},
				},
			},
			expectErr: false,
		},
		{
			name: "valid with content",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "table",
				Content: "pre-formatted table",
			},
			expectErr: false,
		},
		{
			name: "missing data and content",
			step: &schema.WorkflowStep{
				Name: "test",
				Type: "table",
			},
			expectErr: true,
		},
		{
			name: "with columns",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "table",
				Columns: []string{"Name", "Status"},
				Data: []map[string]any{
					{"Name": "vpc", "Status": "deployed"},
				},
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.Validate(tt.step)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

//nolint:dupl // Similar test patterns for different handlers.
func TestPagerHandlerValidation(t *testing.T) {
	handler, ok := Get("pager")
	require.True(t, ok)

	tests := []struct {
		name      string
		step      *schema.WorkflowStep
		expectErr bool
	}{
		{
			name: "valid with content",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "pager",
				Content: "Long content to display in pager",
			},
			expectErr: false,
		},
		{
			name: "missing content",
			step: &schema.WorkflowStep{
				Name: "test",
				Type: "pager",
			},
			expectErr: true,
		},
		{
			name: "with title",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "pager",
				Title:   "View Output",
				Content: "Content here",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.Validate(tt.step)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFormatHandlerValidation(t *testing.T) {
	handler, ok := Get("format")
	require.True(t, ok)

	tests := []struct {
		name      string
		step      *schema.WorkflowStep
		expectErr bool
	}{
		{
			name: "valid with content",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "format",
				Content: "Environment: {{ .steps.env.value }}",
			},
			expectErr: false,
		},
		{
			name: "missing content",
			step: &schema.WorkflowStep{
				Name: "test",
				Type: "format",
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.Validate(tt.step)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestJoinHandlerValidation(t *testing.T) {
	handler, ok := Get("join")
	require.True(t, ok)

	tests := []struct {
		name      string
		step      *schema.WorkflowStep
		expectErr bool
	}{
		{
			name: "valid with options",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "join",
				Options: []string{"line1", "line2", "line3"},
			},
			expectErr: false,
		},
		{
			name: "valid with content",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "join",
				Content: "{{ .steps.header.value }}\n{{ .steps.body.value }}",
			},
			expectErr: false,
		},
		{
			name: "missing options and content",
			step: &schema.WorkflowStep{
				Name: "test",
				Type: "join",
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.Validate(tt.step)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestJoinHandlerExecution(t *testing.T) {
	handler, ok := Get("join")
	require.True(t, ok)

	tests := []struct {
		name     string
		step     *schema.WorkflowStep
		expected string
	}{
		{
			name: "join with default separator (newline)",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "join",
				Options: []string{"line1", "line2", "line3"},
			},
			expected: "line1\nline2\nline3",
		},
		{
			name: "join with custom separator",
			step: &schema.WorkflowStep{
				Name:      "test",
				Type:      "join",
				Options:   []string{"a", "b", "c"},
				Separator: ", ",
			},
			expected: "a, b, c",
		},
		{
			name: "join with empty separator",
			step: &schema.WorkflowStep{
				Name:      "test",
				Type:      "join",
				Options:   []string{"foo", "bar"},
				Separator: "",
			},
			expected: "foo\nbar", // Empty separator falls back to newline.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vars := NewVariables()
			result, err := handler.Execute(context.Background(), tt.step, vars)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.Value)
		})
	}
}

func TestStyleHandlerValidation(t *testing.T) {
	handler, ok := Get("style")
	require.True(t, ok)

	tests := []struct {
		name      string
		step      *schema.WorkflowStep
		expectErr bool
	}{
		{
			name: "valid with content",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "style",
				Content: "Styled Header",
			},
			expectErr: false,
		},
		{
			name: "missing content",
			step: &schema.WorkflowStep{
				Name: "test",
				Type: "style",
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.Validate(tt.step)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOutputHandlersByCategory(t *testing.T) {
	byCategory := ListByCategory()
	outputHandlers := byCategory[CategoryOutput]

	// Should have at least 6 output handlers.
	assert.GreaterOrEqual(t, len(outputHandlers), 6,
		"should have at least 6 output handlers (spin, table, pager, format, join, style)")

	// None should require TTY.
	for _, handler := range outputHandlers {
		assert.False(t, handler.RequiresTTY(),
			"output handler %s should not require TTY", handler.GetName())
	}
}

func TestFormatHandlerExecution(t *testing.T) {
	initTestIO(t)
	handler, ok := Get("format")
	require.True(t, ok)

	t.Run("simple content", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_format",
			Type:    "format",
			Content: "Hello, World!",
		}
		vars := NewVariables()

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Equal(t, "Hello, World!", result.Value)
	})

	t.Run("content with template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_format",
			Type:    "format",
			Content: "Environment: {{ .steps.env.value }}",
		}
		vars := NewVariables()
		vars.Set("env", NewStepResult("production"))

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Equal(t, "Environment: production", result.Value)
	})

	t.Run("content with multiple templates", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_format",
			Type:    "format",
			Content: "{{ .steps.component.value }} in {{ .steps.env.value }}",
		}
		vars := NewVariables()
		vars.Set("component", NewStepResult("vpc"))
		vars.Set("env", NewStepResult("production"))

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Equal(t, "vpc in production", result.Value)
	})

	t.Run("invalid template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_format",
			Type:    "format",
			Content: "{{ .steps.missing.value",
		}
		vars := NewVariables()

		_, err := handler.Execute(context.Background(), step, vars)
		assert.Error(t, err)
	})
}

func TestJoinHandlerExecutionWithContent(t *testing.T) {
	handler, ok := Get("join")
	require.True(t, ok)

	t.Run("content template resolution", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_join",
			Type:    "join",
			Content: "Header: {{ .steps.header.value }}\nBody: {{ .steps.body.value }}",
		}
		vars := NewVariables()
		vars.Set("header", NewStepResult("Title"))
		vars.Set("body", NewStepResult("Content"))

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Equal(t, "Header: Title\nBody: Content", result.Value)
	})

	t.Run("options with template variables", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_join",
			Type:    "join",
			Options: []string{"{{ .steps.line1.value }}", "static", "{{ .steps.line3.value }}"},
		}
		vars := NewVariables()
		vars.Set("line1", NewStepResult("first"))
		vars.Set("line3", NewStepResult("third"))

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Equal(t, "first\nstatic\nthird", result.Value)
	})

	t.Run("options with pipe separator", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:      "test_join",
			Type:      "join",
			Options:   []string{"a", "b", "c"},
			Separator: " | ",
		}
		vars := NewVariables()

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Equal(t, "a | b | c", result.Value)
	})

	t.Run("single option", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_join",
			Type:    "join",
			Options: []string{"only one"},
		}
		vars := NewVariables()

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Equal(t, "only one", result.Value)
	})
}

func TestStyleHandlerExecution(t *testing.T) {
	initTestIO(t)
	handler, ok := Get("style")
	require.True(t, ok)

	t.Run("simple content", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_style",
			Type:    "style",
			Content: "Styled Header",
		}
		vars := NewVariables()

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		// Result should contain the content.
		assert.Contains(t, result.Value, "Styled Header")
	})

	t.Run("content with template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_style",
			Type:    "style",
			Content: "Welcome {{ .steps.user.value }}",
		}
		vars := NewVariables()
		vars.Set("user", NewStepResult("Alice"))

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Contains(t, result.Value, "Welcome Alice")
	})

	t.Run("with bold style", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_style",
			Type:    "style",
			Content: "Important",
			Bold:    true,
		}
		vars := NewVariables()

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Contains(t, result.Value, "Important")
	})

	t.Run("with width", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_style",
			Type:    "style",
			Content: "Fixed Width",
			Width:   40,
		}
		vars := NewVariables()

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Contains(t, result.Value, "Fixed Width")
	})
}

func TestParseSpacing(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Spacing
	}{
		{
			name:     "single value",
			input:    "2",
			expected: Spacing{Top: 2, Right: 2, Bottom: 2, Left: 2},
		},
		{
			name:     "two values (vertical, horizontal)",
			input:    "1 2",
			expected: Spacing{Top: 1, Right: 2, Bottom: 1, Left: 2},
		},
		{
			name:     "three values (top, horizontal, bottom)",
			input:    "1 2 3",
			expected: Spacing{Top: 1, Right: 2, Bottom: 3, Left: 2},
		},
		{
			name:     "four values (top, right, bottom, left)",
			input:    "1 2 3 4",
			expected: Spacing{Top: 1, Right: 2, Bottom: 3, Left: 4},
		},
		{
			name:     "empty string",
			input:    "",
			expected: Spacing{Top: 0, Right: 0, Bottom: 0, Left: 0},
		},
		{
			name:     "invalid value defaults to zero",
			input:    "abc",
			expected: Spacing{Top: 0, Right: 0, Bottom: 0, Left: 0},
		},
		{
			name:     "mixed valid and invalid",
			input:    "1 abc 3",
			expected: Spacing{Top: 1, Right: 0, Bottom: 3, Left: 0},
		},
		{
			name:     "extra whitespace",
			input:    "  1   2  ",
			expected: Spacing{Top: 1, Right: 2, Bottom: 1, Left: 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseSpacing(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetBorderStyle(t *testing.T) {
	// Just verify no panics and basic functionality.
	tests := []string{
		"normal",
		"thick",
		"double",
		"hidden",
		"rounded",
		"",
		"unknown",
		"NORMAL", // Test case insensitivity.
	}

	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			// Should not panic.
			border := getBorderStyle(name)
			_ = border // Just verify it returns a value.
		})
	}
}

func TestStyleHandlerBuildStyle(t *testing.T) {
	initTestIO(t)
	handler, ok := Get("style")
	require.True(t, ok)
	styleHandler := handler.(*StyleHandler)

	t.Run("with foreground and background", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:       "test",
			Content:    "test",
			Foreground: "#FF0000",
			Background: "#0000FF",
		}
		style := styleHandler.buildStyle(step)
		_ = style.Render("test") // Verify no panic.
	})

	t.Run("with border and colors", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:             "test",
			Content:          "test",
			Border:           "rounded",
			BorderForeground: "#00FF00",
			BorderBackground: "#FFFFFF",
		}
		style := styleHandler.buildStyle(step)
		_ = style.Render("test")
	})

	t.Run("with padding and margin", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Content: "test",
			Padding: "1 2 3 4",
			Margin:  "2",
		}
		style := styleHandler.buildStyle(step)
		_ = style.Render("test")
	})

	t.Run("with all text decorations", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:          "test",
			Content:       "test",
			Bold:          true,
			Italic:        true,
			Underline:     true,
			Strikethrough: true,
			Faint:         true,
		}
		style := styleHandler.buildStyle(step)
		output := style.Render("test")
		assert.NotEmpty(t, output)
	})

	t.Run("with alignment center", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Content: "test",
			Align:   "center",
			Width:   20,
		}
		style := styleHandler.buildStyle(step)
		_ = style.Render("test")
	})

	t.Run("with alignment right", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Content: "test",
			Align:   "right",
			Width:   20,
		}
		style := styleHandler.buildStyle(step)
		_ = style.Render("test")
	})

	t.Run("with alignment left", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Content: "test",
			Align:   "left",
			Width:   20,
		}
		style := styleHandler.buildStyle(step)
		_ = style.Render("test")
	})

	t.Run("with height", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Content: "test",
			Height:  5,
		}
		style := styleHandler.buildStyle(step)
		_ = style.Render("test")
	})

	t.Run("with all border types", func(t *testing.T) {
		borderTypes := []string{"normal", "thick", "double", "hidden", "rounded"}
		for _, borderType := range borderTypes {
			step := &schema.WorkflowStep{
				Name:    "test",
				Content: "test",
				Border:  borderType,
			}
			style := styleHandler.buildStyle(step)
			_ = style.Render("test")
		}
	})
}

func TestTableHandlerExecution(t *testing.T) {
	initTestIO(t)
	handler, ok := Get("table")
	require.True(t, ok)

	t.Run("content table", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_table",
			Type:    "table",
			Content: "Name\tStatus\nvpc\tdeployed",
		}
		vars := NewVariables()

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Contains(t, result.Value, "vpc")
		assert.Contains(t, result.Value, "deployed")
	})

	t.Run("content table with template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_table",
			Type:    "table",
			Content: "Component\t{{ .steps.status.value }}",
		}
		vars := NewVariables()
		vars.Set("status", NewStepResult("active"))

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Contains(t, result.Value, "active")
	})

	t.Run("data table", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test_table",
			Type: "table",
			Data: []map[string]any{
				{"Name": "vpc", "Status": "deployed"},
				{"Name": "rds", "Status": "pending"},
			},
		}
		vars := NewVariables()

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Contains(t, result.Value, "vpc")
		assert.Contains(t, result.Value, "rds")
		assert.Contains(t, result.Value, "deployed")
		assert.Contains(t, result.Value, "pending")
	})

	t.Run("data table with explicit columns", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_table",
			Type:    "table",
			Columns: []string{"Name", "Status"},
			Data: []map[string]any{
				{"Name": "app", "Status": "running", "Extra": "ignored"},
			},
		}
		vars := NewVariables()

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Contains(t, result.Value, "app")
		assert.Contains(t, result.Value, "running")
		// "ignored" should not appear since it's not in columns.
	})

	t.Run("data table with title", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:  "test_table",
			Type:  "table",
			Title: "Component Status",
			Data: []map[string]any{
				{"Name": "vpc", "Status": "deployed"},
			},
		}
		vars := NewVariables()

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Contains(t, result.Value, "Component Status")
	})
}
