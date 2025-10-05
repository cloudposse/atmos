package merge

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestMergeWithProvenance_Disabled(t *testing.T) {
	// Test that provenance tracking is skipped when disabled.
	data := map[string]any{
		"vars": map[string]any{
			"name": "test",
		},
	}

	// Create atmos config with provenance disabled.
	atmosConfig := &schema.AtmosConfiguration{
		TrackProvenance: false,
	}

	ctx := NewMergeContext()
	ctx.CurrentFile = "test.yaml"
	// Don't enable provenance in context.

	// Merge should work but skip provenance tracking.
	result, err := MergeWithProvenance(atmosConfig, []map[string]any{data}, ctx, nil)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Verify no provenance was recorded.
	assert.False(t, ctx.HasProvenance("vars.name"))
}

func TestMergeWithProvenance_SimpleMap(t *testing.T) {
	// Test provenance tracking for a simple map.
	data := map[string]any{
		"vars": map[string]any{
			"name":    "test-component",
			"enabled": true,
		},
	}

	// Simulate position information.
	positions := u.PositionMap{
		"vars":         {Line: 1, Column: 1},
		"vars.name":    {Line: 2, Column: 3},
		"vars.enabled": {Line: 3, Column: 3},
	}

	// Create merge context with provenance enabled.
	ctx := NewMergeContext()
	ctx.CurrentFile = "test.yaml"
	ctx.EnableProvenance()

	atmosConfig := &schema.AtmosConfiguration{
		TrackProvenance: true,
	}

	// Merge with provenance.
	result, err := MergeWithProvenance(atmosConfig, []map[string]any{data}, ctx, positions)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Verify provenance was recorded.
	assert.True(t, ctx.HasProvenance("vars"))
	assert.True(t, ctx.HasProvenance("vars.name"))
	assert.True(t, ctx.HasProvenance("vars.enabled"))

	// Verify provenance details.
	entries := ctx.GetProvenance("vars.name")
	require.Len(t, entries, 1)
	assert.Equal(t, "test.yaml", entries[0].File)
	assert.Equal(t, 2, entries[0].Line)
	assert.Equal(t, 3, entries[0].Column)
	assert.Equal(t, ProvenanceTypeInline, entries[0].Type)
	assert.Equal(t, 0, entries[0].Depth)
}

func TestMergeWithProvenance_NestedMap(t *testing.T) {
	// Test provenance tracking for nested maps.
	data := map[string]any{
		"vars": map[string]any{
			"tags": map[string]any{
				"environment": "dev",
				"owner":       "platform",
			},
		},
	}

	positions := u.PositionMap{
		"vars":                  {Line: 1, Column: 1},
		"vars.tags":             {Line: 2, Column: 3},
		"vars.tags.environment": {Line: 3, Column: 5},
		"vars.tags.owner":       {Line: 4, Column: 5},
	}

	ctx := NewMergeContext()
	ctx.CurrentFile = "nested.yaml"
	ctx.EnableProvenance()

	atmosConfig := &schema.AtmosConfiguration{
		TrackProvenance: true,
	}

	_, err := MergeWithProvenance(atmosConfig, []map[string]any{data}, ctx, positions)
	require.NoError(t, err)

	// Verify nested provenance.
	assert.True(t, ctx.HasProvenance("vars.tags.environment"))
	assert.True(t, ctx.HasProvenance("vars.tags.owner"))

	entries := ctx.GetProvenance("vars.tags.environment")
	require.Len(t, entries, 1)
	assert.Equal(t, "nested.yaml", entries[0].File)
	assert.Equal(t, 3, entries[0].Line)
}

func TestMergeWithProvenance_Arrays(t *testing.T) {
	// Test provenance tracking for arrays.
	data := map[string]any{
		"import": []any{
			"catalog/vpc/defaults",
			"mixins/region/us-east-2",
		},
	}

	positions := u.PositionMap{
		"import":    {Line: 1, Column: 1},
		"import[0]": {Line: 2, Column: 3},
		"import[1]": {Line: 3, Column: 3},
	}

	ctx := NewMergeContext()
	ctx.CurrentFile = "with-imports.yaml"
	ctx.EnableProvenance()

	atmosConfig := &schema.AtmosConfiguration{
		TrackProvenance: true,
	}

	_, err := MergeWithProvenance(atmosConfig, []map[string]any{data}, ctx, positions)
	require.NoError(t, err)

	// Verify array provenance.
	assert.True(t, ctx.HasProvenance("import"))
	assert.True(t, ctx.HasProvenance("import[0]"))
	assert.True(t, ctx.HasProvenance("import[1]"))

	// Verify array element details.
	entries := ctx.GetProvenance("import[0]")
	require.Len(t, entries, 1)
	assert.Equal(t, "with-imports.yaml", entries[0].File)
	assert.Equal(t, 2, entries[0].Line)
}

