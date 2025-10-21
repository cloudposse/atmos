package markdown

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRenderer(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		mustContain  string
		atmosConfig  schema.AtmosConfiguration
		expectNoANSI bool
	}{
		{
			name:         "Test with no color",
			input:        "## Hello **world**",
			mustContain:  "## Hello **world**",
			expectNoANSI: true,
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						NoColor: true,
					},
				},
			},
		},
		{
			name:         "Test with color",
			input:        "## Hello **world**",
			mustContain:  "## Hello **world**",
			expectNoANSI: false,
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
			assert.NoError(t, err)
			// Strip ANSI codes to check content
			strippedStr := stripANSI(str)
			assert.Contains(t, strippedStr, tt.mustContain)
			if tt.expectNoANSI {
				assert.NotContains(t, str, "\x1b[", "NoColor mode should not contain ANSI codes")
			} else {
				assert.Contains(t, str, "\x1b[", "Color mode should contain ANSI codes")
			}

			str, err = r.RenderWithoutWordWrap(tt.input)
			assert.NoError(t, err)
			// Strip ANSI codes to check content
			strippedStr = stripANSI(str)
			assert.Contains(t, strippedStr, tt.mustContain)
			if tt.expectNoANSI {
				assert.NotContains(t, str, "\x1b[", "NoColor mode should not contain ANSI codes")
			} else {
				assert.Contains(t, str, "\x1b[", "Color mode should contain ANSI codes")
			}
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

func TestTrimTrailingSpaces(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "removes trailing spaces from single line",
			input:    "Hello world    \n",
			expected: "Hello world\n",
		},
		{
			name:     "removes trailing tabs",
			input:    "Hello world\t\t\n",
			expected: "Hello world\n",
		},
		{
			name:     "preserves blank lines",
			input:    "Line 1  \n\nLine 3  \n",
			expected: "Line 1\n\nLine 3\n",
		},
		{
			name:     "preserves multiple consecutive blank lines",
			input:    "Line 1  \n\n\n\nLine 5  \n",
			expected: "Line 1\n\n\n\nLine 5\n",
		},
		{
			name:     "handles blank lines with only spaces",
			input:    "Line 1  \n    \nLine 3  \n",
			expected: "Line 1\n\nLine 3\n",
		},
		{
			name:     "no trailing spaces unchanged",
			input:    "Hello\nWorld\n",
			expected: "Hello\nWorld\n",
		},
		{
			name:     "empty string unchanged",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trimTrailingSpaces(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRenderer_NonTTY_ASCII(t *testing.T) {
	r, _ := NewRenderer(schema.AtmosConfiguration{})
	r.isTTYSupportForStdout = func() bool { return false }
	defer func() {
		r.isTTYSupportForStdout = term.IsTTYSupportForStdout
	}()

	result, err := r.Render("## Hello **world**")
	assert.NoError(t, err)
	assert.NotContains(t, result, "\x1b[", "Non-TTY should not contain ANSI codes")
	assert.Contains(t, result, "Hello")

	// Verify no trailing whitespace.
	lines := strings.Split(result, "\n")
	for i, line := range lines {
		assert.Equal(t, strings.TrimRight(line, " \t"), line,
			"Line %d should not have trailing whitespace", i)
	}
}

func TestRenderer_AllMethodsTrimTrailingSpaces(t *testing.T) {
	r, _ := NewRenderer(schema.AtmosConfiguration{})
	r.isTTYSupportForStdout = func() bool { return true }
	defer func() {
		r.isTTYSupportForStdout = term.IsTTYSupportForStdout
	}()

	testContent := "## Test\n\nContent"

	tests := []struct {
		name       string
		renderFunc func(string) (string, error)
	}{
		{"Render", r.Render},
		{"RenderAscii", r.RenderAscii},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.renderFunc(testContent)
			assert.NoError(t, err)

			// Check no trailing spaces on any line.
			lines := strings.Split(result, "\n")
			for i, line := range lines {
				assert.Equal(t, strings.TrimRight(line, " \t"), line,
					"%s: Line %d should not have trailing whitespace", tt.name, i)
			}
		})
	}
}
