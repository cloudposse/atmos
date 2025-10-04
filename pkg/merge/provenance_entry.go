package merge

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// ProvenanceType represents the type of provenance entry.
type ProvenanceType string

const (
	// ProvenanceTypeImport indicates the value was imported from another file.
	ProvenanceTypeImport ProvenanceType = "import"

	// ProvenanceTypeInline indicates the value was defined inline in the current file.
	ProvenanceTypeInline ProvenanceType = "inline"

	// ProvenanceTypeOverride indicates the value overrides a previous value.
	ProvenanceTypeOverride ProvenanceType = "override"

	// ProvenanceTypeComputed indicates the value was computed (e.g., from a template).
	ProvenanceTypeComputed ProvenanceType = "computed"

	// ProvenanceTypeDefault indicates the value is a default value.
	ProvenanceTypeDefault ProvenanceType = "default"
)

// ProvenanceEntry represents the provenance information for a single value.
// It tracks where a value came from in the configuration file hierarchy.
type ProvenanceEntry struct {
	// File is the source file path where this value was defined.
	File string

	// Line is the line number (1-indexed) where this value appears.
	Line int

	// Column is the column number (1-indexed) where this value starts.
	Column int

	// Type indicates how this value was introduced (import, inline, override, computed, default).
	Type ProvenanceType

	// ValueHash is a hash of the value for change detection.
	// This allows detecting when a value was overridden with the same value.
	ValueHash string

	// Depth is the inheritance depth (0=parent stack, 1=direct import, 2+=nested imports).
	Depth int
}

// ProvenanceEntryParams contains parameters for creating a provenance entry.
type ProvenanceEntryParams struct {
	File   string
	Line   int
	Column int
	Type   ProvenanceType
	Value  any
	Depth  int
}

// NewProvenanceEntry creates a new provenance entry.
func NewProvenanceEntry(params ProvenanceEntryParams) *ProvenanceEntry {
	return &ProvenanceEntry{
		File:      params.File,
		Line:      params.Line,
		Column:    params.Column,
		Type:      params.Type,
		ValueHash: hashValue(params.Value),
		Depth:     params.Depth,
	}
}

// String returns a human-readable representation of the provenance entry.
func (p *ProvenanceEntry) String() string {
	if p == nil {
		return "<nil>"
	}

	if p.Column > 0 {
		return fmt.Sprintf("%s:%d:%d [%d] (%s)", p.File, p.Line, p.Column, p.Depth, p.Type)
	}

	return fmt.Sprintf("%s:%d [%d] (%s)", p.File, p.Line, p.Depth, p.Type)
}

// Equals checks if two provenance entries are equal.
// Two entries are equal if they have the same file, line, column, type, value hash, and depth.
func (p *ProvenanceEntry) Equals(other *ProvenanceEntry) bool {
	if p == nil || other == nil {
		return p == other
	}

	return p.File == other.File &&
		p.Line == other.Line &&
		p.Column == other.Column &&
		p.Type == other.Type &&
		p.ValueHash == other.ValueHash &&
		p.Depth == other.Depth
}

// Clone creates a deep copy of the provenance entry.
func (p *ProvenanceEntry) Clone() *ProvenanceEntry {
	if p == nil {
		return nil
	}

	return &ProvenanceEntry{
		File:      p.File,
		Line:      p.Line,
		Column:    p.Column,
		Type:      p.Type,
		ValueHash: p.ValueHash,
		Depth:     p.Depth,
	}
}

// IsValid checks if the provenance entry has valid data.
// A valid entry must have a file path and a positive line number.
func (p *ProvenanceEntry) IsValid() bool {
	if p == nil {
		return false
	}

	return p.File != "" && p.Line > 0
}

// hashValue creates a hash of a value for change detection.
func hashValue(value any) string {
	if value == nil {
		return ""
	}

	// Convert value to a deterministic string representation.
	valueBytes, err := json.Marshal(value)
	valueStr := ""
	if err == nil {
		valueStr = string(valueBytes)
	} else {
		valueStr = fmt.Sprintf("%v", value)
	}

	// Create SHA-256 hash
	hash := sha256.Sum256([]byte(valueStr))

	// Return hex representation (first 16 characters for brevity)
	return fmt.Sprintf("%x", hash[:8])
}
