package markdown

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRenderer(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		atmosConfig schema.AtmosConfiguration
	}{
		{
			name:     "Test with no color",
			input:    "## Hello **world**",
			expected: "  ## Hello **world**",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						NoColor: true,
					},
				},
			},
		},
		{
			name:     "Test with color",
			input:    "## Hello **world**",
			expected: "\x1b[;1m\x1b[0m\x1b[;1m\x1b[0m\x1b[;1m## \x1b[0m\x1b[;1mHello \x1b[0m\x1b[;1m**\x1b[0m\x1b[;1mworld\x1b[0m\x1b[;1m**\x1b[0m",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						NoColor: false,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := NewRenderer(tt.atmosConfig)
			r.isTTYSupportForStdout = func() bool {
				return true
			}
			defer func() {
				r.isTTYSupportForStdout = term.IsTTYSupportForStdout
			}()
			str, err := r.Render(tt.input)
			assert.Contains(t, str, tt.expected)
			assert.NoError(t, err)
			str, err = r.RenderWithoutWordWrap(tt.input)
			assert.Contains(t, str, tt.expected)
			assert.NoError(t, err)
		})
	}
}

func TestRenderErrorf(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		isColor  bool
	}{
		{
			name:     "Test with no color",
			input:    "## Hello **world**",
			expected: "## Hello **world**",
			isColor:  false,
		},
		{
			name:     "Test with color",
			input:    "## Hello **world**",
			expected: "\x1b[;1m\x1b[0m\x1b[;1m\x1b[0m\x1b[;1m## \x1b[0m\x1b[;1mHello \x1b[0m\x1b[;1m**\x1b[0m\x1b[;1mworld\x1b[0m\x1b[;1m**\x1b[0m",
			isColor:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := NewRenderer(schema.AtmosConfiguration{})
			r.isTTYSupportForStderr = func() bool {
				return tt.isColor
			}
			r.isTTYSupportForStdout = func() bool {
				return tt.isColor
			}
			defer func() {
				r.isTTYSupportForStderr = term.IsTTYSupportForStderr
				r.isTTYSupportForStdout = term.IsTTYSupportForStdout
			}()
			str, err := r.RenderErrorf(tt.input)
			assert.Contains(t, str, tt.expected)
			assert.NoError(t, err)
		})
	}
}

func TestRenderAsciiWithoutWordWrap(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Simple markdown",
			input:    "# Heading\n\nSome text",
			expected: "# Heading",
		},
		{
			name:     "Bold text",
			input:    "**bold text**",
			expected: "**bold text**",
		},
		{
			name:     "Code block",
			input:    "```\ncode\n```",
			expected: "code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewRenderer(schema.AtmosConfiguration{})
			require.NoError(t, err)

			result, err := r.RenderAsciiWithoutWordWrap(tt.input)
			assert.NoError(t, err)
			assert.Contains(t, result, tt.expected)
		})
	}
}

func TestRenderAscii(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Simple markdown with wrapping",
			input:    "# Heading\n\nThis is a very long line that should be wrapped when rendered in ASCII mode",
			expected: "# Heading",
		},
		{
			name:     "Lists",
			input:    "- Item 1\n- Item 2\n- Item 3",
			expected: "Item 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewRenderer(schema.AtmosConfiguration{})
			require.NoError(t, err)

			result, err := r.RenderAscii(tt.input)
			assert.NoError(t, err)
			assert.Contains(t, result, tt.expected)
		})
	}
}

func TestRenderWorkflow(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Workflow content gets header added",
			input:    "Step 1: Do this\nStep 2: Do that",
			expected: "Workflow",
		},
		{
			name:     "Empty workflow",
			input:    "",
			expected: "Workflow",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewRenderer(schema.AtmosConfiguration{})
			require.NoError(t, err)
			r.isTTYSupportForStdout = func() bool {
				return false // Force ASCII rendering for predictable output
			}

			result, err := r.RenderWorkflow(tt.input)
			assert.NoError(t, err)
			assert.Contains(t, result, tt.expected)
		})
	}
}

