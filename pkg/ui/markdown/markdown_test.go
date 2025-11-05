package markdown

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/schema"
)

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return ansiRegex.ReplaceAllString(s, "")
}

// TestSplitMarkdownContent tests the SplitMarkdownContent function.
func TestSplitMarkdownContent(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name:     "content with details and suggestion",
			content:  "Error details here\n\nSuggestion goes here",
			expected: []string{"Error details here", "Suggestion goes here"},
		},
		{
			name:     "content with only details",
			content:  "Error details only",
			expected: []string{"Error details only"},
		},
		{
			name:     "content with multiple paragraphs",
			content:  "First paragraph\n\nSecond paragraph\n\nThird paragraph",
			expected: []string{"First paragraph", "Second paragraph\n\nThird paragraph"},
		},
		{
			name:     "content with leading empty lines",
			content:  "\n\nActual content\n\nSuggestion",
			expected: []string{"Actual content", "Suggestion"},
		},
		{
			name:     "empty content",
			content:  "",
			expected: nil,
		},
		{
			name:     "only whitespace",
			content:  "   \n\n   ",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SplitMarkdownContent(tt.content)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestRenderAsciiWithoutWordWrap tests ASCII rendering without word wrap.
func TestRenderAsciiWithoutWordWrap(t *testing.T) {
	r, err := NewRenderer(&schema.AtmosConfiguration{})
	require.NoError(t, err)

	tests := []struct {
		name     string
		content  string
		contains string
	}{
		{
			name:     "simple markdown",
			content:  "## Hello World",
			contains: "Hello World",
		},
		{
			name:     "markdown with bold",
			content:  "**Bold text**",
			contains: "Bold text",
		},
		{
			name:     "empty content",
			content:  "",
			contains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := r.RenderAsciiWithoutWordWrap(tt.content)
			assert.NoError(t, err)
			assert.Contains(t, result, tt.contains)
		})
	}
}

// TestRenderAscii tests ASCII rendering with word wrap.
func TestRenderAscii(t *testing.T) {
	r, err := NewRenderer(&schema.AtmosConfiguration{})
	require.NoError(t, err)

	tests := []struct {
		name     string
		content  string
		contains string
	}{
		{
			name:     "simple markdown",
			content:  "## Test Content",
			contains: "Test Content",
		},
		{
			name:     "markdown with code",
			content:  "`code snippet`",
			contains: "code snippet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := r.RenderAscii(tt.content)
			assert.NoError(t, err)
			assert.Contains(t, result, tt.contains)
		})
	}
}

// TestRenderWorkflow tests workflow rendering.
func TestRenderWorkflow(t *testing.T) {
	r, err := NewRenderer(&schema.AtmosConfiguration{})
	require.NoError(t, err)

	r.isTTYSupportForStdout = func() bool {
		return true
	}
	defer func() {
		r.isTTYSupportForStdout = term.IsTTYSupportForStdout
	}()

	tests := []struct {
		name     string
		content  string
		contains []string
	}{
		{
			name:     "workflow with description",
			content:  "This is a workflow description",
			contains: []string{"Workflow", "This is a workflow description"},
		},
		{
			name:     "empty workflow",
			content:  "",
			contains: []string{"Workflow"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := r.RenderWorkflow(tt.content)
			assert.NoError(t, err)
			for _, expected := range tt.contains {
				assert.Contains(t, result, expected)
			}
		})
	}
}

// TestRenderError tests error message rendering.
func TestRenderError(t *testing.T) {
	r, err := NewRenderer(&schema.AtmosConfiguration{})
	require.NoError(t, err)

	r.isTTYSupportForStderr = func() bool {
		return true
	}
	r.isTTYSupportForStdout = func() bool {
		return true
	}
	defer func() {
		r.isTTYSupportForStderr = term.IsTTYSupportForStderr
		r.isTTYSupportForStdout = term.IsTTYSupportForStdout
	}()

	tests := []struct {
		name       string
		title      string
		details    string
		suggestion string
		contains   []string
	}{
		{
			name:       "error with all fields",
			title:      "Error Title",
			details:    "Error details here",
			suggestion: "Try this fix",
			contains:   []string{"Error Title", "Error details here", "Try this fix"},
		},
		{
			name:       "error with URL suggestion",
			title:      "Error",
			details:    "Something went wrong",
			suggestion: "https://example.com/docs",
			contains:   []string{"Error", "Something went wrong", "docs", "https://example.com/docs"},
		},
		{
			name:       "error with HTTP URL suggestion",
			title:      "Error",
			details:    "Problem occurred",
			suggestion: "http://example.com/help",
			contains:   []string{"Error", "Problem occurred", "docs", "http://example.com/help"},
		},
		{
			name:       "error with only title",
			title:      "Simple Error",
			details:    "",
			suggestion: "",
			contains:   []string{"Simple Error"},
		},
		{
			name:       "error with only details",
			title:      "",
			details:    "Details without title",
			suggestion: "",
			contains:   []string{"Details without title"},
		},
		{
			name:       "error with empty title but details and suggestion",
			title:      "",
			details:    "Error occurred",
			suggestion: "Fix it this way",
			contains:   []string{"Error occurred", "Fix it this way"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := r.RenderError(tt.title, tt.details, tt.suggestion)
			assert.NoError(t, err)
			// Strip ANSI codes for assertion
			strippedResult := stripANSI(result)
			for _, expected := range tt.contains {
				assert.Contains(t, strippedResult, expected)
			}
		})
	}
}

