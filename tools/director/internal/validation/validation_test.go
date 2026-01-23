package validation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tools/director/internal/scene"
)

func TestExtractTextFromSVG(t *testing.T) {
	tests := []struct {
		name     string
		svg      string
		expected string
	}{
		{
			name:     "empty SVG",
			svg:      `<svg></svg>`,
			expected: "",
		},
		{
			name:     "single tspan",
			svg:      `<svg><text><tspan>Hello World</tspan></text></svg>`,
			expected: "Hello World",
		},
		{
			name: "multiple tspans",
			svg: `<svg>
				<text>
					<tspan>Line 1</tspan>
					<tspan>Line 2</tspan>
				</text>
			</svg>`,
			expected: "Line 1\nLine 2",
		},
		{
			name: "tspan with attributes",
			svg: `<svg>
				<text>
					<tspan x="0" y="10" class="text">Error: something went wrong</tspan>
				</text>
			</svg>`,
			expected: "Error: something went wrong",
		},
		{
			name: "VHS-style SVG with prompt and command",
			svg: `<svg xmlns="http://www.w3.org/2000/svg">
				<text>
					<tspan fill="#5a56e0" x="0">></tspan>
					<tspan fill="#ffffff" x="20"> atmos list components</tspan>
				</text>
				<text>
					<tspan>┌────────────────┬──────────┐</tspan>
				</text>
			</svg>`,
			expected: ">\n atmos list components\n┌────────────────┬──────────┐",
		},
		{
			name:     "error message in SVG",
			svg:      `<svg><tspan>Error: no vendor configurations found</tspan></svg>`,
			expected: "Error: no vendor configurations found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractTextFromSVG(tt.svg)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateSVG(t *testing.T) {
	// Create a temporary directory for test SVGs.
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		svgContent  string
		defaults    *scene.ValidationConfig
		sceneConfig *scene.ValidationConfig
		expectPass  bool
		expectErrs  int
		expectMiss  int
	}{
		{
			name:       "clean SVG passes with defaults",
			svgContent: `<svg><tspan>> atmos list components</tspan><tspan>vpc</tspan></svg>`,
			defaults: &scene.ValidationConfig{
				MustNotMatch: []string{"Error: ", "command not found"},
			},
			expectPass: true,
		},
		{
			name:       "SVG with error fails",
			svgContent: `<svg><tspan>Error: no vendor configurations found</tspan></svg>`,
			defaults: &scene.ValidationConfig{
				MustNotMatch: []string{"Error: "},
			},
			expectPass: false,
			expectErrs: 1,
		},
		{
			name:       "SVG with command not found fails",
			svgContent: `<svg><tspan>bash: foo: command not found</tspan></svg>`,
			defaults: &scene.ValidationConfig{
				MustNotMatch: []string{"bash: .*: command not found"},
			},
			expectPass: false,
			expectErrs: 1,
		},
		{
			name:       "must_match pattern missing",
			svgContent: `<svg><tspan>some output</tspan></svg>`,
			defaults: &scene.ValidationConfig{
				MustMatch: []string{"atmos"},
			},
			expectPass: false,
			expectMiss: 1,
		},
		{
			name:       "must_match pattern present",
			svgContent: `<svg><tspan>atmos list components</tspan></svg>`,
			defaults: &scene.ValidationConfig{
				MustMatch: []string{"atmos"},
			},
			expectPass: true,
		},
		{
			name:       "scene config overrides defaults - empty must_not_match",
			svgContent: `<svg><tspan>Error: intentional error for demo</tspan></svg>`,
			defaults: &scene.ValidationConfig{
				MustNotMatch: []string{"Error: "},
			},
			sceneConfig: &scene.ValidationConfig{
				MustNotMatch: []string{},         // Empty = no checks.
				MustMatch:    []string{"Error:"}, // We WANT the error to appear.
			},
			expectPass: true,
		},
		{
			name:       "multiple violations",
			svgContent: `<svg><tspan>Error: failed</tspan><tspan>bash: atmos: command not found</tspan></svg>`,
			defaults: &scene.ValidationConfig{
				MustNotMatch: []string{"Error: ", "command not found"},
				MustMatch:    []string{"success"},
			},
			expectPass: false,
			expectErrs: 2,
			expectMiss: 1,
		},
		{
			name:       "no validation config - passes",
			svgContent: `<svg><tspan>anything</tspan></svg>`,
			defaults:   nil,
			expectPass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write test SVG to temp file.
			svgPath := filepath.Join(tmpDir, tt.name+".svg")
			err := os.WriteFile(svgPath, []byte(tt.svgContent), 0o644)
			require.NoError(t, err)

			validator := New(tt.defaults)
			result, err := validator.ValidateSVG(svgPath, tt.sceneConfig)
			require.NoError(t, err)

			assert.Equal(t, tt.expectPass, result.Passed, "expected Passed=%v, got %v", tt.expectPass, result.Passed)
			assert.Len(t, result.Errors, tt.expectErrs, "expected %d errors, got %d: %v", tt.expectErrs, len(result.Errors), result.Errors)
			assert.Len(t, result.Missing, tt.expectMiss, "expected %d missing, got %d: %v", tt.expectMiss, len(result.Missing), result.Missing)
		})
	}
}

func TestValidateSVG_InvalidPattern(t *testing.T) {
	tmpDir := t.TempDir()
	svgPath := filepath.Join(tmpDir, "test.svg")
	err := os.WriteFile(svgPath, []byte(`<svg><tspan>test</tspan></svg>`), 0o644)
	require.NoError(t, err)

	validator := New(&scene.ValidationConfig{
		MustNotMatch: []string{"[invalid regex"},
	})

	_, err = validator.ValidateSVG(svgPath, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid must_not_match pattern")
}

func TestValidateSVG_FileNotFound(t *testing.T) {
	validator := New(nil)
	_, err := validator.ValidateSVG("/nonexistent/path.svg", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read SVG")
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"exactly10c", 10, "exactly10c"},
		{"this is a longer string", 10, "this is..."},
		{"abc", 5, "abc"},
		{"", 5, ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := truncate(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}
