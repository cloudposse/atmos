package provenance

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	m "github.com/cloudposse/atmos/pkg/merge"
)

// TestRenderExplainTrace_NilContext tests that nil context returns helpful message.
func TestRenderExplainTrace_NilContext(t *testing.T) {
	result := RenderExplainTrace(map[string]any{}, nil, nil, "")
	assert.Contains(t, result, "No provenance data")
}

// TestRenderExplainTrace_DisabledProvenance tests that disabled provenance returns helpful message.
func TestRenderExplainTrace_DisabledProvenance(t *testing.T) {
	ctx := m.NewMergeContext()
	// Provenance NOT enabled
	result := RenderExplainTrace(map[string]any{}, ctx, nil, "")
	assert.Contains(t, result, "No provenance data")
}

// TestRenderExplainTrace_EmptyData tests output with enabled but empty provenance.
func TestRenderExplainTrace_EmptyData(t *testing.T) {
	ctx := m.NewMergeContext()
	ctx.EnableProvenance()
	result := RenderExplainTrace(map[string]any{}, ctx, nil, "")
	assert.Contains(t, result, "No provenance data")
}

// TestRenderExplainTrace_SingleEntry tests output with a single provenance entry (no overrides).
func TestRenderExplainTrace_SingleEntry(t *testing.T) {
	ctx := m.NewMergeContext()
	ctx.EnableProvenance()

	entry := m.ProvenanceEntry{
		File:  "stacks/orgs/acme/prod/us-east-2.yaml",
		Line:  24,
		Type:  m.ProvenanceTypeInline,
		Depth: 1,
		Value: "10.0.0.0/16",
	}
	ctx.RecordProvenance("vars.cidr_block", entry)

	data := map[string]any{
		"vars": map[string]any{
			"cidr_block": "10.0.0.0/16",
		},
	}

	result := RenderExplainTrace(data, ctx, nil, "")

	// Should contain the key path.
	assert.Contains(t, result, "vars.cidr_block")
	// Should show "SET by" with the file.
	assert.Contains(t, result, explainSetByLabel)
	assert.Contains(t, result, "orgs/acme/prod/us-east-2.yaml")
	assert.Contains(t, result, "line 24")
	// No OVERRIDE data entries since there's only one entry.
	// Verify no file path appears after "OVERRIDE:" (which would indicate an override data entry).
	lines := strings.Split(result, "\n")
	hasOverrideEntry := false
	for _, line := range lines {
		// Override data entries are indented (not the legend #-prefixed lines).
		stripped := strings.TrimSpace(line)
		if !strings.HasPrefix(stripped, "#") && strings.Contains(line, explainOverrideLabel) {
			hasOverrideEntry = true
			break
		}
	}
	assert.False(t, hasOverrideEntry, "Should not have any OVERRIDE data entries for single-entry provenance")
}

// TestRenderExplainTrace_WithOverride tests output showing SET by + OVERRIDE.
func TestRenderExplainTrace_WithOverride(t *testing.T) {
	ctx := m.NewMergeContext()
	ctx.EnableProvenance()

	// Base value (deeper import = higher depth = lower priority).
	baseEntry := m.ProvenanceEntry{
		File:  "catalog/vpc/defaults.yaml",
		Line:  10,
		Type:  m.ProvenanceTypeImport,
		Depth: 2,
		Value: "172.16.0.0/16",
	}
	// Winner (root stack = lower depth = higher priority).
	winnerEntry := m.ProvenanceEntry{
		File:  "stacks/orgs/acme/prod/us-east-2.yaml",
		Line:  24,
		Type:  m.ProvenanceTypeInline,
		Depth: 1,
		Value: "10.0.0.0/16",
	}
	// Record base first (as would happen during processing).
	ctx.RecordProvenance("vars.cidr_block", baseEntry)
	ctx.RecordProvenance("vars.cidr_block", winnerEntry)

	data := map[string]any{
		"vars": map[string]any{
			"cidr_block": "10.0.0.0/16",
		},
	}

	result := RenderExplainTrace(data, ctx, nil, "")

	// Should contain key path.
	assert.Contains(t, result, "vars.cidr_block")
	// Winner should be the root stack file (lower depth).
	assert.Contains(t, result, explainSetByLabel)
	assert.Contains(t, result, "orgs/acme/prod/us-east-2.yaml")
	// Override should show the base entry.
	assert.Contains(t, result, explainOverrideLabel)
	assert.Contains(t, result, "catalog/vpc/defaults.yaml")
}

