package merge

import (
	"fmt"
	"strings"

	u "github.com/cloudposse/atmos/pkg/utils"
)

// MergeContext tracks file paths and import chains during merge operations
// to provide better error messages when merge conflicts occur.
// It also optionally tracks provenance information for configuration values.
//
// MergeContext implements the ProvenanceTracker interface, providing a concrete
// implementation for stack component provenance tracking.
type MergeContext struct {
	// CurrentFile is the file currently being processed.
	CurrentFile string

	// ImportChain tracks the chain of imports leading to the current file.
	// The first element is the root file, the last is the current file.
	ImportChain []string

	// ParentContext is the parent merge context for nested operations.
	ParentContext *MergeContext

	// Provenance stores optional provenance tracking for configuration values.
	// This is nil by default (provenance disabled) for zero performance overhead.
	// Call EnableProvenance() to activate provenance tracking.
	Provenance *ProvenanceStorage

	// Positions stores YAML position information for values in the current file.
	// This maps JSONPath-style paths to line/column positions in the YAML file.
	// Used by MergeWithProvenance to record accurate provenance data.
	Positions u.PositionMap
}

// NewMergeContext creates a new merge context.
func NewMergeContext() *MergeContext {
	return &MergeContext{
		ImportChain: []string{},
	}
}

// WithFile creates a new context for processing a specific file.
func (mc *MergeContext) WithFile(filePath string) *MergeContext {
	if mc == nil {
		mc = NewMergeContext()
	}

	newContext := &MergeContext{
		CurrentFile:   filePath,
		ImportChain:   append(mc.ImportChain, filePath),
		ParentContext: mc,
		Provenance:    mc.Provenance, // Share provenance storage across contexts.
		Positions:     nil,           // Positions are file-specific, reset for new file.
	}

	return newContext
}

// Clone creates a copy of the merge context.
func (mc *MergeContext) Clone() *MergeContext {
	if mc == nil {
		return NewMergeContext()
	}

	cloned := &MergeContext{
		CurrentFile:   mc.CurrentFile,
		ImportChain:   append([]string{}, mc.ImportChain...),
		ParentContext: mc.ParentContext,
		Positions:     mc.Positions, // Shallow copy is fine - positions are read-only.
	}

	// Clone provenance storage if enabled.
	if mc.Provenance != nil {
		cloned.Provenance = mc.Provenance.Clone()
	}

	return cloned
}

// FormatError formats an error with merge context information.
//
//nolint:revive // cognitive-complexity: detailed error formatting requires multiple conditions
func (mc *MergeContext) FormatError(err error, additionalInfo ...string) error {
	if err == nil {
		return nil
	}

	if mc == nil || (mc.CurrentFile == "" && len(mc.ImportChain) == 0) {
		// No context available, return original error unchanged
		return err
	}

	var sb strings.Builder

	// Build the context information (without including the error message itself)
	sb.WriteString("\n\n```\n")

	// Add current file being processed
	if mc.CurrentFile != "" {
		sb.WriteString(fmt.Sprintf("File being processed: %s", mc.CurrentFile))
	}

	// Add import chain if available
	if len(mc.ImportChain) > 0 {
		sb.WriteString("\nImport chain:")
		for i, file := range mc.ImportChain {
			var indent string
			if i == 0 {
				indent = "\n  → "
			} else {
				// Add proper indentation for nested imports
				indent = "\n    → "
			}
			sb.WriteString(fmt.Sprintf("%s%s", indent, file))
		}
	}

	// Add any additional information
	if len(additionalInfo) > 0 {
		for _, info := range additionalInfo {
			if info != "" {
				sb.WriteString("\n")
				sb.WriteString(info)
			}
		}
	}

	// Close the code fence
	sb.WriteString("\n```")

	// Add helpful hints for common merge errors
	errStr := err.Error()
	if strings.Contains(errStr, "cannot override two slices with different type") {
		sb.WriteString("\n\n**Likely cause:** A key is defined as an array in one file and as a string in another.")
		sb.WriteString("\n\n**Debug hint:** Check the files above for keys that have different types.")
		sb.WriteString("\n\n**Common issues:**")
		sb.WriteString("\n- `vars` defined as both array and string")
		sb.WriteString("\n- `settings` with inconsistent types across imports")
		sb.WriteString("\n- `overrides` attempting to change field types")
	} else if strings.Contains(errStr, "cannot override") {
		sb.WriteString("\n\n**Likely cause:** Type mismatch when merging configurations.")
		sb.WriteString("\n\n**Debug hint:** Ensure consistent types for the same keys across all files.")
	}

	// Wrap the original error to preserve the error chain
	// This allows errors.Is() to work with sentinel errors
	return fmt.Errorf("%w%s", err, sb.String())
}

