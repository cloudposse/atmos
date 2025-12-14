package utils

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHighlightCode(t *testing.T) {
	tests := []struct {
		name        string
		code        string
		language    string
		syntaxTheme string
		wantErr     bool
		checkOutput func(t *testing.T, output string)
	}{
		{
			name:        "highlight Go code",
			code:        `package main\n\nfunc main() {\n\tfmt.Println("Hello, World!")\n}`,
			language:    "go",
			syntaxTheme: "monokai",
			wantErr:     false,
			checkOutput: func(t *testing.T, output string) {
				assert.NotEmpty(t, output)
				// The output should contain ANSI escape codes for colors.
				assert.Contains(t, output, "\x1b[")
			},
		},
		{
			name:        "highlight Python code",
			code:        `def hello():\n    print("Hello, World!")`,
			language:    "python",
			syntaxTheme: "monokai",
			wantErr:     false,
			checkOutput: func(t *testing.T, output string) {
				assert.NotEmpty(t, output)
				assert.Contains(t, output, "\x1b[")
			},
		},
		{
			name:        "highlight YAML",
			code:        `key: value\nlist:\n  - item1\n  - item2`,
			language:    "yaml",
			syntaxTheme: "monokai",
			wantErr:     false,
			checkOutput: func(t *testing.T, output string) {
				assert.NotEmpty(t, output)
			},
		},
		{
			name:        "highlight JSON",
			code:        `{"key": "value", "list": ["item1", "item2"]}`,
			language:    "json",
			syntaxTheme: "monokai",
			wantErr:     false,
			checkOutput: func(t *testing.T, output string) {
				assert.NotEmpty(t, output)
			},
		},
		{
			name:        "empty code",
			code:        "",
			language:    "go",
			syntaxTheme: "monokai",
			wantErr:     false,
			checkOutput: func(t *testing.T, output string) {
				assert.Empty(t, output)
			},
		},
		{
			name:        "invalid language",
			code:        "some code",
			language:    "not-a-real-language",
			syntaxTheme: "monokai",
			wantErr:     false, // Chroma falls back to plain text for unknown languages.
			checkOutput: func(t *testing.T, output string) {
				assert.NotEmpty(t, output)
			},
		},
		{
			name:        "different theme",
			code:        `package main`,
			language:    "go",
			syntaxTheme: "dracula",
			wantErr:     false,
			checkOutput: func(t *testing.T, output string) {
				assert.NotEmpty(t, output)
			},
		},
		{
			name:        "invalid theme falls back to default",
			code:        `package main`,
			language:    "go",
			syntaxTheme: "not-a-real-theme",
			wantErr:     false,
			checkOutput: func(t *testing.T, output string) {
				assert.NotEmpty(t, output)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := HighlightCode(tt.code, tt.language, tt.syntaxTheme)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.checkOutput != nil {
					tt.checkOutput(t, output)
				}
			}
		})
	}
}

func TestPrintStyledText(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		wantErr bool
	}{
		{
			name:    "simple text",
			text:    "Hello",
			wantErr: false,
		},
		{
			name:    "empty text",
			text:    "",
			wantErr: false,
		},
		{
			name:    "multiline text",
			text:    "Line1\nLine2\nLine3",
			wantErr: false,
		},
		{
			name:    "text with special characters",
			text:    "Hello @#$%^&*()!",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Since this function writes to stdout and checks terminal capabilities,
			// we can only test that it doesn't panic or return an error.
			err := PrintStyledText(tt.text)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				// The function returns nil in non-color terminals, so we just check no error.
				assert.NoError(t, err)
			}
		})
	}
}

func TestPrintStyledTextToSpecifiedOutput(t *testing.T) {
	tests := []struct {
		name         string
		text         string
		wantErr      bool
		checkOutput  func(t *testing.T, output string)
		expectOutput bool
	}{
		{
			name:         "simple text to buffer",
			text:         "HELLO",
			wantErr:      false,
			expectOutput: true,
			checkOutput: func(t *testing.T, output string) {
				// The figurine library will render ASCII art.
				assert.NotEmpty(t, output)
			},
		},
		{
			name:         "empty text to buffer",
			text:         "",
			wantErr:      false,
			expectOutput: true, // figurine still produces output for empty string.
			checkOutput: func(t *testing.T, output string) {
				// Empty input still produces some whitespace output.
				assert.NotNil(t, output)
			},
		},
		{
			name:         "single line text to buffer",
			text:         "TEST",
			wantErr:      false,
			expectOutput: true,
			checkOutput: func(t *testing.T, output string) {
				assert.NotEmpty(t, output)
			},
		},
		{
			name:         "single character",
			text:         "A",
			wantErr:      false,
			expectOutput: true,
			checkOutput: func(t *testing.T, output string) {
				assert.NotEmpty(t, output)
			},
		},
		{
			name:         "numbers",
			text:         "12345",
			wantErr:      false,
			expectOutput: true,
			checkOutput: func(t *testing.T, output string) {
				assert.NotEmpty(t, output)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Enable color output for tests by setting ATMOS_FORCE_COLOR.
			t.Setenv("ATMOS_FORCE_COLOR", "1")

			var buf bytes.Buffer
			err := PrintStyledTextToSpecifiedOutput(&buf, tt.text)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				output := buf.String()

				if tt.expectOutput {
					assert.NotEmpty(t, output, "Expected output but got empty string")
				}

				if tt.checkOutput != nil {
					tt.checkOutput(t, output)
				}
			}
		})
	}
}

