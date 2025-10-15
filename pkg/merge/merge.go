package merge

import (
	"errors"
	"fmt"
	"reflect"
	"sync"

	"dario.cat/mergo"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	ListMergeStrategyReplace = "replace"
	ListMergeStrategyAppend  = "append"
	ListMergeStrategyMerge   = "merge"

	// Initial capacity for pooled maps and slices.
	// This is a reasonable default based on typical component configuration sizes.
	initialMapCapacity   = 16
	initialSliceCapacity = 8
)

var (
	// MapPool provides reusable map[string]any objects to reduce allocations during deep copy operations.
	// Maps from the pool are cleared and resized before use, then returned to the caller.
	// This reduces GC pressure during high-volume merge operations (118k+ calls per run).
	mapPool = sync.Pool{
		New: func() interface{} {
			return make(map[string]any, initialMapCapacity)
		},
	}

	// SlicePool provides reusable []any slices to reduce allocations during deep copy operations.
	// Slices from the pool are cleared and resized before use, then returned to the caller.
	slicePool = sync.Pool{
		New: func() interface{} {
			return make([]any, 0, initialSliceCapacity)
		},
	}
)

// deepCopyMap performs a deep copy of a map optimized for map[string]any structures.
// This custom implementation avoids reflection overhead for common cases (maps, slices, primitives)
// and only falls back to copystructure for rare complex types.
// Preserves numeric types (unlike JSON which converts all numbers to float64) and is faster than
// reflection-based copying. The data is already in Go map format with custom tags already processed,
// so we only need structural copying to work around mergo's pointer mutation bug.
// Uses object pooling to reduce allocations and GC pressure during high-volume operations.
func deepCopyMap(m map[string]any) (map[string]any, error) {
	defer perf.Track(nil, "merge.deepCopyMap")()

	if m == nil {
		return nil, nil
	}

	// Get a map from the pool to reduce allocations.
	result := mapPool.Get().(map[string]any)

	// Clear the map in case it has leftover data from previous use.
	for k := range result {
		delete(result, k)
	}

	// Copy all key-value pairs.
	for k, v := range m {
		result[k] = deepCopyValue(v)
	}

	return result, nil
}

// deepCopyValue performs a deep copy of a value, handling common types without reflection.
// Falls back to copystructure for rare complex types.
// Uses object pooling for maps and slices to reduce allocations.
func deepCopyValue(v any) any {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case map[string]any:
		// Common case: nested map - use pool and recurse with fast path.
		result := mapPool.Get().(map[string]any)

		// Clear the map in case it has leftover data from previous use.
		for k := range result {
			delete(result, k)
		}

		// Copy all key-value pairs.
		for k, v := range val {
			result[k] = deepCopyValue(v)
		}
		return result

	case []any:
		// Common case: slice - use pool and recurse with fast path.
		result := slicePool.Get().([]any)

		// Clear and resize the slice.
		result = result[:0]
		if cap(result) < len(val) {
			// Need more capacity, allocate new slice.
			result = make([]any, len(val))
		} else {
			// Reuse existing capacity.
			result = result[:len(val)]
		}

		// Copy all elements.
		for i, item := range val {
			result[i] = deepCopyValue(item)
		}
		return result

	case string, int, int64, int32, int16, int8,
		uint, uint64, uint32, uint16, uint8,
		float64, float32, bool:
		// Common case: immutable primitives - return as-is (no copy needed).
		return v

	default:
		// Rare case: complex types - use reflection-based normalization.
		// This handles typed slices/maps that need conversion to []any/map[string]any.
		return normalizeValueReflect(v)
	}
}

// getEmptyPooledMap returns an empty map from the pool, clearing any leftover data.
func getEmptyPooledMap() map[string]any {
	result := mapPool.Get().(map[string]any)
	for k := range result {
		delete(result, k)
	}
	return result
}

// normalizeSliceReflect converts a typed slice to []any using reflection and pooling.
func normalizeSliceReflect(rv reflect.Value) []any {
	result := slicePool.Get().([]any)
	result = result[:0]

	sliceLen := rv.Len()
	if cap(result) < sliceLen {
		// Need more capacity, allocate new slice.
		result = make([]any, sliceLen)
	} else {
		// Reuse existing capacity.
		result = result[:sliceLen]
	}

	for i := 0; i < sliceLen; i++ {
		result[i] = deepCopyValue(rv.Index(i).Interface())
	}
	return result
}

