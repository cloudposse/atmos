package merge

import (
	"sort"
	"sync"
)

// ProvenanceStorage stores provenance information for configuration values.
// It maps JSONPath-style paths to a chain of provenance entries showing
// the inheritance/override history of each value.
type ProvenanceStorage struct {
	// entries maps JSONPath to a chain of provenance entries.
	// The chain is ordered from base → override, with the last entry
	// being the final (current) value.
	entries map[string][]ProvenanceEntry

	// mutex protects concurrent access to entries.
	mutex sync.RWMutex
}

// NewProvenanceStorage creates a new provenance storage.
func NewProvenanceStorage() *ProvenanceStorage {
	return &ProvenanceStorage{
		entries: make(map[string][]ProvenanceEntry),
	}
}

// Record adds a provenance entry for a given path.
// Multiple entries can be recorded for the same path to track inheritance chains.
func (ps *ProvenanceStorage) Record(path string, entry ProvenanceEntry) {
	if ps == nil {
		return
	}

	ps.mutex.Lock()
	defer ps.mutex.Unlock()

	ps.entries[path] = append(ps.entries[path], entry)
}

// Get returns the provenance chain for a given path.
// Returns nil if no provenance exists for the path.
// The returned slice is ordered from base → override.
func (ps *ProvenanceStorage) Get(path string) []ProvenanceEntry {
	if ps == nil {
		return nil
	}

	ps.mutex.RLock()
	defer ps.mutex.RUnlock()

	entries, exists := ps.entries[path]
	if !exists {
		return nil
	}

	// Return a copy to prevent external modification.
	result := make([]ProvenanceEntry, len(entries))
	copy(result, entries)

	return result
}

// Has checks if provenance exists for a given path.
func (ps *ProvenanceStorage) Has(path string) bool {
	if ps == nil {
		return false
	}

	ps.mutex.RLock()
	defer ps.mutex.RUnlock()

	_, exists := ps.entries[path]
	return exists
}

// GetPaths returns all paths that have provenance information.
// The paths are returned in sorted order for consistent iteration.
func (ps *ProvenanceStorage) GetPaths() []string {
	if ps == nil {
		return nil
	}

	ps.mutex.RLock()
	defer ps.mutex.RUnlock()

	paths := make([]string, 0, len(ps.entries))
	for path := range ps.entries {
		paths = append(paths, path)
	}

	// Sort for consistent ordering.
	sort.Strings(paths)

	return paths
}

// Clear removes all provenance entries.
func (ps *ProvenanceStorage) Clear() {
	if ps == nil {
		return
	}

	ps.mutex.Lock()
	defer ps.mutex.Unlock()

	ps.entries = make(map[string][]ProvenanceEntry)
}

// Clone creates a deep copy of the provenance storage.
func (ps *ProvenanceStorage) Clone() *ProvenanceStorage {
	if ps == nil {
		return nil
	}

	ps.mutex.RLock()
	defer ps.mutex.RUnlock()

	clone := NewProvenanceStorage()

	for path, entries := range ps.entries {
		clonedEntries := make([]ProvenanceEntry, len(entries))
		for i, entry := range entries {
			clonedEntries[i] = *entry.Clone()
		}
		clone.entries[path] = clonedEntries
	}

	return clone
}

// Size returns the number of paths with provenance information.
func (ps *ProvenanceStorage) Size() int {
	if ps == nil {
		return 0
	}

	ps.mutex.RLock()
	defer ps.mutex.RUnlock()

	return len(ps.entries)
}

// Remove deletes provenance information for a given path.
func (ps *ProvenanceStorage) Remove(path string) {
	if ps == nil {
		return
	}

	ps.mutex.Lock()
	defer ps.mutex.Unlock()

	delete(ps.entries, path)
}

// GetLatest returns the most recent (final) provenance entry for a path.
// Returns nil if no provenance exists for the path.
func (ps *ProvenanceStorage) GetLatest(path string) *ProvenanceEntry {
	if ps == nil {
		return nil
	}

	ps.mutex.RLock()
	defer ps.mutex.RUnlock()

	entries, exists := ps.entries[path]
	if !exists || len(entries) == 0 {
		return nil
	}

	// Return a copy of the last entry.
	latest := entries[len(entries)-1]
	return latest.Clone()
}