// TestRenderSuccess tests success message rendering.
func TestRenderSuccess(t *testing.T) {
	r, err := NewRenderer(&schema.AtmosConfiguration{})
	require.NoError(t, err)

	r.isTTYSupportForStdout = func() bool {
		return true
	}
	defer func() {
		r.isTTYSupportForStdout = term.IsTTYSupportForStdout
	}()

	tests := []struct {
		name     string
		title    string
		details  string
		contains []string
	}{
		{
			name:     "success with details",
			title:    "Operation Successful",
			details:  "All changes applied",
			contains: []string{"Operation Successful", "Details", "All changes applied"},
		},
		{
			name:     "success without details",
			title:    "Success",
			details:  "",
			contains: []string{"Success"},
		},
		{
			name:     "success with multiline details",
			title:    "Deployment Complete",
			details:  "Line 1\nLine 2\nLine 3",
			contains: []string{"Deployment Complete", "Line 1", "Line 2", "Line 3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := r.RenderSuccess(tt.title, tt.details)
			assert.NoError(t, err)
			// Strip ANSI codes for assertion
			strippedResult := stripANSI(result)
			for _, expected := range tt.contains {
				assert.Contains(t, strippedResult, expected)
			}
		})
	}
}

// TestWithWidth tests the WithWidth option.
func TestWithWidth(t *testing.T) {
	r, err := NewRenderer(&schema.AtmosConfiguration{})
	require.NoError(t, err)

	// Initial width should be default
	initialWidth := r.width

	// Apply WithWidth option
	option := WithWidth(120)
	option(r)

	assert.Equal(t, uint(120), r.width)
	assert.NotEqual(t, initialWidth, r.width)
}

// TestNewTerminalMarkdownRenderer tests the NewTerminalMarkdownRenderer constructor.
func TestNewTerminalMarkdownRenderer(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig schema.AtmosConfiguration
		expectError bool
	}{
		{
			name: "with max width set",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Docs: schema.Docs{
						MaxWidth: 100,
					},
				},
			},
			expectError: false,
		},
		{
			name: "with default config",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Docs: schema.Docs{
						MaxWidth: 0,
					},
				},
			},
			expectError: false,
		},
		{
			name: "with color disabled",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						NoColor: true,
					},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			renderer, err := NewTerminalMarkdownRenderer(&tt.atmosConfig)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, renderer)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, renderer)
			}
		})
	}
}

// TestApplyStyleSafely tests the applyStyleSafely function.
func TestApplyStyleSafely(t *testing.T) {
	// This is an indirect test since applyStyleSafely is called by GetDefaultStyle
	// We verify it through GetDefaultStyle's behavior
	r, err := NewRenderer(&schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{
				Color: true,
			},
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, r)

	// If applyStyleSafely works correctly, NewRenderer should not panic
	// even with various color configurations
	r2, err := NewRenderer(&schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{
				NoColor: true,
			},
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, r2)
}

// TestGetDefaultStyle tests the GetDefaultStyle function.
func TestGetDefaultStyle(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig schema.AtmosConfiguration
	}{
		{
			name: "with color enabled",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						Color:   true,
						NoColor: false,
					},
				},
			},
		},
		{
			name: "with no color",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						NoColor: true,
					},
				},
			},
		},
		{
			name: "with default settings",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			styleBytes, err := GetDefaultStyle(&tt.atmosConfig)
			assert.NoError(t, err)
			assert.NotNil(t, styleBytes)
			assert.Greater(t, len(styleBytes), 0, "Style bytes should not be empty")
		})
	}
}