// normalizeMapReflect converts a typed map with string keys to map[string]any using reflection and pooling.
func normalizeMapReflect(rv reflect.Value, value any) any {
	// Empty map - return empty pooled map.
	if rv.Len() == 0 {
		return getEmptyPooledMap()
	}

	// Check if keys are strings.
	iter := rv.MapRange()
	if !iter.Next() {
		return getEmptyPooledMap()
	}

	// Non-string keys - return as-is.
	if iter.Key().Kind() != reflect.String {
		return value
	}

	// Convert to map[string]any.
	result := getEmptyPooledMap()

	// Process first key-value pair.
	result[iter.Key().String()] = deepCopyValue(iter.Value().Interface())
	// Process remaining pairs.
	for iter.Next() {
		result[iter.Key().String()] = deepCopyValue(iter.Value().Interface())
	}
	return result
}

// normalizeValueReflect uses reflection to normalize typed slices and maps.
// This is used as a fallback for complex types that aren't handled by the fast path.
// Uses object pooling for the resulting maps and slices.
func normalizeValueReflect(value any) any {
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Slice:
		return normalizeSliceReflect(rv)
	case reflect.Map:
		return normalizeMapReflect(rv, value)
	default:
		// Primitives and other types - return as-is.
		return value
	}
}

// MergeWithOptions takes a list of maps and options as input, deep-merges the items in the order they are defined in the list,
// and returns a single map with the merged contents.
func MergeWithOptions(
	atmosConfig *schema.AtmosConfiguration,
	inputs []map[string]any,
	appendSlice bool,
	sliceDeepCopy bool,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "merge.MergeWithOptions")()

	// Fast-path: empty inputs.
	if len(inputs) == 0 {
		return map[string]any{}, nil
	}

	// Fast-path: filter out empty maps and check for trivial cases.
	nonEmptyInputs := make([]map[string]any, 0, len(inputs))
	for _, input := range inputs {
		if len(input) > 0 {
			nonEmptyInputs = append(nonEmptyInputs, input)
		}
	}

	// Fast-path: all inputs were empty.
	if len(nonEmptyInputs) == 0 {
		return map[string]any{}, nil
	}

	// Fast-path: only one non-empty input, return a deep copy to maintain immutability.
	if len(nonEmptyInputs) == 1 {
		return deepCopyMap(nonEmptyInputs[0])
	}

	// Standard merge path for multiple non-empty inputs.
	merged := map[string]any{}

	for index := range nonEmptyInputs {
		current := nonEmptyInputs[index]

		// Due to a bug in `mergo.Merge`
		// (Note: in the `for` loop, it DOES modify the source of the previous loop iteration if it's a complex map and `mergo` gets a pointer to it,
		// not only the destination of the current loop iteration),
		// we don't give it our maps directly; we deep copy them using copystructure (faster than YAML serialization),
		// so `mergo` does not have access to the original pointers.
		// Deep copy preserves types and is sufficient because the data is already in Go map format with custom tags already processed.
		dataCurrent, err := deepCopyMap(current)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to deep copy map: %v", errUtils.ErrMerge, err)
		}

		var opts []func(*mergo.Config)
		opts = append(opts, mergo.WithOverride, mergo.WithTypeCheck)

		// This was fixed/broken in https://github.com/imdario/mergo/pull/231/files
		// It was released in https://github.com/imdario/mergo/releases/tag/v0.3.14
		// It was not working before in `github.com/imdario/mergo` so we need to disable it in our code
		// opts = append(opts, mergo.WithOverwriteWithEmptyValue)

		if sliceDeepCopy {
			opts = append(opts, mergo.WithSliceDeepCopy)
		} else if appendSlice {
			opts = append(opts, mergo.WithAppendSlice)
		}

		if err := mergo.Merge(&merged, dataCurrent, opts...); err != nil {
			// Return the error without debug logging.
			return nil, fmt.Errorf("%w: mergo merge failed: %v", errUtils.ErrMerge, err)
		}
	}

	return merged, nil
}

// Merge takes a list of maps as input, deep-merges the items in the order they are defined in the list, and returns a single map with the merged contents.
func Merge(
	atmosConfig *schema.AtmosConfiguration,
	inputs []map[string]any,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "merge.Merge")()

	// Check for nil config to prevent panic.
	if atmosConfig == nil {
		return nil, errors.Join(errUtils.ErrMerge, errUtils.ErrAtmosConfigIsNil)
	}

	// Default to replace strategy if strategy is empty
	strategy := ListMergeStrategyReplace
	if atmosConfig.Settings.ListMergeStrategy != "" {
		strategy = atmosConfig.Settings.ListMergeStrategy
	}

	if strategy != ListMergeStrategyReplace &&
		strategy != ListMergeStrategyAppend &&
		strategy != ListMergeStrategyMerge {
		err := fmt.Errorf("%w: '%s'. Supported list merge strategies are: %s",
			errUtils.ErrInvalidListMergeStrategy,
			strategy,
			fmt.Sprintf("%s, %s, %s", ListMergeStrategyReplace, ListMergeStrategyAppend, ListMergeStrategyMerge))
		return nil, errors.Join(errUtils.ErrMerge, err)
	}

	sliceDeepCopy := false
	appendSlice := false

	switch strategy {
	case ListMergeStrategyMerge:
		sliceDeepCopy = true
	case ListMergeStrategyAppend:
		appendSlice = true
	}

	return MergeWithOptions(atmosConfig, inputs, appendSlice, sliceDeepCopy)
}

