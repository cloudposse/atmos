package markdown

import (
	"strings"
	"testing"

	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stripANSIForTest removes ANSI escape codes for content verification.
// This is a test helper, different from the production stripANSI.
func stripANSIForTest(s string) string {
	result := strings.Builder{}
	inEscape := false
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if s[i] == 'm' {
				inEscape = false
			}
			continue
		}
		result.WriteByte(s[i])
	}
	return result.String()
}

func TestNewCustomRenderer(t *testing.T) {
	tests := []struct {
		name    string
		opts    []CustomRendererOption
		wantErr bool
	}{
		{
			name:    "creates renderer with default options",
			opts:    nil,
			wantErr: false,
		},
		{
			name: "creates renderer with word wrap",
			opts: []CustomRendererOption{
				WithWordWrap(80),
			},
			wantErr: false,
		},
		{
			name: "creates renderer with color profile",
			opts: []CustomRendererOption{
				WithColorProfile(termenv.TrueColor),
			},
			wantErr: false,
		},
		{
			name: "creates renderer with preserved newlines",
			opts: []CustomRendererOption{
				WithPreservedNewLines(),
			},
			wantErr: false,
		},
		{
			name: "creates renderer with multiple options",
			opts: []CustomRendererOption{
				WithWordWrap(100),
				WithColorProfile(termenv.ANSI256),
				WithPreservedNewLines(),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			renderer, err := NewCustomRenderer(tt.opts...)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, renderer)
		})
	}
}

func TestCustomRenderer_Render_BasicMarkdown(t *testing.T) {
	renderer, err := NewCustomRenderer(WithColorProfile(termenv.TrueColor))
	require.NoError(t, err)

	tests := []struct {
		name        string
		input       string
		mustContain string
	}{
		{
			name:        "renders plain text",
			input:       "Hello world",
			mustContain: "Hello world",
		},
		{
			name:        "renders bold text",
			input:       "Hello **world**",
			mustContain: "world",
		},
		{
			name:        "renders italic text",
			input:       "Hello *world*",
			mustContain: "world",
		},
		{
			name:        "renders inline code",
			input:       "Use `command` here",
			mustContain: "command",
		},
		{
			name:        "renders headings",
			input:       "## Hello",
			mustContain: "Hello",
		},
		{
			name:        "renders links",
			input:       "[link](https://example.com)",
			mustContain: "link",
		},
		{
			name:        "renders lists",
			input:       "- item 1\n- item 2",
			mustContain: "item 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := renderer.Render(tt.input)
			assert.NoError(t, err)
			stripped := stripANSIForTest(result)
			assert.Contains(t, stripped, tt.mustContain)
		})
	}
}

func TestCustomRenderer_Render_Muted(t *testing.T) {
	renderer, err := NewCustomRenderer(WithColorProfile(termenv.TrueColor))
	require.NoError(t, err)

	tests := []struct {
		name        string
		input       string
		mustContain string
		hasANSI     bool
	}{
		{
			name:        "renders muted text with double parens",
			input:       "This is ((muted)) text",
			mustContain: "muted",
			hasANSI:     true,
		},
		{
			name:        "renders multiple muted sections",
			input:       "((one)) and ((two))",
			mustContain: "one",
			hasANSI:     true,
		},
		{
			name:        "ignores single parens",
			input:       "(normal) text",
			mustContain: "(normal)",
			hasANSI:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := renderer.Render(tt.input)
			assert.NoError(t, err)
			stripped := stripANSIForTest(result)
			assert.Contains(t, stripped, tt.mustContain)
			if tt.hasANSI {
				// Muted uses gray color - either TrueColor RGB (128,128,128) or 256-color (245).
				hasTrueColor := strings.Contains(result, "\x1b[38;2;128;128;128m")
				has256Color := strings.Contains(result, "\x1b[38;5;245m")
				assert.True(t, hasTrueColor || has256Color, "should contain muted color code (TrueColor or 256-color)")
			}
		})
	}
}

func TestCustomRenderer_Render_Strikethrough(t *testing.T) {
	renderer, err := NewCustomRenderer(WithColorProfile(termenv.TrueColor))
	require.NoError(t, err)

	// GFM strikethrough is styled as muted text (not crossed out).
	result, err := renderer.Render("This is ~~strikethrough~~ text")
	assert.NoError(t, err)
	stripped := stripANSIForTest(result)
	assert.Contains(t, stripped, "strikethrough")
	assert.Contains(t, stripped, "text")
}

