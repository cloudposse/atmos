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

	// Verify provenance comments are present with correct format (exact format check).
	// Comments should be on the same line or adjacent line to the field they document.
	assert.Contains(t, result, "# ● [1] config.yaml:10")
	assert.Contains(t, result, "# ● [1] config.yaml:11")

	// Verify legend symbols and descriptions with exact text.
	assert.Contains(t, result, "#   ● [1] Defined in parent stack")
	assert.Contains(t, result, "#   ○ [N] Inherited/imported (N=2+ levels deep)")
	assert.Contains(t, result, "#   ∴ Computed/templated")
}

func TestProvenanceOutputFormat(t *testing.T) {
	// This test validates the complete provenance output structure.
	data := map[string]any{
		"stage":     "dev",       // depth 1 - parent stack
		"region":    "us-east-2", // depth 2 - first import (green)
		"namespace": "acme",      // depth 3 - second import (orange)
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
		Depth: 3, // Second import - should show ○ [3] in orange
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

func TestRenderInlineProvenance_NilAtmosConfig(t *testing.T) {
	// Test that nil atmosConfig is handled gracefully.
	data := map[string]any{
		"name": "test",
	}

	ctx := merge.NewMergeContext()
	ctx.EnableProvenance()
	ctx.RecordProvenance("name", merge.ProvenanceEntry{
		File:   "config.yaml",
		Line:   10,
		Column: 5,
		Type:   merge.ProvenanceTypeInline,
		Depth:  1,
	})

	// Should not panic with nil atmosConfig.
	result := RenderInlineProvenanceWithStackFile(data, ctx, nil, "config.yaml")

	// Should still render YAML output (may or may not include provenance depending on implementation).
	assert.Contains(t, result, "name: test")
}

func TestRenderInlineProvenance_MalformedProvenanceEntries(t *testing.T) {
	// Test handling of malformed provenance entries.
	tests := []struct {
		name  string
		entry merge.ProvenanceEntry
		data  map[string]any
	}{
		{
			name: "negative line number",
			entry: merge.ProvenanceEntry{
				File:   "config.yaml",
				Line:   -1,
				Column: 5,
				Type:   merge.ProvenanceTypeInline,
				Depth:  1,
			},
			data: map[string]any{"field": "value"},
		},
		{
			name: "empty filename",
			entry: merge.ProvenanceEntry{
				File:   "",
				Line:   10,
				Column: 5,
				Type:   merge.ProvenanceTypeInline,
				Depth:  1,
			},
			data: map[string]any{"field": "value"},
		},
		{
			name: "zero line number",
			entry: merge.ProvenanceEntry{
				File:   "config.yaml",
				Line:   0,
				Column: 5,
				Type:   merge.ProvenanceTypeInline,
				Depth:  1,
			},
			data: map[string]any{"field": "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := merge.NewMergeContext()
			ctx.EnableProvenance()
			ctx.RecordProvenance("field", tt.entry)

			atmosConfig := &schema.AtmosConfiguration{}

			// Should not panic and should return valid YAML.
			result := RenderInlineProvenanceWithStackFile(tt.data, ctx, atmosConfig, "test.yaml")
			assert.Contains(t, result, "field: value")
			// Result may or may not include the malformed provenance comment - implementation decides.
		})
	}
}

func TestRenderInlineProvenance_NullValues(t *testing.T) {
	// Test that nil/null values are handled correctly with provenance.
	data := map[string]any{
		"nullable_field": nil,
		"normal_field":   "value",
	}

	ctx := merge.NewMergeContext()
	ctx.EnableProvenance()
	ctx.RecordProvenance("nullable_field", merge.ProvenanceEntry{
		File:   "config.yaml",
		Line:   10,
		Column: 5,
		Type:   merge.ProvenanceTypeInline,
		Depth:  1,
	})
	ctx.RecordProvenance("normal_field", merge.ProvenanceEntry{
		File:   "config.yaml",
		Line:   11,
		Column: 5,
		Type:   merge.ProvenanceTypeInline,
		Depth:  1,
	})

	atmosConfig := &schema.AtmosConfiguration{}
	result := RenderInlineProvenanceWithStackFile(data, ctx, atmosConfig, "config.yaml")

	// Should contain the null value in YAML format.
	assert.Contains(t, result, "nullable_field: null")
	assert.Contains(t, result, "normal_field: value")

	// Should include provenance for both fields.
	assert.Contains(t, result, "# ● [1] config.yaml:10")
	assert.Contains(t, result, "# ● [1] config.yaml:11")
}

func TestRenderInlineProvenance_NestedStructures(t *testing.T) {
	// Test provenance rendering with nested structures.
	// Note: Only fields with recorded provenance get inline comments.
	data := map[string]any{
		"top_level": "value1",
		"nested": map[string]any{
			"level2": "value2",
			"deeper": map[string]any{
				"level3": "value3",
			},
		},
	}

	ctx := merge.NewMergeContext()
	ctx.EnableProvenance()
	ctx.RecordProvenance("top_level", merge.ProvenanceEntry{
		File:  "config.yaml",
		Line:  5,
		Type:  merge.ProvenanceTypeInline,
		Depth: 1,
	})
	ctx.RecordProvenance("nested.level2", merge.ProvenanceEntry{
		File:  "config.yaml",
		Line:  10,
		Type:  merge.ProvenanceTypeInline,
		Depth: 1,
	})
	ctx.RecordProvenance("nested.deeper.level3", merge.ProvenanceEntry{
		File:  "imported.yaml",
		Line:  15,
		Type:  merge.ProvenanceTypeImport,
		Depth: 2,
	})

	atmosConfig := &schema.AtmosConfiguration{}
	result := RenderInlineProvenanceWithStackFile(data, ctx, atmosConfig, "config.yaml")

	// Should contain top-level field with provenance.
	assert.Contains(t, result, "top_level: value1")
	assert.Contains(t, result, "# ● [1] config.yaml:5")

	// The function only renders fields with provenance at the top level.
	// Nested structures are rendered as full objects without inline comments for nested fields.
	// This is expected behavior - only top-level scalar fields with provenance get inline comments.
}

func TestRenderInlineProvenance_DifferentProvenanceTypes(t *testing.T) {
	// Test different provenance types: inline, import, override, computed.
	data := map[string]any{
		"inline_value":   "val1",
		"import_value":   "val2",
		"override_value": "val3",
		"computed_value": "val4",
	}

	ctx := merge.NewMergeContext()
	ctx.EnableProvenance()

	ctx.RecordProvenance("inline_value", merge.ProvenanceEntry{
		File:  "stack.yaml",
		Line:  10,
		Type:  merge.ProvenanceTypeInline,
		Depth: 1,
	})

	ctx.RecordProvenance("import_value", merge.ProvenanceEntry{
		File:  "imported.yaml",
		Line:  20,
		Type:  merge.ProvenanceTypeImport,
		Depth: 2,
	})

	ctx.RecordProvenance("override_value", merge.ProvenanceEntry{
		File:  "override.yaml",
		Line:  30,
		Type:  merge.ProvenanceTypeOverride,
		Depth: 3,
	})

	ctx.RecordProvenance("computed_value", merge.ProvenanceEntry{
		File:  "stack.yaml",
		Line:  40,
		Type:  merge.ProvenanceTypeComputed,
		Depth: 1,
	})

	atmosConfig := &schema.AtmosConfiguration{}
	result := RenderInlineProvenanceWithStackFile(data, ctx, atmosConfig, "stack.yaml")

	// Verify inline type uses ●.
	assert.Contains(t, result, "# ● [1] stack.yaml:10")

	// Verify import type uses ○.
	assert.Contains(t, result, "# ○ [2] imported.yaml:20")

	// Verify override type uses ○.
	assert.Contains(t, result, "# ○ [3] override.yaml:30")

	// Verify computed type uses ∴ (with depth indicator).
	assert.Contains(t, result, "# ∴ [1] stack.yaml:40")
}

func TestRenderInlineProvenance_YAMLMarshallingError(t *testing.T) {
	// Test that non-serializable values are handled gracefully.
	// Note: gopkg.in/yaml.v3 has internal panic recovery that converts
	// unmarshallable types (like functions, channels) to `{}` instead of
	// propagating the panic. This test documents the behavior and ensures
	// panic recovery is in place for any future YAML library changes.
	//
	// The panic recovery in RenderInlineProvenanceWithStackFile exists as
	// defensive programming to handle edge cases, but current yaml.v3 behavior
	// makes it difficult to trigger via test data without directly invoking
	// the low-level encoder.
	data := map[string]any{
		"func": func() {},
		"chan": make(chan int),
	}

	ctx := merge.NewMergeContext()
	ctx.EnableProvenance()

	atmosConfig := &schema.AtmosConfiguration{}
	result := RenderInlineProvenanceWithStackFile(data, ctx, atmosConfig, "broken.yaml")

	// yaml.v3 marshals these as `{}` instead of panicking.
	// Since the values have no provenance, they're filtered out by filterEmptySections,
	// resulting in an empty top-level map `{}`.
	// Verify the function doesn't panic and returns valid output.
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "Provenance Legend")
	assert.Contains(t, result, "{}")
}
