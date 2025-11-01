package merge

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"dario.cat/mergo"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	ListMergeStrategyReplace = "replace"
	ListMergeStrategyAppend  = "append"
	ListMergeStrategyMerge   = "merge"
)

// DeepCopyMap performs a deep copy of a map optimized for map[string]any structures.
// This custom implementation avoids reflection overhead for common cases (maps, slices, primitives)
// and uses reflection-based normalization for rare complex types (typed slices/maps).
// Preserves numeric types (unlike JSON which converts all numbers to float64) and is faster than
// generic reflection-based copying. The data is already in Go map format with custom tags already processed,
// so we only need structural copying to work around mergo's pointer mutation bug.
// Uses properly-sized allocations to reduce GC pressure during high-volume operations (118k+ calls per run).
func DeepCopyMap(m map[string]any) (map[string]any, error) {
	defer perf.Track(nil, "merge.DeepCopyMap")()

	if m == nil {
		return nil, nil
	}

	// Allocate map with exact size to avoid resizing.
	result := make(map[string]any, len(m))

	// Copy all key-value pairs.
	for k, v := range m {
		result[k] = deepCopyValue(v)
	}

	return result, nil
}

// deepCopyValue performs a deep copy of a value, handling common types without reflection.
// Uses reflection-based normalization for rare complex types (typed slices/maps).
// Allocates maps and slices with proper sizing to reduce allocations.
func deepCopyValue(v any) any {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case map[string]any:
		// Common case: nested map - allocate with exact size and recurse.
		result := make(map[string]any, len(val))

		// Copy all key-value pairs.
		for k, v := range val {
			result[k] = deepCopyValue(v)
		}
		return result

	case []any:
		// Common case: slice - allocate with exact size and recurse.
		result := make([]any, len(val))

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

// deepCopyTypedValue performs a deep copy of a typed value using reflection.
// This handles slices and maps with proper type preservation for non-interface element types.
//
//nolint:revive,funlen // Complexity and length are inherent to reflection-based type handling.
func deepCopyTypedValue(rv reflect.Value) reflect.Value {
	switch rv.Kind() {
	case reflect.Struct:
		// Deep copy exported fields of struct values.
		// This prevents aliasing of nested slices/maps inside struct values in typed maps.
		t := rv.Type()
		dst := reflect.New(t).Elem()
		// Preserve unexported fields via shallow copy first.
		dst.Set(rv)
		// Now deep-copy exported reference fields to avoid aliasing.
		for i := 0; i < rv.NumField(); i++ {
			f := dst.Field(i)
			if !f.CanSet() {
				continue
			}
			f.Set(deepCopyTypedValue(rv.Field(i)))
		}
		return dst

	case reflect.Slice:
		// Deep copy typed slice.
		if rv.IsNil() {
			return rv
		}
		sliceLen := rv.Len()
		newSlice := reflect.MakeSlice(rv.Type(), sliceLen, sliceLen)
		for i := 0; i < sliceLen; i++ {
			elem := deepCopyTypedValue(rv.Index(i))
			newSlice.Index(i).Set(elem)
		}
		return newSlice

	case reflect.Map:
		// Deep copy typed map.
		if rv.IsNil() {
			return rv
		}
		newMap := reflect.MakeMapWithSize(rv.Type(), rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			key := iter.Key()
			val := deepCopyTypedValue(iter.Value())
			newMap.SetMapIndex(key, val)
		}
		return newMap

	case reflect.Ptr:
		// For pointers, don't deep copy through them - aliasing pointers is usually intentional.
		return rv

	case reflect.Interface:
		// Unwrap interface and deep copy the underlying value.
		if rv.IsNil() {
			return rv
		}
		elem := rv.Elem()
		copiedElem := deepCopyTypedValue(elem)
		newVal := reflect.New(rv.Type()).Elem()
		newVal.Set(copiedElem)
		return newVal

	default:
		// Primitives, strings, functions, channels - return as-is.
		return rv
	}
}

// normalizeSliceReflect converts a typed slice to []any using reflection.
func normalizeSliceReflect(rv reflect.Value) []any {
	sliceLen := rv.Len()
	result := make([]any, sliceLen)

	for i := 0; i < sliceLen; i++ {
		result[i] = deepCopyValue(rv.Index(i).Interface())
	}
	return result
}

// copyNonStringKeyMap deep copies a map with non-string keys, preserving the type.
func copyNonStringKeyMap(rv reflect.Value, iter *reflect.MapIter) any {
	dstType := rv.Type()
	elemType := dstType.Elem()
	copyMap := reflect.MakeMapWithSize(dstType, rv.Len())

	for iter.Next() {
		val := copyMapValue(iter.Value(), elemType)
		copyMap.SetMapIndex(iter.Key(), val)
	}
	return copyMap.Interface()
}

// copyMapValue deep copies a map value, handling both interface and typed elements.
func copyMapValue(value reflect.Value, elemType reflect.Type) reflect.Value {
	// Prefer a typed deep copy. If not assignable to Elem(), fall back to original typed value
	// (handles non-empty interface element types safely).
	val := deepCopyTypedValue(value)
	if val.Type().AssignableTo(elemType) {
		return val
	}
	// If original is assignable, keep it to avoid SetMapIndex panic.
	if value.Type().AssignableTo(elemType) {
		return value
	}
	// As a last resort, if Elem() is empty interface, any deep-copied shape is fine.
	if elemType.Kind() == reflect.Interface && elemType.NumMethod() == 0 {
		return reflect.ValueOf(deepCopyValue(value.Interface()))
	}
	// Otherwise, retain the original typed value.
	return value
}

// normalizeMapReflect converts a typed map to map[string]any (for string keys) or deep copies it (for non-string keys).
func normalizeMapReflect(rv reflect.Value) any {
	keyKind := rv.Type().Key().Kind()

	// Empty map - return properly typed empty map.
	if rv.Len() == 0 {
		if keyKind != reflect.String {
			return reflect.MakeMapWithSize(rv.Type(), 0).Interface()
		}
		return make(map[string]any, 0)
	}

	iter := rv.MapRange()
	_ = iter // We'll iterate below using Next().

	// Non-string keys: copy to same type, ensuring value type matches Elem().
	if keyKind != reflect.String {
		return copyNonStringKeyMap(rv, iter)
	}

	// String keys: convert to map[string]any.
	result := make(map[string]any, rv.Len())
	for iter.Next() {
		result[iter.Key().String()] = deepCopyValue(iter.Value().Interface())
	}
	return result
}

// normalizeValueReflect uses reflection to normalize typed slices and maps.
// This is used as a fallback for complex types that aren't handled by the fast path.
// Allocates maps and slices with proper sizing.
func normalizeValueReflect(value any) any {
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Slice:
		return normalizeSliceReflect(rv)
	case reflect.Map:
		return normalizeMapReflect(rv)
	case reflect.Struct:
		return structToMapReflect(rv)
	case reflect.Ptr:
		if rv.IsNil() {
			return nil
		}
		return normalizeValueReflect(rv.Elem().Interface())
	default:
		// Primitives and other types - return as-is.
		return value
	}
}