func TestCustomRenderer_Render_Highlight(t *testing.T) {
	renderer, err := NewCustomRenderer(WithColorProfile(termenv.TrueColor))
	require.NoError(t, err)

	tests := []struct {
		name        string
		input       string
		mustContain string
		hasANSI     bool
	}{
		{
			name:        "renders highlight text",
			input:       "This is ==highlighted== text",
			mustContain: "highlighted",
			hasANSI:     true,
		},
		{
			name:        "renders multiple highlights",
			input:       "==one== and ==two==",
			mustContain: "one",
			hasANSI:     true,
		},
		{
			name:        "ignores single equals",
			input:       "a = b",
			mustContain: "a = b",
			hasANSI:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := renderer.Render(tt.input)
			assert.NoError(t, err)
			stripped := stripANSIForTest(result)
			assert.Contains(t, stripped, tt.mustContain)
			if tt.hasANSI {
				// Highlight uses yellow background (43) and black foreground (30).
				assert.Contains(t, result, "\x1b[43m", "should contain yellow background code")
			}
		})
	}
}

func TestCustomRenderer_Render_Badge(t *testing.T) {
	renderer, err := NewCustomRenderer(WithColorProfile(termenv.TrueColor))
	require.NoError(t, err)

	tests := []struct {
		name        string
		input       string
		mustContain string
		bgColor     string // Expected background color code.
	}{
		{
			name:        "renders default badge",
			input:       "[!BADGE EXPERIMENTAL]",
			mustContain: "EXPERIMENTAL",
			bgColor:     "99", // Purple
		},
		{
			name:        "renders warning badge",
			input:       "[!BADGE:warning DEPRECATED]",
			mustContain: "DEPRECATED",
			bgColor:     "208", // Orange
		},
		{
			name:        "renders success badge",
			input:       "[!BADGE:success READY]",
			mustContain: "READY",
			bgColor:     "34", // Green
		},
		{
			name:        "renders error badge",
			input:       "[!BADGE:error FAILED]",
			mustContain: "FAILED",
			bgColor:     "196", // Red
		},
		{
			name:        "renders info badge",
			input:       "[!BADGE:info NEW]",
			mustContain: "NEW",
			bgColor:     "33", // Blue
		},
		{
			name:        "renders badge with spaces in text",
			input:       "[!BADGE coming soon]",
			mustContain: "coming soon",
			bgColor:     "99",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := renderer.Render(tt.input)
			assert.NoError(t, err)
			stripped := stripANSIForTest(result)
			assert.Contains(t, stripped, tt.mustContain)
			// Check for 256-color background code.
			assert.Contains(t, result, "\x1b[48;5;"+tt.bgColor+"m",
				"should contain expected background color")
		})
	}
}

func TestCustomRenderer_Render_Admonition(t *testing.T) {
	renderer, err := NewCustomRenderer(WithColorProfile(termenv.TrueColor))
	require.NoError(t, err)

	tests := []struct {
		name         string
		input        string
		mustContain  []string
		labelColor   string
		expectedIcon string
	}{
		{
			name:         "renders NOTE admonition",
			input:        "> [!NOTE]\n> This is a note",
			mustContain:  []string{"Note", "This is a note"},
			labelColor:   "33", // Blue
			expectedIcon: "ℹ",
		},
		{
			name:         "renders WARNING admonition",
			input:        "> [!WARNING]\n> Be careful",
			mustContain:  []string{"Warning", "Be careful"},
			labelColor:   "208", // Orange
			expectedIcon: "⚠",
		},
		{
			name:         "renders TIP admonition",
			input:        "> [!TIP]\n> Try this",
			mustContain:  []string{"Tip", "Try this"},
			labelColor:   "34", // Green
			expectedIcon: "💡",
		},
		{
			name:         "renders IMPORTANT admonition",
			input:        "> [!IMPORTANT]\n> Remember this",
			mustContain:  []string{"Important", "Remember this"},
			labelColor:   "99", // Purple
			expectedIcon: "❗",
		},
		{
			name:         "renders CAUTION admonition",
			input:        "> [!CAUTION]\n> Danger ahead",
			mustContain:  []string{"Caution", "Danger ahead"},
			labelColor:   "196", // Red
			expectedIcon: "🔥",
		},
		{
			name:         "renders admonition with inline content",
			input:        "> [!NOTE] Quick note here",
			mustContain:  []string{"Note", "Quick note here"},
			labelColor:   "33",
			expectedIcon: "ℹ",
		},
		{
			name:        "renders multi-line admonition",
			input:       "> [!WARNING]\n> Line 1\n> Line 2",
			mustContain: []string{"Warning", "Line 1", "Line 2"},
			labelColor:  "208",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := renderer.Render(tt.input)
			assert.NoError(t, err)
			stripped := stripANSIForTest(result)
			for _, expected := range tt.mustContain {
				assert.Contains(t, stripped, expected, "output should contain %q", expected)
			}
			// Check for colored label.
			assert.Contains(t, result, "\x1b[38;5;"+tt.labelColor+"m",
				"should contain expected label color")
			if tt.expectedIcon != "" {
				assert.Contains(t, result, tt.expectedIcon, "should contain icon")
			}
		})
	}
}

