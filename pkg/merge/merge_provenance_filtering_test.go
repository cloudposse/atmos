package merge

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// TestMergeWithOptionsAndContext_EmptyInputsFiltering verifies that filtering
// empty maps doesn't break provenance tracking or cause index misalignment.
func TestMergeWithOptionsAndContext_EmptyInputsFiltering(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		TrackProvenance: true,
		Settings: schema.AtmosSettings{
			ListMergeStrategy: "replace",
		},
	}

	// Create test inputs with some empty maps interspersed.
	input1 := map[string]any{
		"vars": map[string]any{
			"environment": "dev",
			"region":      "us-east-1",
		},
	}

	input2 := map[string]any{} // Empty map - will be filtered out

	input3 := map[string]any{
		"vars": map[string]any{
			"environment": "prod",  // Override
			"namespace":   "myapp", // New field
		},
	}

	input4 := map[string]any{} // Another empty map

	input5 := map[string]any{
		"vars": map[string]any{
			"tier": "frontend",
		},
	}

	inputs := []map[string]any{input1, input2, input3, input4, input5}

	// Create positions for the non-empty inputs.
	// Positions are keyed by JSONPath, not by input index.
	positions := u.PositionMap{
		"vars":             {Line: 1, Column: 1},
		"vars.environment": {Line: 2, Column: 3},
		"vars.region":      {Line: 3, Column: 3},
		"vars.namespace":   {Line: 10, Column: 3},
		"vars.tier":        {Line: 20, Column: 3},
	}

	// Create merge context with provenance enabled.
	ctx := NewMergeContext()
	ctx.EnableProvenance()
	ctx.CurrentFile = "test.yaml"
	ctx.Positions = positions

	// Perform merge with context.
	result, err := MergeWithOptionsAndContext(atmosConfig, inputs, false, false, ctx)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify the merge result is correct (last wins).
	varsSection, ok := result["vars"].(map[string]any)
	require.True(t, ok, "vars section should exist")
	assert.Equal(t, "prod", varsSection["environment"], "environment should be overridden")
	assert.Equal(t, "us-east-1", varsSection["region"], "region should be preserved")
	assert.Equal(t, "myapp", varsSection["namespace"], "namespace should be added")
	assert.Equal(t, "frontend", varsSection["tier"], "tier should be added")

	// Verify provenance was recorded.
	assert.True(t, ctx.IsProvenanceEnabled(), "provenance should be enabled")
	assert.NotNil(t, ctx.Provenance, "provenance storage should exist")

	// Check that provenance entries exist for the values.
	paths := ctx.Provenance.GetPaths()
	assert.Greater(t, len(paths), 0, "should have provenance entries")

	// Verify specific provenance entries have correct positions.
	envEntries := ctx.Provenance.Get("vars.environment")
	if len(envEntries) > 0 {
		latest := envEntries[len(envEntries)-1]
		assert.Equal(t, "test.yaml", latest.File, "should track file")
		assert.Equal(t, 2, latest.Line, "should have correct line number")
	}

	tierEntries := ctx.Provenance.Get("vars.tier")
	if len(tierEntries) > 0 {
		latest := tierEntries[len(tierEntries)-1]
		assert.Equal(t, "test.yaml", latest.File, "should track file")
		assert.Equal(t, 20, latest.Line, "should have correct line number")
	}
}

// TestMergeWithOptionsAndContext_AllEmptyInputs verifies handling of all empty inputs.
func TestMergeWithOptionsAndContext_AllEmptyInputs(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		TrackProvenance: true,
		Settings: schema.AtmosSettings{
			ListMergeStrategy: "replace",
		},
	}

	// All empty inputs.
	inputs := []map[string]any{
		{},
		{},
		{},
	}

	ctx := NewMergeContext()
	ctx.EnableProvenance()
	ctx.CurrentFile = "test.yaml"
	ctx.Positions = u.PositionMap{}

	// Should return empty map, not error.
	result, err := MergeWithOptionsAndContext(atmosConfig, inputs, false, false, ctx)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 0, len(result), "result should be empty")
}

