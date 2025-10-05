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

func TestProvenanceOutputFormat(t *testing.T) {
	// This test validates the complete provenance output structure.
	data := map[string]any{
		"stage":     "dev",       // depth 1 - parent stack
		"region":    "us-east-2", // depth 2 - first import (green)
		"namespace": "acme",      // depth 3 - second import (yellow)
		"tenant":    "plat",      // depth 4 - third import (red)
	}

	ctx := merge.NewMergeContext()
	ctx.EnableProvenance()

	// Record provenance with different depths to test symbols and colors.
	ctx.RecordProvenance("stage", merge.ProvenanceEntry{
		File:  "orgs/acme/plat/dev/us-east-2.yaml",
		Line:  10,
		Type:  merge.ProvenanceTypeInline,
		Depth: 1, // Parent stack - should show ● [1] in green
	})

	ctx.RecordProvenance("region", merge.ProvenanceEntry{
		File:  "mixins/region/us-east-2.yaml",
		Line:  8,
		Type:  merge.ProvenanceTypeImport,
		Depth: 2, // First import - should show ○ [2] in green
	})

	ctx.RecordProvenance("namespace", merge.ProvenanceEntry{
		File:  "orgs/acme/_defaults.yaml",
		Line:  2,
		Type:  merge.ProvenanceTypeImport,
		Depth: 3, // Second import - should show ○ [3] in yellow
	})

	ctx.RecordProvenance("tenant", merge.ProvenanceEntry{
		File:  "mixins/tenant/plat.yaml",
		Line:  2,
		Type:  merge.ProvenanceTypeImport,
		Depth: 4, // Third import - should show ○ [4] in red
	})

	atmosConfig := &schema.AtmosConfiguration{}
	result := RenderInlineProvenanceWithStackFile(data, ctx, atmosConfig, "orgs/acme/plat/dev/us-east-2.yaml")

	// Validate legend is present with correct text.
	assert.Contains(t, result, "# Provenance Legend:")
	assert.Contains(t, result, "#   ● [1] Defined in parent stack")
	assert.Contains(t, result, "#   ○ [N] Inherited/imported (N=2+ levels deep)")
	assert.Contains(t, result, "#   ∴ Computed/templated")

	// Validate stack file header.
	assert.Contains(t, result, "# Stack: orgs/acme/plat/dev/us-east-2.yaml")

	// Validate depth 1 uses solid disc ●.
	assert.Contains(t, result, "stage: dev")
	assert.Contains(t, result, "# ● [1] orgs/acme/plat/dev/us-east-2.yaml:10")

	// Validate depth 2+ uses hollow disc ○.
	assert.Contains(t, result, "region: us-east-2")
	assert.Contains(t, result, "# ○ [2] mixins/region/us-east-2.yaml:8")

	assert.Contains(t, result, "namespace: acme")
	assert.Contains(t, result, "# ○ [3] orgs/acme/_defaults.yaml:2")

	assert.Contains(t, result, "tenant: plat")
	assert.Contains(t, result, "# ○ [4] mixins/tenant/plat.yaml:2")
}

func TestProvenanceComputedType(t *testing.T) {
	// Test that computed values show the ∴ symbol.
	data := map[string]any{
		"computed_value": "{{ some_template }}",
	}

	ctx := merge.NewMergeContext()
	ctx.EnableProvenance()

	ctx.RecordProvenance("computed_value", merge.ProvenanceEntry{
		File:  "config.yaml",
		Line:  5,
		Type:  merge.ProvenanceTypeComputed,
		Depth: 2,
	})

	atmosConfig := &schema.AtmosConfiguration{}
	result := RenderInlineProvenanceWithStackFile(data, ctx, atmosConfig, "config.yaml")

	// Computed type should show ∴ symbol regardless of depth.
	assert.Contains(t, result, "# ∴")
	assert.Contains(t, result, "config.yaml:5")
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