func TestMergeWithProvenance_MultipleInputs(t *testing.T) {
	// Test provenance tracking when merging multiple inputs.
	base := map[string]any{
		"vars": map[string]any{
			"name": "base",
		},
	}

	override := map[string]any{
		"vars": map[string]any{
			"name":    "override",
			"enabled": true,
		},
	}

	// Positions for the override file.
	positions := u.PositionMap{
		"vars":         {Line: 1, Column: 1},
		"vars.name":    {Line: 2, Column: 3},
		"vars.enabled": {Line: 3, Column: 3},
	}

	ctx := NewMergeContext()
	ctx.CurrentFile = "override.yaml"
	ctx.EnableProvenance()

	atmosConfig := &schema.AtmosConfiguration{
		TrackProvenance: true,
	}

	_, err := MergeWithProvenance(atmosConfig, []map[string]any{base, override}, ctx, positions)
	require.NoError(t, err)

	// Provenance should be recorded.
	assert.True(t, ctx.HasProvenance("vars.name"))
}

func TestMergeWithProvenance_ImportDepth(t *testing.T) {
	// Test that import depth is tracked correctly.
	data := map[string]any{
		"vars": map[string]any{
			"name": "test",
		},
	}

	positions := u.PositionMap{
		"vars":      {Line: 1, Column: 1},
		"vars.name": {Line: 2, Column: 3},
	}

	// Create a parent context (simulating an import chain).
	parentCtx := NewMergeContext()
	parentCtx.CurrentFile = "parent.yaml"
	parentCtx.EnableProvenance()

	// Create child context with parent.
	childCtx := NewMergeContext()
	childCtx.CurrentFile = "child.yaml"
	childCtx.ParentContext = parentCtx
	childCtx.EnableProvenance()

	atmosConfig := &schema.AtmosConfiguration{
		TrackProvenance: true,
	}

	_, err := MergeWithProvenance(atmosConfig, []map[string]any{data}, childCtx, positions)
	require.NoError(t, err)

	// Verify depth is recorded.
	entries := childCtx.GetProvenance("vars.name")
	require.Len(t, entries, 1)
	assert.Equal(t, 1, entries[0].Depth, "Child context should have depth 1")
	assert.Equal(t, ProvenanceTypeImport, entries[0].Type, "Depth > 0 should use import type")
}

func TestMergeWithProvenance_MissingPositions(t *testing.T) {
	// Test that merge works even when position information is missing.
	data := map[string]any{
		"vars": map[string]any{
			"name": "test",
		},
	}

	// No position information provided.
	ctx := NewMergeContext()
	ctx.CurrentFile = "test.yaml"
	ctx.EnableProvenance()

	atmosConfig := &schema.AtmosConfiguration{
		TrackProvenance: true,
	}

	_, err := MergeWithProvenance(atmosConfig, []map[string]any{data}, ctx, nil)
	require.NoError(t, err)

	// Provenance should still be recorded, just with zero line/column.
	assert.True(t, ctx.HasProvenance("vars.name"))
	entries := ctx.GetProvenance("vars.name")
	require.Len(t, entries, 1)
	assert.Equal(t, 0, entries[0].Line)
	assert.Equal(t, 0, entries[0].Column)
}

func TestGetImportDepth(t *testing.T) {
	// Test GetImportDepth method.
	tests := []struct {
		name     string
		setup    func() *MergeContext
		expected int
	}{
		{
			name: "nil context",
			setup: func() *MergeContext {
				return nil
			},
			expected: 0,
		},
		{
			name:     "no parent",
			setup:    NewMergeContext,
			expected: 0,
		},
		{
			name: "one parent",
			setup: func() *MergeContext {
				parent := NewMergeContext()
				child := NewMergeContext()
				child.ParentContext = parent
				return child
			},
			expected: 1,
		},
		{
			name: "two parents",
			setup: func() *MergeContext {
				grandparent := NewMergeContext()
				parent := NewMergeContext()
				parent.ParentContext = grandparent
				child := NewMergeContext()
				child.ParentContext = parent
				return child
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setup()
			depth := ctx.GetImportDepth()
			assert.Equal(t, tt.expected, depth)
		})
	}
}