// TestMergeWithOptionsAndContext_SingleNonEmptyInput verifies single non-empty input
// with provenance enabled (should skip fast-path to ensure position tracking).
func TestMergeWithOptionsAndContext_SingleNonEmptyInput(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		TrackProvenance: true,
		Settings: schema.AtmosSettings{
			ListMergeStrategy: "replace",
		},
	}

	input := map[string]any{
		"vars": map[string]any{
			"environment": "dev",
		},
	}

	inputs := []map[string]any{
		{}, // Empty
		input,
		{}, // Empty
	}

	positions := u.PositionMap{
		"vars":             {Line: 1, Column: 1},
		"vars.environment": {Line: 2, Column: 3},
	}

	ctx := NewMergeContext()
	ctx.EnableProvenance()
	ctx.CurrentFile = "test.yaml"
	ctx.Positions = positions

	result, err := MergeWithOptionsAndContext(atmosConfig, inputs, false, false, ctx)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify result.
	varsSection, ok := result["vars"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "dev", varsSection["environment"])

	// Verify provenance was recorded despite only one non-empty input.
	assert.True(t, ctx.IsProvenanceEnabled())
	paths := ctx.Provenance.GetPaths()
	assert.Greater(t, len(paths), 0, "should have provenance entries")
}

// TestMergeWithOptionsAndContext_ProvenanceDisabled verifies that filtering
// works correctly when provenance is disabled.
func TestMergeWithOptionsAndContext_ProvenanceDisabled(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		TrackProvenance: false, // Disabled
		Settings: schema.AtmosSettings{
			ListMergeStrategy: "replace",
		},
	}

	input1 := map[string]any{"key1": "value1"}
	input2 := map[string]any{} // Empty
	input3 := map[string]any{"key2": "value2"}

	inputs := []map[string]any{input1, input2, input3}

	ctx := NewMergeContext()
	ctx.CurrentFile = "test.yaml"

	result, err := MergeWithOptionsAndContext(atmosConfig, inputs, false, false, ctx)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "value1", result["key1"])
	assert.Equal(t, "value2", result["key2"])

	// Provenance should not be enabled.
	assert.False(t, ctx.IsProvenanceEnabled())
}

// TestMergeWithOptionsAndContext_PositionMapIndependence verifies that
// PositionMap (keyed by JSONPath) is independent of input array indexes,
// so filtering empty inputs doesn't cause alignment issues.
func TestMergeWithOptionsAndContext_PositionMapIndependence(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		TrackProvenance: true,
		Settings: schema.AtmosSettings{
			ListMergeStrategy: "replace",
		},
	}

	// Simulate a scenario where we have 5 inputs, but only 2 are non-empty.
	// PositionMap should reference paths, not input indexes.
	inputs := []map[string]any{
		{},                            // Index 0 - empty
		{"a": map[string]any{"x": 1}}, // Index 1
		{},                            // Index 2 - empty
		{},                            // Index 3 - empty
		{"a": map[string]any{"y": 2}}, // Index 4
	}

	// Positions reference JSONPath keys, not input indexes.
	positions := u.PositionMap{
		"a":   {Line: 10, Column: 1},
		"a.x": {Line: 11, Column: 3},
		"a.y": {Line: 20, Column: 3},
	}

	ctx := NewMergeContext()
	ctx.EnableProvenance()
	ctx.CurrentFile = "multi-input.yaml"
	ctx.Positions = positions

	result, err := MergeWithOptionsAndContext(atmosConfig, inputs, false, false, ctx)
	require.NoError(t, err)

	// After filtering, only inputs[1] and inputs[4] are merged.
	// But PositionMap lookups work by path, not by filtered index.
	aSection, ok := result["a"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 1, aSection["x"])
	assert.Equal(t, 2, aSection["y"])

	// Provenance should be recorded with correct line numbers from PositionMap.
	paths := ctx.Provenance.GetPaths()
	assert.Greater(t, len(paths), 0)

	// Verify line numbers match the PositionMap (not dependent on filtered input order).
	xEntries := ctx.Provenance.Get("a.x")
	if len(xEntries) > 0 {
		latest := xEntries[len(xEntries)-1]
		assert.Equal(t, 11, latest.Line, "line should match PositionMap, not input index")
	}

	yEntries := ctx.Provenance.Get("a.y")
	if len(yEntries) > 0 {
		latest := yEntries[len(yEntries)-1]
		assert.Equal(t, 20, latest.Line, "line should match PositionMap, not input index")
	}
}