// structToMapReflect converts a struct to map[string]any using reflection.
// Preserves numeric types (unlike JSON marshaling which converts all numbers to float64).
// Uses mapstructure tags if available, otherwise uses field names.
//
//nolint:revive // Cyclomatic complexity is inherent to reflection-based struct-to-map conversion with tag handling.
func structToMapReflect(rv reflect.Value) map[string]any {
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}

	if rv.Kind() != reflect.Struct {
		return nil
	}

	result := make(map[string]any)
	t := rv.Type()

	for i := 0; i < rv.NumField(); i++ {
		field := t.Field(i)
		value := rv.Field(i)

		// Skip unexported fields.
		if !field.IsExported() {
			continue
		}

		// Get field name from mapstructure tag, fallback to JSON tag, then field name.
		mapTag := field.Tag.Get("mapstructure")
		if mapTag == "-" {
			continue
		}

		fieldName := mapTag
		if fieldName == "" {
			fieldName = field.Tag.Get("json")
		}
		if fieldName == "" || fieldName == "-" {
			fieldName = field.Name
		}

		// Remove omitempty and other tag options.
		if idx := strings.Index(fieldName, ","); idx != -1 {
			fieldName = fieldName[:idx]
		}

		// Skip fields with "-" tag.
		if fieldName == "-" {
			continue
		}

		// Recursively convert the value, preserving types.
		result[fieldName] = deepCopyValue(value.Interface())
	}

	return result
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
		return DeepCopyMap(nonEmptyInputs[0])
	}

	// Standard merge path for multiple non-empty inputs.
	merged := map[string]any{}

	for index := range nonEmptyInputs {
		current := nonEmptyInputs[index]

		// Process !append tagged lists before merging
		current = processAppendTags(current, merged)

		// Due to a bug in `mergo.Merge`
		// (Note: in the `for` loop, it DOES modify the source of the previous loop iteration if it's a complex map and `mergo` gets a pointer to it,
		// not only the destination of the current loop iteration),
		// we don't give it our maps directly; we deep copy them using our custom DeepCopyMap (faster than YAML serialization),
		// so `mergo` does not have access to the original pointers.
		// Deep copy preserves types and is sufficient because the data is already in Go map format with custom tags already processed.
		dataCurrent, err := DeepCopyMap(current)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to deep copy map: %w", errUtils.ErrMerge, err)
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
			return nil, fmt.Errorf("%w: mergo merge failed: %w", errUtils.ErrMerge, err)
		}
	}

	return merged, nil
}

