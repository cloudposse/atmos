package merge

// ProvenanceTracker provides a generic interface for tracking configuration provenance.
// This interface allows different configuration systems (stacks, atmos.yaml, vendor, workflows)
// to implement provenance tracking in a consistent way.
//
// Implementations should:
//   - Track the source file, line, and column for each configuration value
//   - Support hierarchical paths (JSONPath-style) for nested values
//   - Record inheritance/override chains when values are merged
//   - Be thread-safe if used in concurrent scenarios
//
// Example implementations:
//   - MergeContext: Tracks provenance for stack component merging
//   - Future: AtmosConfigContext for atmos.yaml imports
//   - Future: VendorContext for vendor.yaml provenance
//   - Future: WorkflowContext for workflow definitions
type ProvenanceTracker interface {
	// RecordProvenance records provenance information for a value at the given path.
	// The path should use JSONPath-style syntax (e.g., "vars.tags.environment").
	// Multiple entries for the same path represent the inheritance chain (base → override).
	RecordProvenance(path string, entry ProvenanceEntry)

	// GetProvenance returns the complete provenance chain for a given path.
	// The chain is ordered from base → override, with the last entry being the final value.
	// Returns nil if no provenance exists for the path.
	GetProvenance(path string) []ProvenanceEntry

	// HasProvenance checks if provenance information exists for the given path.
	// Returns false if provenance tracking is disabled or path has no provenance.
	HasProvenance(path string) bool

	// GetProvenancePaths returns all paths that have provenance information.
	// Returns nil if provenance tracking is disabled.
	// The order of paths is not guaranteed.
	GetProvenancePaths() []string

	// IsProvenanceEnabled returns true if provenance tracking is currently active.
	// When disabled, RecordProvenance calls should be no-ops for performance.
	IsProvenanceEnabled() bool

	// EnableProvenance activates provenance tracking for this tracker.
	// This should be called before any provenance recording occurs.
	// Once enabled, all merge/load operations should track provenance.
	EnableProvenance()
}

// Verify that MergeContext implements ProvenanceTracker at compile time.
var _ ProvenanceTracker = (*MergeContext)(nil)
