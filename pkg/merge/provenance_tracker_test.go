package merge

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMergeContextImplementsProvenanceTracker verifies that MergeContext
// correctly implements the ProvenanceTracker interface.
func TestMergeContextImplementsProvenanceTracker(t *testing.T) {
	// This test ensures MergeContext implements ProvenanceTracker.
	// The compile-time check in provenance_tracker.go catches interface compliance,
	// but this test documents the behavior and ensures methods work as expected.

	var tracker ProvenanceTracker = NewMergeContext()

	// Initially, provenance should be disabled.
	assert.False(t, tracker.IsProvenanceEnabled(), "Provenance should be disabled initially")

	// EnableProvenance should activate tracking.
	tracker.EnableProvenance()
	assert.True(t, tracker.IsProvenanceEnabled(), "Provenance should be enabled after EnableProvenance()")

	// Recording provenance should work.
	entry := ProvenanceEntry{
		File:   "test.yaml",
		Line:   10,
		Column: 5,
		Type:   ProvenanceTypeInline,
		Depth:  0,
	}

	tracker.RecordProvenance("vars.name", entry)

	// HasProvenance should return true for recorded path.
	assert.True(t, tracker.HasProvenance("vars.name"), "HasProvenance should return true for recorded path")
	assert.False(t, tracker.HasProvenance("vars.missing"), "HasProvenance should return false for missing path")

	// GetProvenance should return the recorded entry.
	provenance := tracker.GetProvenance("vars.name")
	assert.Len(t, provenance, 1, "Should have one provenance entry")
	assert.Equal(t, "test.yaml", provenance[0].File)
	assert.Equal(t, 10, provenance[0].Line)

	// GetProvenancePaths should return all recorded paths.
	paths := tracker.GetProvenancePaths()
	assert.Contains(t, paths, "vars.name", "Should contain recorded path")
}

// TestProvenanceTrackerDisabled verifies behavior when provenance is disabled.
func TestProvenanceTrackerDisabled(t *testing.T) {
	tracker := NewMergeContext()

	// Don't enable provenance
	assert.False(t, tracker.IsProvenanceEnabled())

	// Recording should be a no-op when disabled.
	entry := ProvenanceEntry{
		File: "test.yaml",
		Line: 10,
	}

	tracker.RecordProvenance("vars.name", entry)

	// Should not have provenance since tracking is disabled.
	assert.False(t, tracker.HasProvenance("vars.name"))
	assert.Nil(t, tracker.GetProvenance("vars.name"))
	assert.Nil(t, tracker.GetProvenancePaths())
}

// TestProvenanceInheritanceChain tests recording multiple entries for the same path.
func TestProvenanceInheritanceChain(t *testing.T) {
	tracker := NewMergeContext()
	tracker.EnableProvenance()

	// Record base value.
	baseEntry := ProvenanceEntry{
		File:  "base.yaml",
		Line:  5,
		Type:  ProvenanceTypeImport,
		Depth: 1,
	}
	tracker.RecordProvenance("vars.replicas", baseEntry)

	// Record override value.
	overrideEntry := ProvenanceEntry{
		File:  "override.yaml",
		Line:  10,
		Type:  ProvenanceTypeOverride,
		Depth: 0,
	}
	tracker.RecordProvenance("vars.replicas", overrideEntry)

	// Should have two entries in the chain.
	chain := tracker.GetProvenance("vars.replicas")
	assert.Len(t, chain, 2, "Should have inheritance chain with 2 entries")

	// Verify order: base → override.
	assert.Equal(t, "base.yaml", chain[0].File, "First entry should be base")
	assert.Equal(t, "override.yaml", chain[1].File, "Second entry should be override")
}

// TestProvenanceNestedPaths tests provenance for nested configuration paths.
func TestProvenanceNestedPaths(t *testing.T) {
	tracker := NewMergeContext()
	tracker.EnableProvenance()

	// Record provenance for nested paths.
	tracker.RecordProvenance("vars", ProvenanceEntry{File: "root.yaml", Line: 1})
	tracker.RecordProvenance("vars.tags", ProvenanceEntry{File: "tags.yaml", Line: 5})
	tracker.RecordProvenance("vars.tags.environment", ProvenanceEntry{File: "env.yaml", Line: 10})

	// All paths should have provenance.
	assert.True(t, tracker.HasProvenance("vars"))
	assert.True(t, tracker.HasProvenance("vars.tags"))
	assert.True(t, tracker.HasProvenance("vars.tags.environment"))

	// GetProvenancePaths should return all paths.
	paths := tracker.GetProvenancePaths()
	assert.Len(t, paths, 3, "Should have 3 paths recorded")
	assert.Contains(t, paths, "vars")
	assert.Contains(t, paths, "vars.tags")
	assert.Contains(t, paths, "vars.tags.environment")
}