// processAppendTags handles special !append tagged lists during merging.
// It processes any values wrapped with __atmos_append__ metadata and appends them to existing lists.
func processAppendTags(current map[string]any, merged map[string]any) map[string]any {
	result := make(map[string]any)

	for key, value := range current {
		result[key] = processValue(key, value, merged)
	}

	return result
}

// processValue processes a single value for append tags.
func processValue(key string, value any, merged map[string]any) any {
	// Check if this is an append-tagged list
	if list, isAppend := u.ExtractAppendListValue(value); isAppend {
		return processAppendList(key, list, merged)
	}

	// Check if this is a nested map
	if nestedMap, ok := value.(map[string]any); ok {
		return processNestedMap(key, nestedMap, merged)
	}

	// Regular value, pass through
	return value
}

// processAppendList handles appending a list to existing values.
func processAppendList(key string, list []any, merged map[string]any) []any {
	var existingList []any
	if existingValue, exists := merged[key]; exists {
		if el, ok := existingValue.([]any); ok {
			existingList = el
		}
	}

	// Create a new slice to avoid modifying the original
	result := make([]any, len(existingList), len(existingList)+len(list))
	copy(result, existingList)
	result = append(result, list...)
	return result
}

// processNestedMap recursively processes nested maps for append tags.
func processNestedMap(key string, nestedMap map[string]any, merged map[string]any) map[string]any {
	var mergedNested map[string]any
	if existingNested, exists := merged[key]; exists {
		if mn, ok := existingNested.(map[string]any); ok {
			mergedNested = mn
		}
	}
	if mergedNested == nil {
		mergedNested = make(map[string]any)
	}
	return processAppendTags(nestedMap, mergedNested)
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

	// Default to replace strategy if strategy is empty.
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

	// Check for nil config to prevent panic.
	if atmosConfig == nil {
		err := fmt.Errorf("%w: %s", errUtils.ErrMerge, errUtils.ErrAtmosConfigIsNil)
		if context != nil {
			return nil, context.FormatError(err)
		}
		return nil, err
	}

	// Default to replace strategy if strategy is empty.
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
		result, err := DeepCopyMap(nonEmptyInputs[0])
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
		// Remove verbose merge failure logging.
		// The error context will be shown in the formatted error message.

		// Add context information to the error.
		if context != nil {
			return nil, context.FormatError(err)
		}
		return nil, err
	}

	return result, nil
}
