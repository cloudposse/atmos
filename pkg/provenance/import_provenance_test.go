package provenance

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestImportProvenanceRendering tests that import array elements show individual provenance.
func TestImportProvenanceRendering(t *testing.T) {
	// Create test data with imports array.
	data := map[string]any{
		"import": []any{
			"catalog/vpc/defaults",
			"mixins/region/us-east-2",
			"orgs/acme/_defaults",
		},
		"vars": map[string]any{
			"enabled": true,
		},
	}

	// Create merge context with provenance for each import.
	ctx := m.NewMergeContext()
	ctx.EnableProvenance()

	// Record provenance for import array elements with different depths.
	ctx.RecordProvenance("import[0]", m.ProvenanceEntry{
		File:   "catalog/vpc/dev.yaml",
		Line:   5,
		Column: 3,
		Type:   m.ProvenanceTypeInline,
		Depth:  1,
	})

	ctx.RecordProvenance("import[1]", m.ProvenanceEntry{
		File:   "orgs/acme/plat/dev/us-east-2.yaml",
		Line:   3,
		Column: 3,
		Type:   m.ProvenanceTypeImport,
		Depth:  2,
	})

	ctx.RecordProvenance("import[2]", m.ProvenanceEntry{
		File:   "orgs/acme/plat/_defaults.yaml",
		Line:   2,
		Column: 3,
		Type:   m.ProvenanceTypeImport,
		Depth:  3,
	})

	// Record provenance for the import key itself.
	ctx.RecordProvenance("import", m.ProvenanceEntry{
		File:   "catalog/vpc/dev.yaml",
		Line:   4,
		Column: 1,
		Type:   m.ProvenanceTypeInline,
		Depth:  1,
	})

	// Record provenance for vars section.
	ctx.RecordProvenance("vars", m.ProvenanceEntry{
		File:   "catalog/vpc/defaults.yaml",
		Line:   10,
		Column: 1,
		Type:   m.ProvenanceTypeImport,
		Depth:  4,
	})

	ctx.RecordProvenance("vars.enabled", m.ProvenanceEntry{
		File:   "catalog/vpc/defaults.yaml",
		Line:   11,
		Column: 3,
		Type:   m.ProvenanceTypeImport,
		Depth:  4,
	})

	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{},
		},
	}

	// Render with provenance.
	output := RenderInlineProvenanceWithStackFile(data, ctx, atmosConfig, "catalog/vpc/dev.yaml")

	// Check that output contains the import section with provenance.
	assert.Contains(t, output, "import:", "Should have import section")

	// Check that each import element has provenance comment.
	assert.Contains(t, output, "catalog/vpc/defaults", "Should have first import")
	assert.Contains(t, output, "mixins/region/us-east-2", "Should have second import")
	assert.Contains(t, output, "orgs/acme/_defaults", "Should have third import")

	// Check for provenance comments with different symbols and depths.
	// import[0] at depth 1 should show ● [1].
	assert.Contains(t, output, "catalog/vpc/dev.yaml:5", "Should have provenance for first import")
	assert.Contains(t, output, "● [1]", "First import should show defined symbol with depth 1")

	// import[1] at depth 2 should show ○ [2].
	assert.Contains(t, output, "orgs/acme/plat/dev/us-east-2.yaml:3", "Should have provenance for second import")
	assert.Contains(t, output, "○ [2]", "Second import should show inherited symbol with depth 2")

	// import[2] at depth 3 should show ○ [3].
	assert.Contains(t, output, "orgs/acme/plat/_defaults.yaml:2", "Should have provenance for third import")
	assert.Contains(t, output, "○ [3]", "Third import should show inherited symbol with depth 3")
}

// TestImportProvenanceDepthZero tests that depth 1 imports show the defined symbol.
func TestImportProvenanceDepthZero(t *testing.T) {
	data := map[string]any{
		"import": []any{
			"catalog/vpc/dev",
			"mixins/region/us-east-2",
		},
	}

	ctx := m.NewMergeContext()
	ctx.EnableProvenance()

	// Both imports at depth 1 (defined in the parent stack file).
	ctx.RecordProvenance("import[0]", m.ProvenanceEntry{
		File:   "orgs/acme/plat/dev/us-east-2.yaml",
		Line:   5,
		Column: 3,
		Type:   m.ProvenanceTypeInline,
		Depth:  1,
	})

	ctx.RecordProvenance("import[1]", m.ProvenanceEntry{
		File:   "orgs/acme/plat/dev/us-east-2.yaml",
		Line:   6,
		Column: 3,
		Type:   m.ProvenanceTypeInline,
		Depth:  1,
	})

	ctx.RecordProvenance("import", m.ProvenanceEntry{
		File:   "orgs/acme/plat/dev/us-east-2.yaml",
		Line:   4,
		Column: 1,
		Type:   m.ProvenanceTypeInline,
		Depth:  1,
	})

	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{},
		},
	}

	// Render with the stack file matching the provenance file.
	output := RenderInlineProvenanceWithStackFile(data, ctx, atmosConfig, "orgs/acme/plat/dev/us-east-2.yaml")

	// Both imports should show ● [1] because they're defined in the current stack.
	assert.Contains(t, output, "catalog/vpc/dev", "Should have first import")
	assert.Contains(t, output, "mixins/region/us-east-2", "Should have second import")

	// Count occurrences of ● [1] - should appear at least twice (once for each import).
	// Note: We can't do exact count because the import key itself might also show ● [1].
	definedSymbolCount := 0
	for i := 0; i < len(output); {
		idx := findSubstring(output[i:], "● [1]")
		if idx == -1 {
			break
		}
		definedSymbolCount++
		i += idx + 6 // Move past this occurrence.
	}

	assert.GreaterOrEqual(t, definedSymbolCount, 2, "Should have at least 2 occurrences of ● [1] for the two imports")
}

