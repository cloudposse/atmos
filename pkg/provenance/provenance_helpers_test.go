package provenance

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
)

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
}

func TestRenderInlineProvenance_WithProvenance(t *testing.T) {
	data := map[string]any{
		"name":  "test",
		"value": 42,
	}

	ctx := merge.NewMergeContext()
	ctx.EnableProvenance()
	ctx.CurrentFile = "config.yaml"

	// Record provenance for both fields.
	ctx.RecordProvenance("name", merge.ProvenanceEntry{
		File:   "config.yaml",
		Line:   10,
		Column: 5,
		Type:   merge.ProvenanceTypeInline,
	})
	ctx.RecordProvenance("value", merge.ProvenanceEntry{
		File:   "config.yaml",
		Line:   11,
		Column: 5,
		Type:   merge.ProvenanceTypeInline,
	})

	atmosConfig := &schema.AtmosConfiguration{}
	result := RenderInlineProvenanceWithStackFile(data, ctx, atmosConfig, "config.yaml")

	// Should contain valid YAML with provenance legend and both fields.
	assert.Contains(t, result, "# Provenance Legend:")
	assert.Contains(t, result, "name: test")
	assert.Contains(t, result, "value: 42")
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
			result := FormatProvenanceComment(tt.entry)
			assert.Equal(t, tt.expected, result)
		})
	}
}
