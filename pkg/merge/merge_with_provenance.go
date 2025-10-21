package merge

import (
	"sort"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// MergeWithProvenance merges multiple maps and records provenance information.
// It tracks where each value originated (file, line, column) and at what import depth.
//
// Parameters.
//   - atmosConfig: Atmos configuration with TrackProvenance flag.
//   - inputs: Maps to merge (in order: first is base, last wins).
//   - ctx: Merge context with provenance storage.
//   - positions: Map of JSONPath -> Position from YAML parsing.
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
	recordProvenanceRecursive(provenanceRecursiveParams{
		data:        result,
		currentPath: "",
		ctx:         ctx,
		positions:   positions,
		currentFile: ctx.CurrentFile,
		depth:       ctx.GetImportDepth(),
	})

	return result, nil
}

// provenanceRecursiveParams contains parameters for recursive provenance recording.
type provenanceRecursiveParams struct {
	data        any
	currentPath string
	ctx         *MergeContext
	positions   u.PositionMap
	currentFile string
	depth       int
}

// RecordMapProvenance records provenance for a map and its children.
func recordMapProvenance(params provenanceRecursiveParams, m map[string]any) {
	if params.currentPath != "" {
		recordProvenanceEntry(&provenanceEntryParams{
			path:           params.currentPath,
			value:          m,
			ctx:            params.ctx,
			positions:      params.positions,
			currentFile:    params.currentFile,
			depth:          params.depth,
			provenanceType: ProvenanceTypeInline,
		})
	}

	// Sort map keys to ensure deterministic iteration order.
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Iterate over sorted keys for deterministic provenance recording.
	for _, key := range keys {
		value := m[key]
		childPath := u.AppendJSONPathKey(params.currentPath, key)
		recordProvenanceRecursive(provenanceRecursiveParams{
			data:        value,
			currentPath: childPath,
			ctx:         params.ctx,
			positions:   params.positions,
			currentFile: params.currentFile,
			depth:       params.depth,
		})
	}
}

// recordArrayProvenance records provenance for an array and its elements.
func recordArrayProvenance(params provenanceRecursiveParams, arr []any) {
	if params.currentPath != "" {
		recordProvenanceEntry(&provenanceEntryParams{
			path:           params.currentPath,
			value:          arr,
			ctx:            params.ctx,
			positions:      params.positions,
			currentFile:    params.currentFile,
			depth:          params.depth,
			provenanceType: ProvenanceTypeInline,
		})
	}

	for i, item := range arr {
		childPath := u.AppendJSONPathIndex(params.currentPath, i)
		recordProvenanceRecursive(provenanceRecursiveParams{
			data:        item,
			currentPath: childPath,
			ctx:         params.ctx,
			positions:   params.positions,
			currentFile: params.currentFile,
			depth:       params.depth,
		})
	}
}

func recordProvenanceRecursive(params provenanceRecursiveParams) {
	if params.ctx == nil || !params.ctx.IsProvenanceEnabled() {
		return
	}

	if params.data == nil {
		if params.currentPath != "" {
			recordProvenanceEntry(&provenanceEntryParams{
				path:           params.currentPath,
				value:          nil,
				ctx:            params.ctx,
				positions:      params.positions,
				currentFile:    params.currentFile,
				depth:          params.depth,
				provenanceType: ProvenanceTypeInline,
			})
		}
		return
	}

	switch v := params.data.(type) {
	case map[string]any:
		recordMapProvenance(params, v)
	case []any:
		recordArrayProvenance(params, v)
	default:
		// Scalar value - record provenance.
		if params.currentPath != "" {
			recordProvenanceEntry(&provenanceEntryParams{
				path:           params.currentPath,
				value:          v,
				ctx:            params.ctx,
				positions:      params.positions,
				currentFile:    params.currentFile,
				depth:          params.depth,
				provenanceType: ProvenanceTypeInline,
			})
		}
	}
}

// provenanceEntryParams contains parameters for recording a provenance entry.
type provenanceEntryParams struct {
	path           string
	value          any
	ctx            *MergeContext
	positions      u.PositionMap
	currentFile    string
	depth          int
	provenanceType ProvenanceType
}

// recordProvenanceEntry records a single provenance entry for a path.
func recordProvenanceEntry(params *provenanceEntryParams) {
	if params.ctx == nil || !params.ctx.IsProvenanceEnabled() {
		return
	}

	// Get position from the position map.
	pos := u.GetYAMLPosition(params.positions, params.path)

	// Determine provenance type based on depth.
	pType := params.provenanceType
	if params.depth > 0 {
		pType = ProvenanceTypeImport
	}

	// Record the provenance entry.
	entry := ProvenanceEntry{
		File:      params.currentFile,
		Line:      pos.Line,
		Column:    pos.Column,
		Type:      pType,
		ValueHash: hashValue(params.value),
		Depth:     params.depth,
	}

	params.ctx.RecordProvenance(params.path, entry)
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