func TestCustomRenderer_Render_CombinedSyntax(t *testing.T) {
	renderer, err := NewCustomRenderer(WithColorProfile(termenv.TrueColor))
	require.NoError(t, err)

	// Test combining multiple custom syntax elements.
	input := `# Title

This is ((muted)) and ==highlighted== text.

[!BADGE BETA] New feature

> [!NOTE]
> Remember to check the docs.
`
	result, err := renderer.Render(input)
	assert.NoError(t, err)
	stripped := stripANSIForTest(result)

	assert.Contains(t, stripped, "Title")
	assert.Contains(t, stripped, "muted")
	assert.Contains(t, stripped, "highlighted")
	assert.Contains(t, stripped, "BETA")
	assert.Contains(t, stripped, "Note")
}

func TestCustomRenderer_Close(t *testing.T) {
	renderer, err := NewCustomRenderer()
	require.NoError(t, err)

	// Close should be a no-op and return nil.
	err = renderer.Close()
	assert.NoError(t, err)
}

func TestWithStylesFromJSONBytes(t *testing.T) {
	// Test with valid JSON.
	validJSON := `{
		"document": {
			"color": "#ffffff"
		}
	}`
	renderer, err := NewCustomRenderer(
		WithStylesFromJSONBytes([]byte(validJSON)),
		WithColorProfile(termenv.TrueColor),
	)
	require.NoError(t, err)
	assert.NotNil(t, renderer)

	// Test with invalid JSON (should not error, just ignore).
	invalidJSON := `{invalid}`
	renderer, err = NewCustomRenderer(
		WithStylesFromJSONBytes([]byte(invalidJSON)),
	)
	require.NoError(t, err)
	assert.NotNil(t, renderer)
}

func TestCustomRenderer_Options(t *testing.T) {
	// Test that options are applied correctly by verifying renderer creation succeeds
	// and renders properly with each option. We can't inspect internal state directly
	// since the CustomRenderer now wraps glamour.TermRenderer.
	t.Run("WithWordWrap creates valid renderer", func(t *testing.T) {
		renderer, err := NewCustomRenderer(WithWordWrap(40))
		require.NoError(t, err)
		assert.NotNil(t, renderer)
		// Verify it renders
		output, err := renderer.Render("test")
		require.NoError(t, err)
		assert.NotEmpty(t, output)
	})

	t.Run("WithColorProfile creates valid renderer", func(t *testing.T) {
		renderer, err := NewCustomRenderer(WithColorProfile(termenv.ANSI256))
		require.NoError(t, err)
		assert.NotNil(t, renderer)
		// Verify it renders
		output, err := renderer.Render("test")
		require.NoError(t, err)
		assert.NotEmpty(t, output)
	})

	t.Run("WithPreservedNewLines creates valid renderer", func(t *testing.T) {
		renderer, err := NewCustomRenderer(WithPreservedNewLines())
		require.NoError(t, err)
		assert.NotNil(t, renderer)
		// Verify it renders
		output, err := renderer.Render("test")
		require.NoError(t, err)
		assert.NotEmpty(t, output)
	})
}

func TestCustomRenderer_EdgeCases(t *testing.T) {
	renderer, err := NewCustomRenderer(WithColorProfile(termenv.TrueColor))
	require.NoError(t, err)

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "empty string",
			input: "",
		},
		{
			name:  "only whitespace",
			input: "   \n\n   ",
		},
		{
			name:  "unclosed highlight",
			input: "==unclosed",
		},
		{
			name:  "single equals",
			input: "=single=",
		},
		{
			name:  "unclosed muted",
			input: "((unclosed",
		},
		{
			name:  "single parens",
			input: "(single)",
		},
		{
			name:  "malformed badge",
			input: "[!BADGE]", // Missing text.
		},
		{
			name:  "regular blockquote (not admonition)",
			input: "> Just a quote",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic or error.
			result, err := renderer.Render(tt.input)
			assert.NoError(t, err)
			assert.NotNil(t, result)
		})
	}
}