// TestAllImportsHaveProvenance tests that every import in the list shows provenance.
// This is a regression test for the bug where some imports were missing provenance comments.
func TestAllImportsHaveProvenance(t *testing.T) {
	// Create a realistic scenario with many imports like a real stack.
	data := map[string]any{
		"import": []any{
			"catalog/vpc-flow-logs-bucket/defaults",
			"catalog/vpc/defaults",
			"catalog/vpc/dev",
			"catalog/vpc/ue2",
			"mixins/region/us-east-2",
			"mixins/stage/dev",
			"mixins/tenant/plat",
			"orgs/acme/_defaults",
			"orgs/acme/plat/_defaults",
			"orgs/acme/plat/dev/_defaults",
		},
	}

	ctx := m.NewMergeContext()
	ctx.EnableProvenance()

	// Record provenance for ALL import elements.
	// In the real implementation, every import must have been added from some file.
	importMetadata := []struct {
		file  string
		line  int
		depth int
	}{
		{"orgs/acme/plat/_defaults.yaml", 2, 2},
		{"orgs/acme/plat/_defaults.yaml", 3, 2},
		{"orgs/acme/plat/staging/us-west-2.yaml", 5, 0},
		{"catalog/vpc/ue2.yaml", 2, 3},
		{"orgs/acme/plat/dev/us-east-2.yaml", 3, 0},
		{"orgs/acme/plat/dev/_defaults.yaml", 3, 1},
		{"orgs/acme/plat/_defaults.yaml", 3, 2},
		{"orgs/acme/_defaults.yaml", 1, 3},
		{"orgs/acme/plat/_defaults.yaml", 2, 2},
		{"orgs/acme/plat/dev/us-east-2.yaml", 2, 0},
	}

	for i, meta := range importMetadata {
		arrayPath := fmt.Sprintf("import[%d]", i)
		provenanceType := m.ProvenanceTypeInline
		if meta.depth > 0 {
			provenanceType = m.ProvenanceTypeImport
		}
		ctx.RecordProvenance(arrayPath, m.ProvenanceEntry{
			File:   meta.file,
			Line:   meta.line,
			Column: 3,
			Type:   provenanceType,
			Depth:  meta.depth,
		})
	}

	// Record provenance for the import key itself.
	ctx.RecordProvenance("import", m.ProvenanceEntry{
		File:   "orgs/acme/plat/_defaults.yaml",
		Line:   1,
		Column: 1,
		Type:   m.ProvenanceTypeImport,
		Depth:  2,
	})

	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{},
		},
	}

	// Render with provenance.
	output := RenderInlineProvenanceWithStackFile(data, ctx, atmosConfig, "orgs/acme/plat/dev/us-east-2.yaml")

	// Split output into lines and count import lines with provenance.
	lines := strings.Split(output, "\n")
	importSectionStarted := false
	importLinesWithProvenance := 0
	totalImportLines := 0

	for _, line := range lines {
		if strings.Contains(line, "import:") {
			importSectionStarted = true
			continue
		}

		if !importSectionStarted {
			continue
		}

		// Check if this is an import item line (starts with "  - ")
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") {
			totalImportLines++
			// Check if this line has a provenance comment (contains "# ")
			if strings.Contains(line, "# ") && (strings.Contains(line, "●") || strings.Contains(line, "○")) {
				importLinesWithProvenance++
			}
			continue
		}

		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			// We've left the import section
			break
		}
	}

	// Every import line must have provenance.
	assert.Equal(t, 10, totalImportLines, "Should have 10 import lines")
	assert.Equal(t, 10, importLinesWithProvenance, "ALL 10 import lines must have provenance comments")

	// Verify the output contains all imports.
	for _, imp := range data["import"].([]any) {
		assert.Contains(t, output, imp.(string), "Should contain import: %s", imp.(string))
	}
}

// Helper function to find substring index.
func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
