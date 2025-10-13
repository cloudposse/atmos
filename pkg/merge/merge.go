package merge

import (
	"errors"
	"fmt"
	"reflect"

	"dario.cat/mergo"
	"github.com/mitchellh/copystructure"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	ListMergeStrategyReplace = "replace"
	ListMergeStrategyAppend  = "append"
	ListMergeStrategyMerge   = "merge"
)

// deepCopyMap performs a deep copy of a map using mitchellh/copystructure library,
// then normalizes typed slices to []any for mergo compatibility.
// This preserves numeric types (unlike JSON which converts all numbers to float64) and is faster than
// YAML serialization while avoiding processCustomTags. The data is already in Go map format
// with custom tags already processed, so we only need structural copying to work around
// mergo's pointer mutation bug.
func deepCopyMap(m map[string]any) (map[string]any, error) {
	defer perf.Track(nil, "merge.deepCopyMap")()

	if m == nil {
		return nil, nil
	}

	// Use mitchellh/copystructure for reliable deep copying.
	copied, err := copystructure.Copy(m)
	if err != nil {
		return nil, err
	}

	// Type assertion to map[string]any.
	result, ok := copied.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: %T", errUtils.ErrDeepCopyUnexpectedType, copied)
	}

	// Normalize typed slices/maps to []any/map[string]any for mergo compatibility.
	normalizeTypes(result)

	return result, nil
}

// normalizeTypes walks a map and normalizes typed slices to []any and typed maps to map[string]any.
// This is needed because mergo expects []any and map[string]any, but copystructure preserves exact types.
func normalizeTypes(m map[string]any) {
	for key, value := range m {
		m[key] = normalizeValue(value)
	}
}

// normalizeValue normalizes a value by converting typed slices/maps to []any/map[string]any.
func normalizeValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		// Recursively normalize nested maps.
		normalizeTypes(v)
		return v
	case []any:
		// Recursively normalize slice elements.
		for i, item := range v {
			v[i] = normalizeValue(item)
		}
		return v
	default:
		// Handle other types using reflection.
		return normalizeValueReflect(value)
	}
}

// normalizeValueReflect uses reflection to normalize typed slices and maps.
func normalizeValueReflect(value any) any {
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Slice:
		// Convert any typed slice to []any.
		result := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			result[i] = normalizeValue(rv.Index(i).Interface())
		}
		return result
	case reflect.Map:
		// Convert any typed map with string keys to map[string]any.
		if rv.Len() == 0 {
			return make(map[string]any)
		}

		// Check if keys are strings.
		iter := rv.MapRange()
		if !iter.Next() {
			return make(map[string]any)
		}
		if iter.Key().Kind() == reflect.String {
			result := make(map[string]any, rv.Len())
			// Process first key-value pair.
			result[iter.Key().String()] = normalizeValue(iter.Value().Interface())
			// Process remaining pairs.
			for iter.Next() {
				result[iter.Key().String()] = normalizeValue(iter.Value().Interface())
			}
			return result
		}
		// Non-string keys - return as-is.
		return value
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

	merged := map[string]any{}

	for index := range inputs {
		current := inputs[index]

		if len(current) == 0 {
			continue
		}

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

	// Remove verbose merge operation logging - it creates too much noise
	// Users can use ATMOS_LOGS_LEVEL=Trace if they need detailed merge debugging

	var result map[string]any
	var err error

	// Use MergeWithProvenance when provenance tracking is enabled and positions are available
	if atmosConfig != nil && atmosConfig.TrackProvenance &&
		context != nil && context.IsProvenanceEnabled() &&
		context.Positions != nil && len(context.Positions) > 0 {
		// Perform provenance-aware merge
		result, err = MergeWithProvenance(atmosConfig, inputs, context, context.Positions)
	} else {
		// Standard merge without provenance
		result, err = MergeWithOptions(atmosConfig, inputs, appendSlice, sliceDeepCopy)
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
