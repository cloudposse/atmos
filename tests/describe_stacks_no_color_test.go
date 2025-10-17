package tests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// TestDescribeStacks_NoColorFlag tests that the describe stacks command respects the --no-color flag.
func TestDescribeStacks_NoColorFlag(t *testing.T) {
	// This test verifies the bug reported in DEV-3701 where --no-color is not working.
	// It tests the highlighting functions directly which is where the bug exists.

	tests := []struct {
		name           string
		noColor        bool
		expectNoColors bool
		description    string
	}{
		{
			name:           "With NoColor=true, output should have no ANSI colors",
			noColor:        true,
			expectNoColors: true,
			description:    "Bug reproduction: --no-color flag should disable all color output",
		},
		{
			name:           "With NoColor=false, Color=true may have colors",
			noColor:        false,
			expectNoColors: false,
			description:    "When colors enabled, may contain ANSI escape codes in TTY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test config with color settings
			atmosConfig := &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						NoColor: tt.noColor,
						Color:   !tt.noColor,
						SyntaxHighlighting: schema.SyntaxHighlighting{
							Enabled: true,
							Theme:   "dracula",
						},
					},
				},
			}

			// Test data
			testData := map[string]any{
				"test": "value",
				"stack": map[string]any{
					"component": "vpc",
					"vars": map[string]any{
						"region": "us-east-1",
					},
				},
			}

			// Get highlighted output (this is what the bug affects)
			yamlOutput, err := u.GetHighlightedYAML(atmosConfig, testData)
			require.NoError(t, err, "GetHighlightedYAML should not return an error")

			jsonOutput, err := u.GetHighlightedJSON(atmosConfig, testData)
			require.NoError(t, err, "GetHighlightedJSON should not return an error")

			// Check for ANSI escape codes
			yamlHasColor := containsANSIColors(yamlOutput)
			jsonHasColor := containsANSIColors(jsonOutput)

			if tt.expectNoColors {
				assert.False(t, yamlHasColor,
					"YAML output should NOT contain ANSI colors when NoColor=%v", tt.noColor)
				assert.False(t, jsonHasColor,
					"JSON output should NOT contain ANSI colors when NoColor=%v", tt.noColor)
			}

			t.Logf("NoColor=%v, YAML has colors=%v, JSON has colors=%v",
				tt.noColor, yamlHasColor, jsonHasColor)
		})
	}
}

// containsANSIColors checks if a string contains ANSI escape codes.
func containsANSIColors(s string) bool {
	// ANSI escape codes start with ESC[ which is \x1b[
	// Also check for other common ANSI patterns
	return strings.Contains(s, "\x1b[") ||
		strings.Contains(s, "\033[") ||
		strings.Contains(s, "\x1B[")
}
