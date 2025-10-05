package provenance

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
)

// formatProvenanceComment formats a provenance entry as an inline comment.
// This is a test helper function.
func formatProvenanceComment(entry merge.ProvenanceEntry) string {
	if entry.Column > 0 {
		return fmt.Sprintf("# from: %s:%d:%d", entry.File, entry.Line, entry.Column)
	}
	return fmt.Sprintf("# from: %s:%d", entry.File, entry.Line)
}

func TestRenderInlineProvenance_NoProvenance(t *testing.T) {
	data := map[string]any{
		"name":  "test",
		"value": 42,
	}

	atmosConfig := &schema.AtmosConfiguration{}

	// No context - should return regular YAML without provenance comments.
	result := RenderInlineProvenanceWithStackFile(data, nil, atmosConfig, "")

	assert.Contains(t, result, "name: test")
	assert.Contains(t, result, "value: 42")
	assert.NotContains(t, result, "# from:")
	assert.NotContains(t, result, "# Provenance Legend:")
}

func TestRenderInlineProvenance_WithProvenance(t *testing.T) {
	data := map[string]any{
		"name":  "test",
		"value": 42,
	}

	ctx := merge.NewMergeContext()
	ctx.EnableProvenance()
	ctx.CurrentFile = "config.yaml"

	// Record provenance for both fields with depth 1 (parent stack).
	ctx.RecordProvenance("name", merge.ProvenanceEntry{
		File:   "config.yaml",
		Line:   10,
		Column: 5,
		Type:   merge.ProvenanceTypeInline,
		Depth:  1,
	})
	ctx.RecordProvenance("value", merge.ProvenanceEntry{
		File:   "config.yaml",
		Line:   11,
		Column: 5,
		Type:   merge.ProvenanceTypeInline,
		Depth:  1,
	})

	atmosConfig := &schema.AtmosConfiguration{}
	result := RenderInlineProvenanceWithStackFile(data, ctx, atmosConfig, "config.yaml")

	// Should contain valid YAML with provenance legend and both fields.
	assert.Contains(t, result, "# Provenance Legend:")
	assert.Contains(t, result, "# Stack: config.yaml")
	assert.Contains(t, result, "name: test")
	assert.Contains(t, result, "value: 42")

	// Verify provenance comments are present with correct format.
	assert.Contains(t, result, "# ● [1] config.yaml:10")
	assert.Contains(t, result, "# ● [1] config.yaml:11")

	// Verify legend symbols and descriptions.
	assert.Contains(t, result, "● [1] Defined in parent stack")
	assert.Contains(t, result, "○ [N] Inherited/imported (N=2+ levels deep)")
	assert.Contains(t, result, "∴ Computed/templated")
}

func TestFormatProvenanceComment(t *testing.T) {
	tests := []struct {
		name     string
		entry    merge.ProvenanceEntry
		expected string
	}{
		{
			name: "with column",
			entry: merge.ProvenanceEntry{
				File:   "config.yaml",
				Line:   10,
				Column: 5,
			},
			expected: "# from: config.yaml:10:5",
		},
		{
			name: "without column",
			entry: merge.ProvenanceEntry{
				File:   "base.yaml",
				Line:   25,
				Column: 0,
			},
			expected: "# from: base.yaml:25",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatProvenanceComment(tt.entry)
			assert.Equal(t, tt.expected, result)
		})
	}
}