// TestRenderExplainTrace_StackFileHeader tests that the stack file header is included.
func TestRenderExplainTrace_StackFileHeader(t *testing.T) {
	ctx := m.NewMergeContext()
	ctx.EnableProvenance()

	entry := m.ProvenanceEntry{
		File:  "stacks/prod.yaml",
		Line:  5,
		Type:  m.ProvenanceTypeInline,
		Depth: 1,
		Value: "myvalue",
	}
	ctx.RecordProvenance("vars.name", entry)

	result := RenderExplainTrace(map[string]any{"vars": map[string]any{"name": "myvalue"}}, ctx, nil, "stacks/prod.yaml")

	assert.Contains(t, result, "Stack: stacks/prod.yaml")
}

// TestRenderExplainTrace_InternalPathsFiltered tests that __import__ paths are filtered out.
func TestRenderExplainTrace_InternalPathsFiltered(t *testing.T) {
	ctx := m.NewMergeContext()
	ctx.EnableProvenance()

	// Add internal tracking path (should not appear in output).
	ctx.RecordProvenance("__import__:catalog/vpc/defaults.yaml", m.ProvenanceEntry{
		File:  "stacks/prod.yaml",
		Line:  1,
		Depth: 1,
	})
	// Add real path.
	ctx.RecordProvenance("vars.cidr_block", m.ProvenanceEntry{
		File:  "stacks/prod.yaml",
		Line:  5,
		Depth: 1,
		Value: "10.0.0.0/16",
	})

	result := RenderExplainTrace(map[string]any{"vars": map[string]any{"cidr_block": "10.0.0.0/16"}}, ctx, nil, "")

	assert.Contains(t, result, "vars.cidr_block")
	assert.NotContains(t, result, "__import__")
}

