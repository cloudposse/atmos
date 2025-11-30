package provenance

import (
	"testing"

	m "github.com/cloudposse/atmos/pkg/merge"
)

// TestUpdateImportsProvenancePreservesChain tests that the full provenance chain
// is preserved when renaming "imports" to "import".
// This is a regression test for the bug where only the last entry was copied,
// losing the inheritance history.
func TestUpdateImportsProvenancePreservesChain(t *testing.T) {
	ctx := m.NewMergeContext()
	ctx.EnableProvenance()

	// Simulate inheritance chain: base.yaml defines it at depth 2,
	// then override.yaml overrides it at depth 1.
	// This creates multiple provenance entries for the same key.
	ctx.RecordProvenance("imports[0]", m.ProvenanceEntry{
		File:  "base.yaml",
		Line:  10,
		Type:  m.ProvenanceTypeImport,
		Depth: 2,
	})
	ctx.RecordProvenance("imports[0]", m.ProvenanceEntry{
		File:  "override.yaml",
		Line:  5,
		Type:  m.ProvenanceTypeInline,
		Depth: 1,
	})

	// Before: verify we have 2 entries
	beforeEntries := ctx.GetProvenance("imports[0]")
	if len(beforeEntries) != 2 {
		t.Fatalf("Setup failed: expected 2 entries, got %d", len(beforeEntries))
	}

	// Run the rename function
	updateImportsProvenance(ctx)

	// After: verify we still have 2 entries (full chain preserved)
	afterEntries := ctx.GetProvenance("import[0]")
	if len(afterEntries) != len(beforeEntries) {
		t.Errorf("Chain broken! Before: %d entries, After: %d entries", len(beforeEntries), len(afterEntries))
		t.Logf("Before entries:")
		for i, e := range beforeEntries {
			t.Logf("  [%d] %s:%d (depth %d)", i, e.File, e.Line, e.Depth)
		}
		t.Logf("After entries:")
		for i, e := range afterEntries {
			t.Logf("  [%d] %s:%d (depth %d)", i, e.File, e.Line, e.Depth)
		}
		return
	}

	// Verify entries are in the right order
	if afterEntries[0].File != "base.yaml" || afterEntries[0].Depth != 2 {
		t.Errorf("First entry incorrect: got %s depth %d, want base.yaml depth 2",
			afterEntries[0].File, afterEntries[0].Depth)
	}
	if afterEntries[1].File != "override.yaml" || afterEntries[1].Depth != 1 {
		t.Errorf("Second entry incorrect: got %s depth %d, want override.yaml depth 1",
			afterEntries[1].File, afterEntries[1].Depth)
	}
}