// MockProvenanceTracker is a minimal implementation for testing the interface contract.
type MockProvenanceTracker struct {
	enabled    bool
	provenance map[string][]ProvenanceEntry
}

// NewMockProvenanceTracker creates a mock tracker for testing.
func NewMockProvenanceTracker() *MockProvenanceTracker {
	return &MockProvenanceTracker{
		provenance: make(map[string][]ProvenanceEntry),
	}
}

// RecordProvenance implements ProvenanceTracker.
func (m *MockProvenanceTracker) RecordProvenance(path string, entry ProvenanceEntry) {
	if !m.enabled {
		return
	}
	m.provenance[path] = append(m.provenance[path], entry)
}

// GetProvenance implements ProvenanceTracker.
func (m *MockProvenanceTracker) GetProvenance(path string) []ProvenanceEntry {
	if !m.enabled {
		return nil
	}
	return m.provenance[path]
}

// HasProvenance implements ProvenanceTracker.
func (m *MockProvenanceTracker) HasProvenance(path string) bool {
	if !m.enabled {
		return false
	}
	_, exists := m.provenance[path]
	return exists
}

// GetProvenancePaths implements ProvenanceTracker.
func (m *MockProvenanceTracker) GetProvenancePaths() []string {
	if !m.enabled {
		return nil
	}
	paths := make([]string, 0, len(m.provenance))
	for path := range m.provenance {
		paths = append(paths, path)
	}
	return paths
}

// IsProvenanceEnabled implements ProvenanceTracker.
func (m *MockProvenanceTracker) IsProvenanceEnabled() bool {
	return m.enabled
}

// EnableProvenance implements ProvenanceTracker.
func (m *MockProvenanceTracker) EnableProvenance() {
	m.enabled = true
}

// Verify MockProvenanceTracker implements the interface at compile time.
var _ ProvenanceTracker = (*MockProvenanceTracker)(nil)

// TestMockProvenanceTracker verifies the mock implementation.
func TestMockProvenanceTracker(t *testing.T) {
	// This test verifies that alternative implementations of ProvenanceTracker work correctly.
	// This demonstrates that the interface is generic and not tightly coupled to MergeContext.

	var tracker ProvenanceTracker = NewMockProvenanceTracker()

	// Initially disabled.
	assert.False(t, tracker.IsProvenanceEnabled())

	// Enable tracking.
	tracker.EnableProvenance()
	assert.True(t, tracker.IsProvenanceEnabled())

	// Record provenance.
	entry := ProvenanceEntry{
		File: "config.yaml",
		Line: 42,
	}
	tracker.RecordProvenance("setting.value", entry)

	// Verify recording.
	assert.True(t, tracker.HasProvenance("setting.value"))
	provenance := tracker.GetProvenance("setting.value")
	assert.Len(t, provenance, 1)
	assert.Equal(t, "config.yaml", provenance[0].File)
	assert.Equal(t, 42, provenance[0].Line)
}

// TestProvenanceTrackerInterfaceContract tests that any ProvenanceTracker
// implementation follows the expected contract.
func TestProvenanceTrackerInterfaceContract(t *testing.T) {
	// Test with multiple implementations to ensure interface works generically.
	implementations := []struct {
		name    string
		tracker ProvenanceTracker
	}{
		{"MergeContext", NewMergeContext()},
		{"MockTracker", NewMockProvenanceTracker()},
	}

	for _, impl := range implementations {
		t.Run(impl.name, func(t *testing.T) {
			tracker := impl.tracker

			// Contract: Initially disabled.
			assert.False(t, tracker.IsProvenanceEnabled(), "Should be disabled initially")

			// Contract: EnableProvenance enables tracking.
			tracker.EnableProvenance()
			assert.True(t, tracker.IsProvenanceEnabled(), "Should be enabled after EnableProvenance")

			// Contract: Recording and retrieval work.
			entry := ProvenanceEntry{File: "test.yaml", Line: 1}
			tracker.RecordProvenance("key", entry)

			assert.True(t, tracker.HasProvenance("key"), "Should have provenance after recording")
			assert.False(t, tracker.HasProvenance("missing"), "Should not have provenance for unrecorded key")

			provenance := tracker.GetProvenance("key")
			assert.NotNil(t, provenance, "GetProvenance should not return nil for recorded key")
			assert.Len(t, provenance, 1, "Should have one entry")

			paths := tracker.GetProvenancePaths()
			assert.Contains(t, paths, "key", "GetProvenancePaths should include recorded key")
		})
	}
}
