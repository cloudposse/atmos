package provenance

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	termUtils "github.com/cloudposse/atmos/internal/tui/templates/term"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestBuildYAMLPathMap_MultilineSupport(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		expected map[int]YAMLLineInfo
	}{
		{
			name: "Single-line values",
			yaml: `vars:
  enabled: true
  name: test`,
			expected: map[int]YAMLLineInfo{
				0: {Path: "vars", IsKeyLine: true, IsContinuation: false},
				1: {Path: "vars.enabled", IsKeyLine: true, IsContinuation: false},
				2: {Path: "vars.name", IsKeyLine: true, IsContinuation: false},
			},
		},
		{
			name: "Multi-line literal scalar",
			yaml: `vars:
  description: |
    This is a multi-line
    description that spans
    several lines
  name: test`,
			expected: map[int]YAMLLineInfo{
				0: {Path: "vars", IsKeyLine: true, IsContinuation: false},
				1: {Path: "vars.description", IsKeyLine: true, IsContinuation: false},
				2: {Path: "vars.description", IsKeyLine: false, IsContinuation: true},
				3: {Path: "vars.description", IsKeyLine: false, IsContinuation: true},
				4: {Path: "vars.description", IsKeyLine: false, IsContinuation: true},
				5: {Path: "vars.name", IsKeyLine: true, IsContinuation: false},
			},
		},
		{
			name: "Multi-line folded scalar",
			yaml: `vars:
  description: >
    This is a folded
    multi-line value
  enabled: true`,
			expected: map[int]YAMLLineInfo{
				0: {Path: "vars", IsKeyLine: true, IsContinuation: false},
				1: {Path: "vars.description", IsKeyLine: true, IsContinuation: false},
				2: {Path: "vars.description", IsKeyLine: false, IsContinuation: true},
				3: {Path: "vars.description", IsKeyLine: false, IsContinuation: true},
				4: {Path: "vars.enabled", IsKeyLine: true, IsContinuation: false},
			},
		},
		{
			name: "Literal with strip chomping",
			yaml: `vars:
  template: |-
    {{ atmos.Component("vpc", "prod") }}
    {{ terraform.output("vpc", "prod", "id") }}
  name: test`,
			expected: map[int]YAMLLineInfo{
				0: {Path: "vars", IsKeyLine: true, IsContinuation: false},
				1: {Path: "vars.template", IsKeyLine: true, IsContinuation: false},
				2: {Path: "vars.template", IsKeyLine: false, IsContinuation: true},
				3: {Path: "vars.template", IsKeyLine: false, IsContinuation: true},
				4: {Path: "vars.name", IsKeyLine: true, IsContinuation: false},
			},
		},
		{
			name: "Empty objects and arrays",
			yaml: `backend: {}
env: {}
vars:
  tags: {}`,
			expected: map[int]YAMLLineInfo{
				0: {Path: "backend", IsKeyLine: true, IsContinuation: false},
				1: {Path: "env", IsKeyLine: true, IsContinuation: false},
				2: {Path: "vars", IsKeyLine: true, IsContinuation: false},
				3: {Path: "vars.tags", IsKeyLine: true, IsContinuation: false},
			},
		},
		{
			name: "Nested multi-line values",
			yaml: `settings:
  validation:
    policy: |
      package atmos
      deny[msg] {
        msg := "test"
      }
    enabled: true`,
			expected: map[int]YAMLLineInfo{
				0: {Path: "settings", IsKeyLine: true, IsContinuation: false},
				1: {Path: "settings.validation", IsKeyLine: true, IsContinuation: false},
				2: {Path: "settings.validation.policy", IsKeyLine: true, IsContinuation: false},
				3: {Path: "settings.validation.policy", IsKeyLine: false, IsContinuation: true},
				4: {Path: "settings.validation.policy", IsKeyLine: false, IsContinuation: true},
				5: {Path: "settings.validation.policy", IsKeyLine: false, IsContinuation: true},
				6: {Path: "settings.validation.policy", IsKeyLine: false, IsContinuation: true},
				7: {Path: "settings.validation.enabled", IsKeyLine: true, IsContinuation: false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := strings.Split(tt.yaml, "\n")
			result := buildYAMLPathMap(lines)

			// Check that we have the expected number of entries
			assert.Equal(t, len(tt.expected), len(result), "Number of path map entries should match")

			// Check each expected entry
			for lineNum, expected := range tt.expected {
				actual, exists := result[lineNum]
				assert.True(t, exists, "Line %d should have path info", lineNum)
				if exists {
					assert.Equal(t, expected.Path, actual.Path, "Line %d: path mismatch", lineNum)
					assert.Equal(t, expected.IsKeyLine, actual.IsKeyLine, "Line %d: IsKeyLine mismatch", lineNum)
					assert.Equal(t, expected.IsContinuation, actual.IsContinuation, "Line %d: IsContinuation mismatch", lineNum)
				}
			}
		})
	}
}

func TestNormalizeProvenancePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Strip components.terraform prefix",
			input:    "components.terraform.vpc-flow-logs-bucket.vars.enabled",
			expected: "vars.enabled",
		},
		{
			name:     "Strip terraform prefix",
			input:    "terraform.vars.tags.atmos_component",
			expected: "vars.tags.atmos_component",
		},
		{
			name:     "Already normalized",
			input:    "vars.enabled",
			expected: "vars.enabled",
		},
		{
			name:     "Complex nested path",
			input:    "components.terraform.vpc.settings.validation.policy",
			expected: "settings.validation.policy",
		},
		{
			name:     "Just component prefix",
			input:    "components.terraform.vpc",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeProvenancePath(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetCommentColumn(t *testing.T) {
	// Test default column when not a TTY.
	// Since tests don't run with a TTY, this should return default.
	column := getCommentColumn()
	assert.GreaterOrEqual(t, column, 40, "Comment column should be at least 40")
	assert.LessOrEqual(t, column, 200, "Comment column should not be unreasonably large")
}

func TestIsProvenanceColorEnabled(t *testing.T) {
	tests := []struct {
		name     string
		config   *schema.AtmosConfiguration
		expected bool
	}{
		{
			name:     "nil config returns false",
			config:   nil,
			expected: false,
		},
		{
			name: "NoColor wins over everything",
			config: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						NoColor:    true,
						ForceColor: true,
					},
				},
			},
			expected: false,
		},
		{
			name: "ForceColor enables color",
			config: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						ForceColor: true,
					},
				},
			},
			expected: true,
		},
		{
			name: "default config follows stdout TTY detection",
			config: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{},
				},
			},
			expected: termUtils.IsTTYSupportForStdout(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isProvenanceColorEnabled(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestColorize(t *testing.T) {
	text := "hello"
	color := lipgloss.Color("#FF0000")

	t.Run("useColor false returns plain text", func(t *testing.T) {
		result := colorize(text, color, false)
		assert.Equal(t, text, result)
	})

	t.Run("useColor true returns styled text", func(t *testing.T) {
		// Force lipgloss to render colors regardless of TTY.
		lipgloss.SetColorProfile(2) //nolint:mnd // TrueColor profile.
		t.Cleanup(func() { lipgloss.SetColorProfile(0) })

		result := colorize(text, color, true)
		// Styled text should contain original text and differ from plain output.
		assert.Contains(t, result, text)
		assert.NotEqual(t, text, result)
	})
}

func TestFormatProvenanceCommentWithStackFile_NoColor(t *testing.T) {
	tests := []struct {
		name     string
		entry    *m.ProvenanceEntry
		expected string
	}{
		{
			name:     "nil entry returns empty",
			entry:    nil,
			expected: "",
		},
		{
			name: "defined entry depth 1",
			entry: &m.ProvenanceEntry{
				File:  "stacks/orgs/acme/dev.yaml",
				Line:  10,
				Depth: 1,
				Type:  m.ProvenanceTypeInline,
			},
			expected: "# ● [1] orgs/acme/dev.yaml:10",
		},
		{
			name: "inherited entry depth 3",
			entry: &m.ProvenanceEntry{
				File:  "stacks/catalog/defaults.yaml",
				Line:  5,
				Depth: 3,
				Type:  m.ProvenanceTypeImport,
			},
			expected: "# ○ [3] catalog/defaults.yaml:5",
		},
		{
			name: "computed entry",
			entry: &m.ProvenanceEntry{
				File:  "stacks/orgs/acme/dev.yaml",
				Line:  20,
				Depth: 1,
				Type:  m.ProvenanceTypeComputed,
			},
			expected: "# ∴ [1] orgs/acme/dev.yaml:20",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatProvenanceCommentWithStackFile(tt.entry, false)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRenderProvenanceLegend_NoColor(t *testing.T) {
	t.Run("without stack file", func(t *testing.T) {
		var buf strings.Builder
		renderProvenanceLegend(&buf, "", false)
		result := buf.String()
		assert.Contains(t, result, "# Provenance Legend:")
		assert.Contains(t, result, "● [1] Defined in parent stack")
		assert.NotContains(t, result, "# Stack:")
	})

	t.Run("with stack file", func(t *testing.T) {
		var buf strings.Builder
		renderProvenanceLegend(&buf, "orgs/acme/dev/us-east-2.yaml", false)
		result := buf.String()
		assert.Contains(t, result, "# Provenance Legend:")
		assert.Contains(t, result, "# Stack: orgs/acme/dev/us-east-2.yaml")
	})
}

func TestRenderFileTree_NoColor(t *testing.T) {
	t.Run("empty tree", func(t *testing.T) {
		var buf strings.Builder
		renderFileTree(&buf, nil, false)
		assert.Equal(t, "No provenance data available.\n", buf.String())
	})

	t.Run("single file single item", func(t *testing.T) {
		var buf strings.Builder
		tree := []FileTreeNode{
			{
				File: "orgs/acme/dev.yaml",
				Items: []ProvenanceItem{
					{Symbol: SymbolDefined, Line: 10, Path: "vars.enabled"},
				},
			},
		}
		renderFileTree(&buf, tree, false)
		result := buf.String()
		assert.Contains(t, result, "stacks/")
		assert.Contains(t, result, "orgs/acme/dev.yaml")
		assert.Contains(t, result, SymbolDefined)
		assert.Contains(t, result, ":10")
		assert.Contains(t, result, "vars.enabled")
		// No ANSI codes when color is off.
		assert.NotContains(t, result, "\x1b[")
	})

	t.Run("multiple files", func(t *testing.T) {
		var buf strings.Builder
		tree := []FileTreeNode{
			{
				File: "catalog/defaults.yaml",
				Items: []ProvenanceItem{
					{Symbol: SymbolInherited, Line: 5, Path: "vars.name"},
				},
			},
			{
				File: "orgs/acme/dev.yaml",
				Items: []ProvenanceItem{
					{Symbol: SymbolDefined, Line: 10, Path: "vars.enabled"},
					{Symbol: SymbolComputed, Line: 0, Path: "vars.computed"},
				},
			},
		}
		renderFileTree(&buf, tree, false)
		result := buf.String()

		// First file uses ├── connector, last uses └──.
		assert.Contains(t, result, "├── catalog/defaults.yaml")
		assert.Contains(t, result, "└── orgs/acme/dev.yaml")
	})
}

func TestAddProvenanceToLine_NoColor(t *testing.T) {
	entry := &m.ProvenanceEntry{
		File:  "stacks/orgs/acme/dev.yaml",
		Line:  10,
		Depth: 1,
		Type:  m.ProvenanceTypeInline,
	}

	t.Run("short line gets padded comment", func(t *testing.T) {
		var buf strings.Builder
		addProvenanceToLine(&buf, "vars:", entry, 50, false)
		result := buf.String()
		require.Contains(t, result, "vars:")
		require.Contains(t, result, "# ● [1]")
		// Should be on single line (only one newline at end).
		lines := strings.Split(strings.TrimRight(result, "\n"), "\n")
		assert.Len(t, lines, 1)
	})

	t.Run("long line wraps comment to next line", func(t *testing.T) {
		var buf strings.Builder
		longLine := "very_long_key: " + strings.Repeat("x", 60)
		addProvenanceToLine(&buf, longLine, entry, 50, false)
		result := buf.String()
		// Comment should be on a separate line.
		lines := strings.Split(strings.TrimRight(result, "\n"), "\n")
		assert.Len(t, lines, 2)
		assert.Contains(t, lines[1], "# ● [1]")
	})

	t.Run("nil entry produces no comment", func(t *testing.T) {
		var buf strings.Builder
		addProvenanceToLine(&buf, "vars:", nil, 50, false)
		assert.Equal(t, "vars:\n", buf.String())
	})
}
