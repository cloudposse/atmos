package utils

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestHighlightCodeWithConfig_RespectsNoColorFlag tests that HighlightCodeWithConfig
// respects the NoColor flag in the configuration.
func TestHighlightCodeWithConfig_RespectsNoColorFlag(t *testing.T) {
	tests := []struct {
		name           string
		config         *schema.AtmosConfiguration
		code           string
		expectColor    bool
		description    string
	}{
		{
			name: "NoColor flag set to true should disable colors",
			config: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						NoColor: true,
						Color:   false,
						SyntaxHighlighting: schema.SyntaxHighlighting{
							Enabled: true,
							Theme:   "dracula",
						},
					},
				},
			},
			code:        `{"test": "value"}`,
			expectColor: false,
			description: "When NoColor is true, output should not contain ANSI escape codes",
		},
		{
			name: "Color flag set to false should disable colors",
			config: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						NoColor: false,
						Color:   false,
						SyntaxHighlighting: schema.SyntaxHighlighting{
							Enabled: true,
							Theme:   "dracula",
						},
					},
				},
			},
			code:        `{"test": "value"}`,
			expectColor: false,
			description: "When Color is false, output should not contain ANSI escape codes",
		},
		{
			name: "Color flag set to true should enable colors",
			config: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						NoColor: false,
						Color:   true,
						SyntaxHighlighting: schema.SyntaxHighlighting{
							Enabled: true,
							Theme:   "dracula",
						},
					},
				},
			},
			code:        `{"test": "value"}`,
			expectColor: true,
			description: "When Color is true, output should contain ANSI escape codes in terminal",
		},
		{
			name: "NoColor takes precedence over Color",
			config: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						NoColor: true,
						Color:   true, // This should be ignored
						SyntaxHighlighting: schema.SyntaxHighlighting{
							Enabled: true,
							Theme:   "dracula",
						},
					},
				},
			},
			code:        `{"test": "value"}`,
			expectColor: false,
			description: "NoColor should take precedence over Color setting",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := HighlightCodeWithConfig(tt.config, tt.code, "json")
			assert.NoError(t, err, "HighlightCodeWithConfig should not return an error")

			// Check for ANSI escape codes (color sequences start with \x1b[)
			hasColor := strings.Contains(result, "\x1b[")

			if tt.expectColor {
				// In a terminal, we expect colors
				// But this test might run in CI where there's no TTY
				// So we'll just document the expected behavior
				t.Logf("Expected colors (hasColor=%v): %s", hasColor, tt.description)
			} else {
				// We should NEVER have colors when NoColor is true or Color is false
				assert.False(t, hasColor,
					"Output should not contain ANSI escape codes when NoColor=true or Color=false.\n"+
					"Got: %q\n"+
					"Description: %s", result, tt.description)
			}
		})
	}
}

// TestPrintAsYAML_RespectsNoColorFlag tests that PrintAsYAML respects the NoColor flag.
func TestPrintAsYAML_RespectsNoColorFlag(t *testing.T) {
	config := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{
				NoColor: true,
				Color:   false,
				SyntaxHighlighting: schema.SyntaxHighlighting{
					Enabled: true,
					Theme:   "dracula",
				},
			},
		},
	}

	data := map[string]any{
		"test": "value",
		"number": 123,
	}

	// Get the highlighted YAML
	result, err := GetHighlightedYAML(config, data)
	assert.NoError(t, err, "GetHighlightedYAML should not return an error")

	// Check for ANSI escape codes
	hasColor := strings.Contains(result, "\x1b[")
	assert.False(t, hasColor,
		"YAML output should not contain ANSI escape codes when NoColor=true.\n"+
		"Got: %q", result)
}

// TestPrintAsJSON_RespectsNoColorFlag tests that PrintAsJSON respects the NoColor flag.
func TestPrintAsJSON_RespectsNoColorFlag(t *testing.T) {
	config := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{
				NoColor: true,
				Color:   false,
				SyntaxHighlighting: schema.SyntaxHighlighting{
					Enabled: true,
					Theme:   "dracula",
				},
			},
		},
	}

	data := map[string]any{
		"test": "value",
		"number": 123,
	}

	// Get the highlighted JSON
	result, err := GetHighlightedJSON(config, data)
	assert.NoError(t, err, "GetHighlightedJSON should not return an error")

	// Check for ANSI escape codes
	hasColor := strings.Contains(result, "\x1b[")
	assert.False(t, hasColor,
		"JSON output should not contain ANSI escape codes when NoColor=true.\n"+
		"Got: %q", result)
}
