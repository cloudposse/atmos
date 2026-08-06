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

// TestFilterEmptySectionsKeepsComponentSections is a regression test for a bug
// where every top-level component section (vars, metadata, settings, and
// others) was silently dropped from `describe component` output whenever the
// stack manifest had no identically-named stack-root section. Provenance for
// a component's own fields is always recorded under a
// "components.<type>.<component>." prefix (e.g.
// "components.terraform.app.vars.foo"), never under the bare section name
// ("vars") -- filterEmptySections must normalize before comparing, the same
// way findProvenance does, or it drops every section it should keep.
func TestFilterEmptySectionsKeepsComponentSections(t *testing.T) {
	ctx := m.NewMergeContext()
	ctx.EnableProvenance()
	ctx.RecordProvenance("components.terraform.app.vars.foo", m.ProvenanceEntry{
		File: "dev.yaml", Line: 5, Type: m.ProvenanceTypeInline, Depth: 1,
	})

	data := map[string]any{
		"vars":     map[string]any{"foo": "bar"},
		"backend":  map[string]any{},
		"metadata": map[string]any{},
	}

	filtered := filterEmptySections(data, ctx)
	filteredMap, ok := filtered.(map[string]any)
	if !ok {
		t.Fatalf("expected filterEmptySections to return a map, got %T", filtered)
	}

	if _, ok := filteredMap["vars"]; !ok {
		t.Errorf("expected 'vars' to survive filtering (has provenance under a prefixed path), but it was dropped: %v", filteredMap)
	}
	if _, ok := filteredMap["backend"]; ok {
		t.Errorf("expected empty 'backend' (no provenance recorded) to be filtered out, but it survived: %v", filteredMap)
	}
	if _, ok := filteredMap["metadata"]; ok {
		t.Errorf("expected empty 'metadata' (no provenance recorded) to be filtered out, but it survived: %v", filteredMap)
	}
}

// TestFilterEmptySectionsNilContextKeepsEverything verifies the documented
// behavior that a nil MergeContext (provenance tracking disabled) keeps every
// top-level key rather than filtering anything.
func TestFilterEmptySectionsNilContextKeepsEverything(t *testing.T) {
	data := map[string]any{
		"vars":    map[string]any{"foo": "bar"},
		"backend": map[string]any{},
	}

	filtered := filterEmptySections(data, nil)
	filteredMap, ok := filtered.(map[string]any)
	if !ok {
		t.Fatalf("expected filterEmptySections to return a map, got %T", filtered)
	}

	if len(filteredMap) != len(data) {
		t.Errorf("expected all %d keys to survive with a nil context, got %d: %v", len(data), len(filteredMap), filteredMap)
	}
}