// TestFormatExplainValue tests various value formatting cases.
func TestFormatExplainValue(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{"nil value", nil, explainValueNone},
		{"string value", "hello", `"hello"`},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"int value", 42, "42"},
		{"float value", 3.14, "3.14"},
		{"empty map", map[string]any{}, "{}"},
		{"non-empty map", map[string]any{"a": 1, "b": 2}, "{2 keys}"},
		{"empty slice", []any{}, "[]"},
		{"non-empty slice", []any{"a", "b", "c"}, "[3 items]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatExplainValue(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestFindWinnerIndex tests that the winner is correctly identified as the entry with lowest depth.
func TestFindWinnerIndex(t *testing.T) {
	tests := []struct {
		name     string
		chain    []m.ProvenanceEntry
		expected int
	}{
		{
			name: "single entry",
			chain: []m.ProvenanceEntry{
				{Depth: 1, File: "root.yaml"},
			},
			expected: 0,
		},
		{
			name: "winner is last (lowest depth recorded last)",
			chain: []m.ProvenanceEntry{
				{Depth: 3, File: "deep.yaml"}, // base import
				{Depth: 2, File: "mid.yaml"},  // middle import
				{Depth: 1, File: "root.yaml"}, // root file = winner
			},
			expected: 2, // Last entry has lowest depth
		},
		{
			name: "winner is first (unusual case)",
			chain: []m.ProvenanceEntry{
				{Depth: 1, File: "root.yaml"}, // root file recorded first
				{Depth: 3, File: "deep.yaml"}, // deep import
			},
			expected: 0, // Index 0 has lowest depth
		},
		{
			name: "tie broken by last recorded",
			chain: []m.ProvenanceEntry{
				{Depth: 1, File: "a.yaml"},
				{Depth: 1, File: "b.yaml"}, // Same depth, last recorded
			},
			expected: 1, // Last with same depth
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findWinnerIndex(tt.chain)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestLookupValue tests value lookup from nested maps.
func TestLookupValue(t *testing.T) {
	data := map[string]any{
		"vars": map[string]any{
			"cidr_block": "10.0.0.0/16",
			"tags": map[string]any{
				"env": "prod",
			},
		},
		"imports": []any{"catalog/vpc", "mixins/region"},
	}

	tests := []struct {
		name     string
		path     string
		expected any
	}{
		{"top-level key", "vars", map[string]any{"cidr_block": "10.0.0.0/16", "tags": map[string]any{"env": "prod"}}},
		{"nested key", "vars.cidr_block", "10.0.0.0/16"},
		{"deeply nested", "vars.tags.env", "prod"},
		{"missing key", "vars.missing", nil},
		{"missing top-level", "missing", nil},
		{"array first element", "imports[0]", "catalog/vpc"},
		{"array second element", "imports[1]", "mixins/region"},
		{"array out of bounds", "imports[5]", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := lookupValue(data, tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestParseArrayIndex tests array index parsing.
func TestParseArrayIndex(t *testing.T) {
	tests := []struct {
		input  string
		expIdx int
		expKey string
		expOK  bool
	}{
		{"imports[0]", 0, "imports", true},
		{"imports[5]", 5, "imports", true},
		{"plain", 0, "", false},
		{"no_close[1", 0, "", false},
		{"vars", 0, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			idx, key, ok := parseArrayIndex(tt.input)
			assert.Equal(t, tt.expOK, ok)
			if tt.expOK {
				assert.Equal(t, tt.expIdx, idx)
				assert.Equal(t, tt.expKey, key)
			}
		})
	}
}

// TestFilterExplainPaths tests that internal paths are filtered out.
func TestFilterExplainPaths(t *testing.T) {
	paths := []string{
		"vars.cidr_block",
		"__import__:catalog/vpc",
		"__import_meta__:catalog/vpc",
		"settings.spacelift",
		"imports[0]",
	}

	result := filterExplainPaths(paths)

	require.Len(t, result, 3)
	assert.Contains(t, result, "vars.cidr_block")
	assert.Contains(t, result, "settings.spacelift")
	assert.Contains(t, result, "imports[0]")
	assert.NotContains(t, result, "__import__:catalog/vpc")
	assert.NotContains(t, result, "__import_meta__:catalog/vpc")
}

// TestRenderExplainTrace_Legend tests that the legend is included in output.
func TestRenderExplainTrace_Legend(t *testing.T) {
	ctx := m.NewMergeContext()
	ctx.EnableProvenance()

	ctx.RecordProvenance("vars.name", m.ProvenanceEntry{
		File:  "stack.yaml",
		Line:  5,
		Depth: 1,
		Value: "myname",
	})

	result := RenderExplainTrace(map[string]any{"vars": map[string]any{"name": "myname"}}, ctx, nil, "")

	// Legend should be present.
	assert.Contains(t, result, "Merge Trace Legend")
	assert.True(t, strings.Contains(result, "SET by") || strings.Contains(result, explainSetByLabel))
}

// TestRenderExplainTrace_MultipleKeys tests output with multiple keys.
func TestRenderExplainTrace_MultipleKeys(t *testing.T) {
	ctx := m.NewMergeContext()
	ctx.EnableProvenance()

	ctx.RecordProvenance("vars.cidr_block", m.ProvenanceEntry{
		File: "stack.yaml", Line: 5, Depth: 1, Value: "10.0.0.0/16",
	})
	ctx.RecordProvenance("vars.region", m.ProvenanceEntry{
		File: "defaults.yaml", Line: 8, Depth: 2, Value: "us-east-2",
	})

	data := map[string]any{
		"vars": map[string]any{
			"cidr_block": "10.0.0.0/16",
			"region":     "us-east-2",
		},
	}

	result := RenderExplainTrace(data, ctx, nil, "")

	assert.Contains(t, result, "vars.cidr_block")
	assert.Contains(t, result, "vars.region")
}
