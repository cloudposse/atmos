package markdown

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
)

type testTerminal struct {
	isTTY        map[terminal.Stream]bool
	colorProfile terminal.ColorProfile
	width        map[terminal.Stream]int
}

func (t testTerminal) Write(string) error { return nil }
func (t testTerminal) IsTTY(stream terminal.Stream) bool {
	return t.isTTY != nil && t.isTTY[stream]
}
func (t testTerminal) IsPiped(terminal.Stream) bool { return false }
func (t testTerminal) ColorProfile() terminal.ColorProfile {
	return t.colorProfile
}

func (t testTerminal) Width(stream terminal.Stream) int {
	if t.width == nil {
		return 0
	}
	return t.width[stream]
}
func (t testTerminal) Height(terminal.Stream) int { return 0 }
func (t testTerminal) SetTitle(string)            {}
func (t testTerminal) RestoreTitle()              {}
func (t testTerminal) Alert()                     {}

func resetRendererTerminalTestState(t *testing.T) {
	t.Helper()
	viper.Reset()
	for _, envVar := range []string{"NO_COLOR", "CLICOLOR", "CLICOLOR_FORCE", "FORCE_COLOR", "TERM", "COLORTERM", "CI"} {
		t.Setenv(envVar, "")
	}
	t.Cleanup(viper.Reset)
}

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
			r.shouldRender = func(terminal.Stream) bool {
				return true
			}
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
	t.Setenv("NO_COLOR", "1")
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
			expected: "  \x1b[;1m## \x1b[0m\x1b[;1mHello \x1b[0m\x1b[;1m**\x1b[0m\x1b[;1mworld\x1b[0m\x1b[;1m**\x1b[0m",
			isColor:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := NewRenderer(schema.AtmosConfiguration{})
			r.shouldRender = func(terminal.Stream) bool {
				return tt.isColor
			}
			str, err := r.RenderErrorf(tt.input)
			assert.Contains(t, str, tt.expected)
			assert.NoError(t, err)
		})
	}
}

func TestRendererUsesSharedTerminalSettings(t *testing.T) {
	t.Run("shared terminal color profile enables styled non-tty rendering", func(t *testing.T) {
		r, err := NewRenderer(schema.AtmosConfiguration{}, WithTerminal(testTerminal{
			isTTY:        map[terminal.Stream]bool{terminal.Stderr: false},
			colorProfile: terminal.Color16,
		}))
		require.NoError(t, err)

		assert.True(t, r.shouldRenderStyled(terminal.Stderr))
	})

	t.Run("shared terminal no color disables styled rendering", func(t *testing.T) {
		r, err := NewRenderer(schema.AtmosConfiguration{}, WithTerminal(testTerminal{
			isTTY:        map[terminal.Stream]bool{terminal.Stderr: true},
			colorProfile: terminal.ColorNone,
		}))
		require.NoError(t, err)

		assert.False(t, r.shouldRenderStyled(terminal.Stderr))
	})
}

func TestNewTerminalMarkdownRendererHonorsForceTTYWidth(t *testing.T) {
	resetRendererTerminalTestState(t)
	t.Setenv("TERM", "xterm-256color")
	viper.Set("force-tty", true)

	r, err := NewTerminalMarkdownRenderer(schema.AtmosConfiguration{})
	require.NoError(t, err)

	assert.True(t, r.shouldRenderStyled(terminal.Stdout))
	assert.Equal(t, uint(120), r.width)
}

func TestRendererHonorsForceColorAndNoColor(t *testing.T) {
	t.Run("force color enables styled non-tty rendering", func(t *testing.T) {
		resetRendererTerminalTestState(t)
		viper.Set("force-color", true)

		r, err := NewRenderer(schema.AtmosConfiguration{})
		require.NoError(t, err)

		assert.True(t, r.shouldRenderStyled(terminal.Stderr))
	})

	t.Run("clicolor force enables styled non-tty rendering", func(t *testing.T) {
		resetRendererTerminalTestState(t)
		t.Setenv("CLICOLOR_FORCE", "1")

		r, err := NewRenderer(schema.AtmosConfiguration{})
		require.NoError(t, err)

		assert.True(t, r.shouldRenderStyled(terminal.Stderr))
	})

	t.Run("no color disables styled rendering", func(t *testing.T) {
		resetRendererTerminalTestState(t)
		t.Setenv("NO_COLOR", "1")
		viper.Set("force-color", true)

		r, err := NewRenderer(schema.AtmosConfiguration{})
		require.NoError(t, err)

		assert.False(t, r.shouldRenderStyled(terminal.Stderr))
	})
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
	t.Setenv("NO_COLOR", "1")
	r, _ := NewRenderer(schema.AtmosConfiguration{})
	r.shouldRender = func(terminal.Stream) bool { return false }

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
	r.shouldRender = func(terminal.Stream) bool { return true }

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

func TestNewHelpRenderer(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		expectError bool
	}{
		{
			name: "creates help renderer successfully",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						NoColor: false,
					},
				},
			},
			expectError: false,
		},
		{
			name: "creates help renderer with no color",
			atmosConfig: &schema.AtmosConfiguration{
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
			renderer, err := NewHelpRenderer(tt.atmosConfig)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, renderer)
			}
		})
	}
}