// GetImportChainString returns a formatted string of the import chain.
func (mc *MergeContext) GetImportChainString() string {
	if mc == nil || len(mc.ImportChain) == 0 {
		return ""
	}

	return strings.Join(mc.ImportChain, " → ")
}

// GetDepth returns the depth of the import chain.
func (mc *MergeContext) GetDepth() int {
	if mc == nil {
		return 0
	}
	return len(mc.ImportChain)
}

// HasFile checks if a file is already in the import chain (to detect circular imports).
func (mc *MergeContext) HasFile(filePath string) bool {
	if mc == nil {
		return false
	}

	for _, file := range mc.ImportChain {
		if file == filePath {
			return true
		}
	}

	return false
}

// EnableProvenance activates provenance tracking for this context.
// This must be called before any provenance recording occurs.
// Once enabled, all merge operations will track the source of each value.
func (mc *MergeContext) EnableProvenance() {
	if mc == nil {
		return
	}

	if mc.Provenance == nil {
		mc.Provenance = NewProvenanceStorage()
	}
}

// RecordProvenance records provenance information for a value at the given path.
// This is a no-op if provenance is not enabled (Provenance is nil).
func (mc *MergeContext) RecordProvenance(path string, entry ProvenanceEntry) {
	if mc == nil || mc.Provenance == nil {
		return
	}

	mc.Provenance.Record(path, entry)
}

// GetProvenance returns the provenance chain for a given path.
// Returns nil if provenance is not enabled or no provenance exists for the path.
func (mc *MergeContext) GetProvenance(path string) []ProvenanceEntry {
	if mc == nil || mc.Provenance == nil {
		return nil
	}

	return mc.Provenance.Get(path)
}

// HasProvenance checks if provenance exists for a given path.
// Returns false if provenance is not enabled.
func (mc *MergeContext) HasProvenance(path string) bool {
	if mc == nil || mc.Provenance == nil {
		return false
	}

	return mc.Provenance.Has(path)
}

// GetProvenancePaths returns all paths that have provenance information.
// Returns nil if provenance is not enabled.
func (mc *MergeContext) GetProvenancePaths() []string {
	if mc == nil || mc.Provenance == nil {
		return nil
	}

	return mc.Provenance.GetPaths()
}

// IsProvenanceEnabled returns true if provenance tracking is enabled.
func (mc *MergeContext) IsProvenanceEnabled() bool {
	if mc == nil {
		return false
	}

	return mc.Provenance != nil
}

// GetProvenanceType returns the appropriate provenance type for the current file.
// Returns ProvenanceTypeInline if this is the root file (first in import chain),
// or ProvenanceTypeImport if this is an imported file.
func (mc *MergeContext) GetProvenanceType() ProvenanceType {
	if mc == nil {
		return ProvenanceTypeInline
	}

	// If import chain is empty or has only one file, it's the root.
	if len(mc.ImportChain) == 0 {
		return ProvenanceTypeInline
	}

	// The root stack file is the first element in the import chain (index 0).
	// If the current file matches the first element, it's defined at this level (inline).
	// Otherwise, it's imported.
	// NOTE: This logic is no longer used for inline provenance display, which now uses
	// file path matching at render time. This is kept for potential future use.
	if len(mc.ImportChain) > 0 && mc.CurrentFile == mc.ImportChain[0] {
		return ProvenanceTypeInline
	}

	return ProvenanceTypeImport
}