func TestRenderMarkdown(t *testing.T) {
	tests := []struct {
		name        string
		markdown    string
		style       string
		wantErr     bool
		errContains string
		checkOutput func(t *testing.T, output string)
	}{
		{
			name:     "simple markdown",
			markdown: "# Heading\n\nThis is a paragraph.",
			style:    "",
			wantErr:  false,
			checkOutput: func(t *testing.T, output string) {
				assert.NotEmpty(t, output)
				// The rendered output should contain the heading text.
				assert.Contains(t, strings.ToLower(output), "heading")
			},
		},
		{
			name:     "markdown with code block",
			markdown: "# Code Example\n\n```go\npackage main\n```",
			style:    "",
			wantErr:  false,
			checkOutput: func(t *testing.T, output string) {
				assert.NotEmpty(t, output)
				// Just verify it doesn't error - formatting may vary.
			},
		},
		{
			name:     "markdown with list",
			markdown: "# List\n\n- Item 1\n- Item 2\n- Item 3",
			style:    "",
			wantErr:  false,
			checkOutput: func(t *testing.T, output string) {
				assert.NotEmpty(t, output)
				// Just verify it doesn't error - formatting may vary.
			},
		},
		{
			name:     "markdown with bold and italic",
			markdown: "This is **bold** and this is *italic*.",
			style:    "",
			wantErr:  false,
			checkOutput: func(t *testing.T, output string) {
				assert.NotEmpty(t, output)
				assert.Contains(t, output, "bold")
				assert.Contains(t, output, "italic")
			},
		},
		{
			name:     "markdown with table",
			markdown: "| Header 1 | Header 2 |\n|----------|----------|\n| Cell 1   | Cell 2   |",
			style:    "",
			wantErr:  false,
			checkOutput: func(t *testing.T, output string) {
				assert.NotEmpty(t, output)
				// Just verify it doesn't error - table formatting may vary.
			},
		},
		{
			name:     "markdown with link",
			markdown: "[Click here](https://example.com)",
			style:    "",
			wantErr:  false,
			checkOutput: func(t *testing.T, output string) {
				assert.NotEmpty(t, output)
				assert.Contains(t, output, "Click here")
			},
		},
		{
			name:        "empty markdown",
			markdown:    "",
			style:       "",
			wantErr:     true,
			errContains: "empty markdown input",
		},
		{
			name:     "markdown with blockquote",
			markdown: "> This is a blockquote\n> with multiple lines",
			style:    "",
			wantErr:  false,
			checkOutput: func(t *testing.T, output string) {
				assert.NotEmpty(t, output)
				// Just check that output is not empty - glamour may format blockquotes differently.
			},
		},
		{
			name:     "markdown with horizontal rule",
			markdown: "Above\n\n---\n\nBelow",
			style:    "",
			wantErr:  false,
			checkOutput: func(t *testing.T, output string) {
				assert.NotEmpty(t, output)
				assert.Contains(t, output, "Above")
				assert.Contains(t, output, "Below")
			},
		},
		{
			name: "complex markdown document",
			markdown: `# Main Title

## Introduction

This is a **complex** markdown document with various elements.

### Features

- Feature 1
- Feature 2
  - Sub-feature 2.1
  - Sub-feature 2.2

### Code Example

` + "```go" + `
func main() {
    fmt.Println("Hello, World!")
}
` + "```" + `

> **Note:** This is an important note.

| Column 1 | Column 2 |
|----------|----------|
| Data 1   | Data 2   |

---

*Thank you for reading!*`,
			style:   "",
			wantErr: false,
			checkOutput: func(t *testing.T, output string) {
				assert.NotEmpty(t, output)
				// Just verify the complex document renders without error.
				// Glamour adds lots of ANSI codes that make string matching unreliable.
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := RenderMarkdown(tt.markdown, tt.style)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				if tt.checkOutput != nil {
					tt.checkOutput(t, output)
				}
			}
		})
	}
}

func TestRenderMarkdown_EdgeCases(t *testing.T) {
	t.Run("very long line", func(t *testing.T) {
		longLine := strings.Repeat("a", 200)
		markdown := "# Heading\n\n" + longLine

		output, err := RenderMarkdown(markdown, "")
		assert.NoError(t, err)
		assert.NotEmpty(t, output)
		// The output should wrap the long line.
		assert.Contains(t, output, "aaa")
	})

	t.Run("unicode characters", func(t *testing.T) {
		markdown := "# 你好世界 🌍\n\nThis contains unicode: 😀 ñ ü é"

		output, err := RenderMarkdown(markdown, "")
		assert.NoError(t, err)
		assert.NotEmpty(t, output)
		assert.Contains(t, output, "你好")
		assert.Contains(t, output, "😀")
	})

	t.Run("nested structures", func(t *testing.T) {
		markdown := `
1. First item
   - Sub item 1
   - Sub item 2
     - Sub-sub item
2. Second item
   > Quoted text in list
   ` + "```" + `
   code in list
   ` + "```"

		output, err := RenderMarkdown(markdown, "")
		assert.NoError(t, err)
		assert.NotEmpty(t, output)
		// Just verify it renders without error - exact formatting may vary.
	})
}

// TestNewAtmosHuhTheme tests the NewAtmosHuhTheme function.
func TestNewAtmosHuhTheme(t *testing.T) {
	// Create theme once and run table-driven assertions.
	theme := NewAtmosHuhTheme()
	require.NotNil(t, theme, "NewAtmosHuhTheme should return a non-nil theme")

	tests := []struct {
		name  string
		check func() interface{}
	}{
		{"Focused styles", func() interface{} { return theme.Focused }},
		{"Blurred styles", func() interface{} { return theme.Blurred }},
		{"Focused.SelectSelector", func() interface{} { return theme.Focused.SelectSelector }},
		{"Blurred.Title", func() interface{} { return theme.Blurred.Title }},
		{"Focused.FocusedButton", func() interface{} { return theme.Focused.FocusedButton }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotNil(t, tt.check(), "theme should have %s", tt.name)
		})
	}
}
