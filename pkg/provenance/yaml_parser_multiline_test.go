package provenance

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsMultilineScalarIndicator tests the detection of multiline YAML scalars.
func TestIsMultilineScalarIndicator(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected bool
	}{
		// Basic literal style
		{"literal basic", "|", true},
		{"literal strip", "|-", true},
		{"literal keep", "|+", true},

		// Basic folded style
		{"folded basic", ">", true},
		{"folded strip", ">-", true},
		{"folded keep", ">+", true},

		// With indent indicators
		{"literal indent 2", "|2", true},
		{"literal indent 4", "|4", true},
		{"folded indent 2", ">2", true},
		{"folded indent 4", ">4", true},

		// With chomping and indent
		{"literal strip indent 2", "|-2", true},
		{"literal keep indent 2", "|+2", true},
		{"folded strip indent 2", ">-2", true},
		{"folded keep indent 2", ">+2", true},

		// With spaces
		{"literal with space", "| ", true},
		{"literal strip with space", "|- ", true},
		{"literal indent with space", "|2 ", true},
		{"folded keep indent with space", ">+2 ", true},

		// Edge cases
		{"empty string", "", false},
		{"single quote", "'", false},
		{"double quote", "\"", false},
		{"plain scalar", "value", false},
		{"number", "123", false},

		// Invalid multiline indicators
		{"literal with text", "|text", false},
		{"folded with text", ">text", false},
		{"literal invalid char", "|a", false},
		{"folded invalid char", ">b", false},
		{"literal multiple chomping", "|+-", false},
		{"literal chomping letter", "|a2", false},

		// Multiple digits (valid indent indicators)
		{"literal indent 10", "|10", true},
		{"folded keep indent 12", ">+12", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isMultilineScalarIndicator(tt.value)
			assert.Equal(t, tt.expected, result,
				"isMultilineScalarIndicator(%q) = %v, want %v", tt.value, result, tt.expected)
		})
	}
}

// TestMultilineScalarInContext tests multiline scalars within YAML parsing context.
func TestMultilineScalarInContext(t *testing.T) {
	lines := []string{
		"description: |",
		"  This is a multiline",
		"  description that spans",
		"  multiple lines.",
		"command: |-",
		"  echo \"Hello\"",
		"  echo \"World\"",
		"script: |+",
		"  #!/bin/bash",
		"  set -e",
		"indented: |2",
		"    Two-space indent",
		"    preserved here",
	}

	pathMap := buildYAMLPathMap(lines)

	// Debug: print all paths on failure
	t.Cleanup(func() {
		if t.Failed() {
			t.Log("\n=== MULTILINE CONTEXT PATH MAP ===")
			for i := 0; i < len(lines); i++ {
				if info, ok := pathMap[i]; ok {
					t.Logf("Line %d: %q -> Path: %q, IsKey: %v, IsCont: %v",
						i, lines[i], info.Path, info.IsKeyLine, info.IsContinuation)
				} else {
					t.Logf("Line %d: %q -> NOT IN MAP", i, lines[i])
				}
			}
		}
	})

	// Verify multiline keys are detected
	assert.True(t, pathMap[0].IsKeyLine, "description key should be detected")
	assert.Equal(t, "description", pathMap[0].Path)

	assert.True(t, pathMap[4].IsKeyLine, "command key should be detected")
	assert.Equal(t, "command", pathMap[4].Path)

	assert.True(t, pathMap[7].IsKeyLine, "script key should be detected")
	assert.Equal(t, "script", pathMap[7].Path)

	assert.True(t, pathMap[10].IsKeyLine, "indented key should be detected")
	assert.Equal(t, "indented", pathMap[10].Path)

	// Verify continuation lines
	assert.True(t, pathMap[1].IsContinuation, "Line 1 should be continuation")
	assert.Equal(t, "description", pathMap[1].Path)

	assert.True(t, pathMap[5].IsContinuation, "Line 5 should be continuation")
	assert.Equal(t, "command", pathMap[5].Path)

	// Note: Line 8 (#!/bin/bash) is filtered as comment - this is a known limitation
	// where lines starting with # inside multiline scalars are treated as comments
	assert.True(t, pathMap[9].IsContinuation, "Line 9 should be continuation")
	assert.Equal(t, "script", pathMap[9].Path)

	assert.True(t, pathMap[11].IsContinuation, "Line 11 should be continuation")
	assert.Equal(t, "indented", pathMap[11].Path)
}
