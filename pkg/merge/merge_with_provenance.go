package merge

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// MergeWithProvenance merges multiple maps and records provenance information.
// It tracks where each value originated (file, line, column) and at what import depth.
//
// Parameters:
//   - atmosConfig: Atmos configuration with TrackProvenance flag
//   - inputs: Maps to merge (in order: first is base, last wins)
//   - ctx: Merge context with provenance storage
//   - positions: Map of JSONPath -> Position from YAML parsing
//
// Returns the merged map with provenance recorded in ctx.
func MergeWithProvenance(
	atmosConfig *schema.AtmosConfiguration,
	inputs []map[string]any,
	ctx *MergeContext,
	positions u.PositionMap,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "merge.MergeWithProvenance")()

	// If provenance is disabled, fall back to regular merge.
	if atmosConfig == nil || !atmosConfig.TrackProvenance || ctx == nil || !ctx.IsProvenanceEnabled() {
		return Merge(atmosConfig, inputs)
	}

	// Perform the merge using standard logic.
	result, err := Merge(atmosConfig, inputs)
	if err != nil {
		return nil, err
	}

	// Record provenance for all values in the result.
	// Walk the merged result and record where each value came from.
	recordProvenanceRecursive(result, "", ctx, positions, ctx.CurrentFile, ctx.GetImportDepth())

	return result, nil
}

// recordProvenanceRecursive walks a data structure and records provenance for all paths.
func recordProvenanceRecursive(
	data any,
	currentPath string,
	ctx *MergeContext,
	positions u.PositionMap,
	currentFile string,
	depth int,
) {
	if data == nil || ctx == nil || !ctx.IsProvenanceEnabled() {
		return
	}

	switch v := data.(type) {
	case map[string]any:
		// Record provenance for the map itself if it has a path.
		if currentPath != "" {
			recordProvenanceEntry(currentPath, ctx, positions, currentFile, depth, ProvenanceTypeInline)
		}

		// Recurse into map values.
		for key, value := range v {
			childPath := u.AppendJSONPathKey(currentPath, key)
			recordProvenanceRecursive(value, childPath, ctx, positions, currentFile, depth)
		}

	case []any:
		// Record provenance for the array itself if it has a path.
		if currentPath != "" {
			recordProvenanceEntry(currentPath, ctx, positions, currentFile, depth, ProvenanceTypeInline)
		}

		// Recurse into array elements.
		for i, item := range v {
			childPath := u.AppendJSONPathIndex(currentPath, i)
			recordProvenanceRecursive(item, childPath, ctx, positions, currentFile, depth)
		}

	default:
		// Scalar value - record provenance.
		if currentPath != "" {
			recordProvenanceEntry(currentPath, ctx, positions, currentFile, depth, ProvenanceTypeInline)
		}
	}
}

// recordProvenanceEntry records a single provenance entry for a path.
func recordProvenanceEntry(
	path string,
	ctx *MergeContext,
	positions u.PositionMap,
	currentFile string,
	depth int,
	provenanceType ProvenanceType,
) {
	if ctx == nil || !ctx.IsProvenanceEnabled() {
		return
	}

	// Get position from the position map.
	pos := u.GetYAMLPosition(positions, path)

	// Determine provenance type based on depth.
	pType := provenanceType
	if depth > 0 {
		pType = ProvenanceTypeImport
	}

	// Record the provenance entry.
	entry := ProvenanceEntry{
		File:   currentFile,
		Line:   pos.Line,
		Column: pos.Column,
		Type:   pType,
		Depth:  depth,
	}

	ctx.RecordProvenance(path, entry)
}

// GetImportDepth returns the current import depth from the merge context.
// Returns 0 if context is nil or has no parent chain.
func (c *MergeContext) GetImportDepth() int {
	if c == nil {
		return 0
	}

	// Count the depth by walking up the parent chain.
	depth := 0
	current := c
	for current != nil && current.ParentContext != nil {
		depth++
		current = current.ParentContext
	}

	return depth
}
