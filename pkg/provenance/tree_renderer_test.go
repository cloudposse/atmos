package provenance

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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
	// Test default column when not a TTY
	// Since tests don't run with a TTY, this should return default
	column := getCommentColumn()
	assert.GreaterOrEqual(t, column, 40, "Comment column should be at least 40")
	assert.LessOrEqual(t, column, 200, "Comment column should not be unreasonably large")
}
