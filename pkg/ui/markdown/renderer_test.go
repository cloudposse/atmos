package markdown

import (
	"testing"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
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
