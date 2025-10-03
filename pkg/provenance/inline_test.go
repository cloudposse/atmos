package provenance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/merge"
)

func TestRenderInline_NoProvenance(t *testing.T) {
	data := map[string]any{
		"name":  "test",
		"value": 42,
	}

	// No context - should return regular YAML.
	result, err := RenderInline(data, nil)

	require.NoError(t, err)
	assert.Contains(t, result, "name: test")
	assert.Contains(t, result, "value: 42")
}

func TestRenderInline_WithProvenance(t *testing.T) {
	data := map[string]any{
		"name":  "test",
		"value": 42,
	}

	ctx := merge.NewMergeContext()
	ctx.EnableProvenance()
	ctx.CurrentFile = "config.yaml"

	// Record some provenance.
	ctx.RecordProvenance("name", merge.ProvenanceEntry{
		File:   "config.yaml",
		Line:   10,
		Column: 5,
		Type:   merge.ProvenanceTypeInline,
	})

	result, err := RenderInline(data, ctx)

	require.NoError(t, err)
	// Should contain valid YAML.
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