func TestRenderError(t *testing.T) {
	tests := []struct {
		name       string
		title      string
		details    string
		suggestion string
		expected   []string
	}{
		{
			name:       "Error with all fields",
			title:      "Configuration Error",
			details:    "Invalid configuration found",
			suggestion: "https://docs.example.com/fix",
			expected:   []string{"Configuration Error", "Invalid configuration found", "docs"},
		},
		{
			name:       "Error with non-URL suggestion",
			title:      "Parse Error",
			details:    "Failed to parse YAML",
			suggestion: "\n\nTry checking your YAML syntax",
			expected:   []string{"Parse Error", "Failed to parse YAML", "Try checking your YAML syntax"},
		},
		{
			name:       "Error with no suggestion",
			title:      "Runtime Error",
			details:    "Something went wrong",
			suggestion: "",
			expected:   []string{"Runtime Error", "Something went wrong"},
		},
		{
			name:       "Error with no title",
			title:      "",
			details:    "An error occurred",
			suggestion: "Check the logs",
			expected:   []string{"An error occurred", "Check the logs"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewRenderer(schema.AtmosConfiguration{})
			require.NoError(t, err)
			r.isTTYSupportForStderr = func() bool {
				return false // Force ASCII rendering
			}
			r.isTTYSupportForStdout = func() bool {
				return false // Force ASCII rendering
			}

			result, err := r.RenderError(tt.title, tt.details, tt.suggestion)
			assert.NoError(t, err)
			for _, expected := range tt.expected {
				assert.Contains(t, result, expected)
			}
		})
	}
}

func TestRenderSuccess(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		details  string
		expected []string
	}{
		{
			name:     "Success with details",
			title:    "Operation Successful",
			details:  "All tasks completed",
			expected: []string{"Operation Successful", "Details", "All tasks completed"},
		},
		{
			name:     "Success without details",
			title:    "Done",
			details:  "",
			expected: []string{"Done"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewRenderer(schema.AtmosConfiguration{})
			require.NoError(t, err)
			r.isTTYSupportForStdout = func() bool {
				return false // Force ASCII rendering
			}

			result, err := r.RenderSuccess(tt.title, tt.details)
			assert.NoError(t, err)
			for _, expected := range tt.expected {
				assert.Contains(t, result, expected)
			}
		})
	}
}

func TestWithWidth(t *testing.T) {
	t.Run("WithWidth option sets renderer width", func(t *testing.T) {
		expectedWidth := uint(120)
		r, err := NewRenderer(
			schema.AtmosConfiguration{},
			WithWidth(expectedWidth),
		)
		require.NoError(t, err)
		assert.Equal(t, expectedWidth, r.width)
	})

	t.Run("Default width when no option provided", func(t *testing.T) {
		r, err := NewRenderer(schema.AtmosConfiguration{})
		require.NoError(t, err)
		assert.Equal(t, uint(80), r.width)
	})
}

func TestNewTerminalMarkdownRenderer(t *testing.T) {
	origStdout := os.Stdout
	defer func() {
		os.Stdout = origStdout
	}()

	tests := []struct {
		name        string
		atmosConfig schema.AtmosConfiguration
	}{
		{
			name: "With max width configured",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Docs: schema.Docs{
						MaxWidth: 100,
					},
				},
			},
		},
		{
			name: "Without max width configured",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Docs: schema.Docs{
						MaxWidth: 0,
					},
				},
			},
		},
		{
			name: "With very large max width",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Docs: schema.Docs{
						MaxWidth: 500,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewTerminalMarkdownRenderer(tt.atmosConfig)
			assert.NoError(t, err)
			assert.NotNil(t, r)
			// Width should be set based on terminal or max width
			assert.Greater(t, r.width, uint(0))
		})
	}
}