// MergeWithContext performs a merge operation with file context tracking for better error messages.
func MergeWithContext(
	atmosConfig *schema.AtmosConfiguration,
	inputs []map[string]any,
	context *MergeContext,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "merge.MergeWithContext")()

	// Check for nil config to prevent panic
	if atmosConfig == nil {
		err := fmt.Errorf("%w: %s", errUtils.ErrMerge, errUtils.ErrAtmosConfigIsNil)
		if context != nil {
			return nil, context.FormatError(err)
		}
		return nil, err
	}

	// Default to replace strategy if strategy is empty
	strategy := ListMergeStrategyReplace
	if atmosConfig.Settings.ListMergeStrategy != "" {
		strategy = atmosConfig.Settings.ListMergeStrategy
	}

	if strategy != ListMergeStrategyReplace &&
		strategy != ListMergeStrategyAppend &&
		strategy != ListMergeStrategyMerge {
		err := fmt.Errorf("%w: %s: '%s'. Supported list merge strategies are: %s",
			errUtils.ErrMerge,
			errUtils.ErrInvalidListMergeStrategy,
			strategy,
			fmt.Sprintf("%s, %s, %s", ListMergeStrategyReplace, ListMergeStrategyAppend, ListMergeStrategyMerge))
		if context != nil {
			return nil, context.FormatError(err)
		}
		return nil, err
	}

	sliceDeepCopy := false
	appendSlice := false

	switch strategy {
	case ListMergeStrategyMerge:
		sliceDeepCopy = true
	case ListMergeStrategyAppend:
		appendSlice = true
	}

	return MergeWithOptionsAndContext(atmosConfig, inputs, appendSlice, sliceDeepCopy, context)
}

// MergeWithOptionsAndContext performs merge with options and context tracking.
func MergeWithOptionsAndContext(
	atmosConfig *schema.AtmosConfiguration,
	inputs []map[string]any,
	appendSlice bool,
	sliceDeepCopy bool,
	context *MergeContext,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "merge.MergeWithOptionsAndContext")()

	// Fast-path: empty inputs.
	if len(inputs) == 0 {
		return map[string]any{}, nil
	}

	// Fast-path: filter out empty maps and check for trivial cases.
	nonEmptyInputs := make([]map[string]any, 0, len(inputs))
	for _, input := range inputs {
		if len(input) > 0 {
			nonEmptyInputs = append(nonEmptyInputs, input)
		}
	}

	// Fast-path: all inputs were empty.
	if len(nonEmptyInputs) == 0 {
		return map[string]any{}, nil
	}

	// Check if provenance tracking is enabled.
	provenanceEnabled := atmosConfig != nil && atmosConfig.TrackProvenance &&
		context != nil && context.IsProvenanceEnabled() &&
		context.Positions != nil && len(context.Positions) > 0

	// Fast-path: only one non-empty input, return a deep copy to maintain immutability.
	// Skip this fast-path when provenance tracking is enabled to ensure position tracking.
	if len(nonEmptyInputs) == 1 && !provenanceEnabled {
		result, err := deepCopyMap(nonEmptyInputs[0])
		if err != nil && context != nil {
			return nil, context.FormatError(err)
		}
		return result, err
	}

	// Standard merge path for multiple non-empty inputs (or single input with provenance).
	var result map[string]any
	var err error

	// Use MergeWithProvenance when provenance tracking is enabled and positions are available.
	if provenanceEnabled {
		// Perform provenance-aware merge.
		result, err = MergeWithProvenance(atmosConfig, nonEmptyInputs, context, context.Positions)
	} else {
		// Standard merge without provenance.
		result, err = MergeWithOptions(atmosConfig, nonEmptyInputs, appendSlice, sliceDeepCopy)
	}

	if err != nil {
		// Remove verbose merge failure logging
		// The error context will be shown in the formatted error message

		// Add context information to the error
		if context != nil {
			return nil, context.FormatError(err)
		}
		return nil, err
	}

	return result, nil
}
