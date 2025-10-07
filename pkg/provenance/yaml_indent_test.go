package provenance

import (
	"testing"

	"github.com/stretchr/testify/assert"

	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// TestYAMLIndentRespected tests that YAML indentation from LongString folded scalars
// matches the configured indent setting.
func TestYAMLIndentRespected(t *testing.T) {
	// Create test data with nested structure and long string
	longDesc := `Vpc-Flow-Logs-Bucket component "vpc-flow-logs" provisioned in the stack "plat-ue2-staging"`

	data := map[string]any{
		"vars": map[string]any{
			"tags": map[string]any{
				"atmos_component":             "vpc-flow-logs-bucket",
				"atmos_component_description": u.LongString(longDesc),
			},
		},
	}

	ctx := m.NewMergeContext()
	ctx.EnableProvenance()

	// Record provenance for the structure (needed for filterEmptySections)
	ctx.RecordProvenance("vars", m.ProvenanceEntry{
		File:  "catalog/vpc-flow-logs-bucket/defaults.yaml",
		Line:  8,
		Type:  m.ProvenanceTypeImport,
		Depth: 3,
	})

	ctx.RecordProvenance("vars.tags", m.ProvenanceEntry{
		File:  "orgs/acme/_defaults.yaml",
		Line:  20,
		Type:  m.ProvenanceTypeImport,
		Depth: 4,
	})

	ctx.RecordProvenance("vars.tags.atmos_component", m.ProvenanceEntry{
		File:  "orgs/acme/_defaults.yaml",
		Line:  20,
		Type:  m.ProvenanceTypeImport,
		Depth: 4,
	})

	// Record provenance for the long string
	ctx.RecordProvenance("vars.tags.atmos_component_description", m.ProvenanceEntry{
		File:  "orgs/acme/_defaults.yaml",
		Line:  29,
		Type:  m.ProvenanceTypeImport,
		Depth: 4,
	})

	// Test with default indent (2)
	atmosConfig := &schema.AtmosConfiguration{}
	result := RenderInlineProvenanceWithStackFile(data, ctx, atmosConfig, "test.yaml")

	// The structure should be:
	// vars:            (indent 0)
	//   tags:          (indent 2)
	//     atmos_component_description: >-   (indent 4)
	//       <content>  (indent 4, NOT 8!)
	//
	// With indent=2, at depth 2 (vars.tags), we should have 4 spaces total
	// The folded scalar content should ALSO be at 4 spaces (same indent as the key)

	// Check the YAML structure - the folded scalar content should have 4 spaces (not 8)
	assert.Contains(t, result, "vars:")
	assert.Contains(t, result, "  tags:")
	assert.Contains(t, result, "    atmos_component_description: >-")

	// The key insight: when YAML encoder sees indent=2, it adds 2 spaces per level
	// At level 2 (vars.tags.key), we have 4 spaces
	// The folded scalar content should match the key's indent (4 spaces), NOT double it (8 spaces)
	//
	// However, gopkg.in/yaml.v3 actually indents folded scalar content by an ADDITIONAL
	// indent level from the key. So with indent=2:
	// - Key at level 2: 4 spaces
	// - Content: 4 + 2 = 6 spaces
	//
	// This is standard YAML behavior, but let's verify what we actually get.
	lines := splitLines(result)
	for i, line := range lines {
		if !contains(line, "Vpc-Flow-Logs-Bucket") {
			continue
		}

		// Count leading spaces.
		spaces := 0
		for _, ch := range line {
			if ch == ' ' {
				spaces++
			} else {
				break
			}
		}
		t.Logf("Line %d has %d leading spaces: %q", i, spaces, line)

		// With SetIndent(2), the folded scalar content should be at indent level 3
		// (vars=0, tags=1, key=2, content=3) = 6 spaces.
		assert.Equal(t, 6, spaces, "Folded scalar content should have 6 spaces with indent=2")
		break
	}
}

// Helper to split lines (handles both \n and \r\n).
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// TestYAMLIndentWithIndent4 tests that when tab_width=4, folded scalars use correct indentation.
func TestYAMLIndentWithIndent4(t *testing.T) {
	longDesc := `Vpc-Flow-Logs-Bucket component "vpc-flow-logs" provisioned in the stack "plat-ue2-staging"`

	data := map[string]any{
		"vars": map[string]any{
			"tags": map[string]any{
				"atmos_component":             "vpc-flow-logs-bucket",
				"atmos_component_description": u.LongString(longDesc),
			},
		},
	}

	ctx := m.NewMergeContext()
	ctx.EnableProvenance()

	// Record provenance
	ctx.RecordProvenance("vars", m.ProvenanceEntry{File: "test.yaml", Line: 1, Type: m.ProvenanceTypeImport, Depth: 3})
	ctx.RecordProvenance("vars.tags", m.ProvenanceEntry{File: "test.yaml", Line: 2, Type: m.ProvenanceTypeImport, Depth: 4})
	ctx.RecordProvenance("vars.tags.atmos_component", m.ProvenanceEntry{File: "test.yaml", Line: 3, Type: m.ProvenanceTypeImport, Depth: 4})
	ctx.RecordProvenance("vars.tags.atmos_component_description", m.ProvenanceEntry{File: "test.yaml", Line: 4, Type: m.ProvenanceTypeImport, Depth: 4})

	// Test with tab_width=4
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{
				TabWidth: 4,
			},
		},
	}
	result := RenderInlineProvenanceWithStackFile(data, ctx, atmosConfig, "test.yaml")

	// With indent=4:
	// vars:                                (0 spaces)
	//     tags:                            (4 spaces)
	//         atmos_component_description: >-   (8 spaces)
	//             <content>                (12 spaces - one more indent level)

	lines := splitLines(result)
	for i, line := range lines {
		if !contains(line, "Vpc-Flow-Logs-Bucket") {
			continue
		}

		spaces := 0
		for _, ch := range line {
			if ch == ' ' {
				spaces++
			} else {
				break
			}
		}
		t.Logf("Line %d has %d leading spaces with indent=4: %q", i, spaces, line)

		// With SetIndent(4), content should be at 12 spaces (3 levels * 4).
		assert.Equal(t, 12, spaces, "Folded scalar content should have 12 spaces with indent=4")
		break
	}
}

// Helper to check if string contains substring.
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