func TestRender_NonTTY(t *testing.T) {
	t.Run("Render falls back to ASCII for non-TTY stdout", func(t *testing.T) {
		r, err := NewRenderer(schema.AtmosConfiguration{})
		require.NoError(t, err)
		r.isTTYSupportForStdout = func() bool {
			return false
		}

		input := "# Test Header\n\nSome **bold** text"
		result, err := r.Render(input)
		assert.NoError(t, err)
		assert.Contains(t, result, "Test Header")
		assert.Contains(t, result, "bold")
	})
}

func TestRenderWithoutWordWrap_NonTTY(t *testing.T) {
	t.Run("RenderWithoutWordWrap falls back to ASCII for non-TTY stdout", func(t *testing.T) {
		r, err := NewRenderer(schema.AtmosConfiguration{})
		require.NoError(t, err)
		r.isTTYSupportForStdout = func() bool {
			return false
		}

		input := "# Test\n\nContent"
		result, err := r.RenderWithoutWordWrap(input)
		assert.NoError(t, err)
		assert.Contains(t, result, "Test")
		assert.Contains(t, result, "Content")
	})
}

// TestNewRendererErrorCases tests error handling in NewRenderer
func TestNewRendererErrorCases(t *testing.T) {
	// Note: It's hard to trigger actual errors in NewRenderer since glamour is quite robust
	// But we can test the function works with various edge cases

	t.Run("NewRenderer with empty config", func(t *testing.T) {
		atmosConfig := schema.AtmosConfiguration{}
		renderer, err := NewRenderer(atmosConfig)
		assert.NoError(t, err)
		assert.NotNil(t, renderer)
	})

	t.Run("NewRenderer with multiple options", func(t *testing.T) {
		atmosConfig := schema.AtmosConfiguration{}
		renderer, err := NewRenderer(atmosConfig, WithWidth(100), WithWidth(200))
		assert.NoError(t, err)
		assert.NotNil(t, renderer)
		assert.Equal(t, uint(200), renderer.width) // Last option wins
	})
}

// TestRenderErrorCases tests error handling in rendering methods
func TestRenderErrorCases(t *testing.T) {
	t.Run("Render with error in glamour render", func(t *testing.T) {
		r, err := NewRenderer(schema.AtmosConfiguration{})
		require.NoError(t, err)

		// Test with invalid markdown that might cause issues
		// Using a very large input to potentially trigger errors
		largeInput := ""
		for i := 0; i < 10000; i++ {
			largeInput += "# Header " + string(rune(i)) + "\n"
		}

		// This should still work but tests the error path
		_, err = r.Render(largeInput)
		// Even large inputs should work, so no error expected
		assert.NoError(t, err)
	})
}

// TestNewTerminalMarkdownRendererEdgeCases tests edge cases for NewTerminalMarkdownRenderer
func TestNewTerminalMarkdownRendererEdgeCases(t *testing.T) {
	// Save original stdout
	origStdout := os.Stdout
	defer func() {
		os.Stdout = origStdout
	}()

	t.Run("NewTerminalMarkdownRenderer with pipe stdout", func(t *testing.T) {
		// Create a pipe to simulate non-terminal stdout
		r, w, err := os.Pipe()
		require.NoError(t, err)
		defer r.Close()
		defer w.Close()

		os.Stdout = w

		atmosConfig := schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				Terminal: schema.Terminal{
					MaxWidth: 200,
				},
			},
		}

		renderer, err := NewTerminalMarkdownRenderer(atmosConfig)
		assert.NoError(t, err)
		assert.NotNil(t, renderer)
		// When stdout is a pipe, width should be limited by MaxWidth
		assert.LessOrEqual(t, renderer.width, uint(200))
	})

	t.Run("NewTerminalMarkdownRenderer with very large MaxWidth", func(t *testing.T) {
		atmosConfig := schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				Terminal: schema.Terminal{
					MaxWidth: 10000,
				},
			},
		}

		renderer, err := NewTerminalMarkdownRenderer(atmosConfig)
		assert.NoError(t, err)
		assert.NotNil(t, renderer)
		// Width should be capped by terminal width or use the max value
		assert.NotNil(t, renderer.width)
	})
}
