package markdown

import (
	"testing"

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
			str, err := r.Render(tt.input)
			assert.Contains(t, str, tt.expected)
			assert.NoError(t, err)
		})
	}
}
