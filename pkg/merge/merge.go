package merge

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	ListMergeStrategyReplace = "replace"
	ListMergeStrategyAppend  = "append"
	ListMergeStrategyMerge   = "merge"

	// MaxSliceCapacity is the maximum safe capacity for slice allocation to prevent overflow.
	// This is 2^30 (about 1 billion elements), providing a safe margin below the int max.
	maxSliceCapacity = 1 << 30
)

// ThreeWayMerger handles 3-way merging of text files.
type ThreeWayMerger struct {
	maxChanges int
}

// NewThreeWayMerger creates a new 3-way merger with the specified max changes threshold.
func NewThreeWayMerger(maxChanges int) *ThreeWayMerger {
	return &ThreeWayMerger{
		maxChanges: maxChanges,
	}
}

// Merge performs a 3-way merge between existing and new content.
func (m *ThreeWayMerger) Merge(existingContent, newContent, fileName string) (string, error) {
	// Use diffmatchpatch to compute the diff between existing and new content.
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(existingContent, newContent, true)

	// Check if the diff is too complex (too many changes).
	changeCount := 0
	for _, diff := range diffs {
		if diff.Type != diffmatchpatch.DiffEqual {
			changeCount++
		}
	}

	// If there are too many changes, refuse to merge.
	if changeCount > m.maxChanges {
		return "", errUtils.Build(errUtils.ErrMergeThresholdExceeded).
			WithExplanationf("Too many changes detected (%d changes)", changeCount).
			WithHint("Use --force to overwrite or manually merge").
			Err()
	}

	// Apply the diff as patches to existingContent so that only the specific
	// changes from newContent are incorporated. PatchApply attempts each patch
	// hunk independently: if a hunk cannot match the surrounding context it is
	// skipped and the original text is kept for that region.  This preserves
	// any local edits that do not conflict with the incoming changes, unlike
	// DiffText2 which unconditionally returns newContent.
	patches := dmp.PatchMake(existingContent, diffs)
	mergedContentStr, _ := dmp.PatchApply(patches, existingContent)
	mergedContent := mergedContentStr

	// Check for conflicts by looking for diff markers.
	if strings.Contains(mergedContent, "<<<<<<<") || strings.Contains(mergedContent, "=======") || strings.Contains(mergedContent, ">>>>>>>") {
		// There are conflicts, let's handle them intelligently.
		mergedContent = m.resolveConflicts(mergedContent, fileName)
	}

	return mergedContent, nil
}

// resolveConflicts handles merge conflicts by preserving user customizations.
func (m *ThreeWayMerger) resolveConflicts(content, fileName string) string {
	lines := strings.Split(content, "\n")
	var resolvedLines []string
	var inConflict bool
	var conflictBuffer []string

	for _, line := range lines {
		if strings.HasPrefix(line, "<<<<<<<") {
			inConflict = true
			conflictBuffer = []string{}
			continue
		}

		if strings.HasPrefix(line, "=======") {
			// Middle of conflict - separator between ours and theirs sides.
			// Keep conflictBuffer intact so the full conflict block (both sides)
			// is passed to resolveConflictBlock for intelligent resolution.
			continue
		}

		if strings.HasPrefix(line, ">>>>>>>") {
			inConflict = false
			// Resolve the conflict by preferring existing content (user customizations).
			resolvedLines = append(resolvedLines, m.resolveConflictBlock(conflictBuffer, fileName)...)
			continue
		}

		if inConflict {
			conflictBuffer = append(conflictBuffer, line)
		} else {
			resolvedLines = append(resolvedLines, line)
		}
	}

	return strings.Join(resolvedLines, "\n")
}

// resolveConflictBlock resolves a single conflict block.
func (m *ThreeWayMerger) resolveConflictBlock(conflictLines []string, fileName string) []string {
	var resolved []string

	// Add conflict resolution marker.
	resolved = append(resolved, fmt.Sprintf("# CONFLICT RESOLVED for %s", fileName))
	resolved = append(resolved, "# Preserving user customizations and adding new template content")
	resolved = append(resolved, "")

	// For now, preserve all lines from the conflict.
	// In a more sophisticated implementation, you'd analyze the content
	// and make intelligent decisions about what to keep.
	for _, line := range conflictLines {
		if strings.TrimSpace(line) != "" {
			resolved = append(resolved, line)
		}
	}

	resolved = append(resolved, "")
	return resolved
}

// mapKeyCollisionPanic carries a deliberate, expected error through Go's panic/recover
// mechanism, raised by normalizeMapReflect when two distinct original keys stringify to the
// same string but cannot be safely merged (see normalizeMapReflect for details). The
// deepCopyValue function has ~20 call sites across this package's hot merge path, so threading
// an error return through all of them would add a return-value check to every recursive step;
// recover-and-convert at this package's two independent entry points (DeepCopyMap,
// deepMergeNativeTopLevel) achieves the same "abort the whole merge on ambiguous data" outcome
// without that cost. No value of this type is ever allowed to escape the merge package — see
// recoveredMapKeyCollision.
type mapKeyCollisionPanic struct {
	err error
}

// recoveredMapKeyCollision inspects a recover()'d panic value and, if it is a
// mapKeyCollisionPanic raised by normalizeMapReflect, returns its wrapped error. Returns nil
// for a nil input (no panic occurred). Any other panic value is re-panicked immediately: this
// must never mask a genuine bug (e.g. a nil-map SetMapIndex panic) as a merge-collision error,
// and genuine bugs should still reach Atmos's global panic handler (pkg/panics) with their real
// value and stack trace.
func recoveredMapKeyCollision(r any) error {
	if r == nil {
		return nil
	}
	collision, ok := r.(mapKeyCollisionPanic)
	if !ok {
		panic(r)
	}
	return collision.err
}

// DeepCopyMap performs a deep copy of a map optimized for map[string]any structures.
// This custom implementation avoids reflection overhead for common cases (maps, slices, primitives)
// and uses reflection-based normalization for rare complex types (typed slices/maps).
// Preserves numeric types (unlike JSON which converts all numbers to float64) and is faster than
// generic reflection-based copying. The data is already in Go map format with custom tags already processed,
// so structural copying is needed to ensure accumulated merge results are independent of their inputs.
// Uses properly-sized allocations to reduce GC pressure during high-volume operations (118k+ calls per run).
func DeepCopyMap(m map[string]any) (result map[string]any, err error) {
	defer perf.Track(nil, "merge.DeepCopyMap")()
	defer func() {
		if e := recoveredMapKeyCollision(recover()); e != nil {
			result = nil
			err = e
		}
	}()

	if m == nil {
		return nil, nil
	}

	// Allocate map with exact size to avoid resizing.
	result = make(map[string]any, len(m))

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

// normalizeMapReflect converts a typed map to map[string]any (for string or interface{} keys)
// or deep copies it (for other concrete non-string keys).
func normalizeMapReflect(rv reflect.Value) any {
	keyKind := rv.Type().Key().Kind()

	// Empty map - return properly typed empty map.
	if rv.Len() == 0 {
		if keyKind != reflect.String && keyKind != reflect.Interface {
			return reflect.MakeMapWithSize(rv.Type(), 0).Interface()
		}
		return make(map[string]any, 0)
	}

	iter := rv.MapRange()

	// Concrete non-string, non-interface keys (e.g. map[int]schema.Provider): copy to the same
	// type, preserving the key type — there is no map[string]any shape to merge into.
	if keyKind != reflect.String && keyKind != reflect.Interface {
		return copyNonStringKeyMap(rv, iter)
	}

	// String keys, or interface{} keys (e.g. yaml.v3 decodes a mapping with an unquoted
	// non-string key like `1:` as map[interface{}]interface{}, not map[string]interface{}):
	// stringify every key so this collapses onto the same map[string]any shape as an
	// all-string-keyed sibling map, letting deepMergeNative's map[string]any fast path recurse
	// into it instead of treating it as an opaque leaf that gets replaced wholesale.
	//
	// Distinct original keys can still stringify to the same string (e.g. YAML `1` and `1.0`,
	// or `true` and a differently-quoted equivalent) — this is the same collision risk any
	// YAML-to-map[string]any normalization has. Map[interface{}]interface{} iteration order is
	// unspecified, so a plain overwrite would silently and non-deterministically drop one
	// entry's data. If both colliding values are maps, merge them instead of dropping either.
	// Otherwise the collision is genuinely ambiguous — there is no safe way to combine two
	// scalars (or a scalar and a map) without picking one arbitrarily — so abort the merge via
	// mapKeyCollisionPanic rather than silently dropping data; DeepCopyMap and
	// deepMergeNativeTopLevel recover it into a normal returned error.
	result := make(map[string]any, rv.Len())
	for iter.Next() {
		key := iter.Key()
		var keyStr string
		if key.Kind() == reflect.String {
			keyStr = key.String()
		} else {
			keyStr = fmt.Sprintf("%v", key.Interface())
		}
		normalizedVal := deepCopyValue(iter.Value().Interface())
		if existing, collided := result[keyStr]; collided {
			existingMap, existingIsMap := existing.(map[string]any)
			newMap, newIsMap := normalizedVal.(map[string]any)
			if existingIsMap && newIsMap {
				_ = deepMergeNative(existingMap, newMap, false, false)
				continue
			}
			panic(mapKeyCollisionPanic{
				err: errUtils.Build(errUtils.ErrMergeKeyCollision).
					WithExplanationf("multiple keys normalize to %q but are not both maps, so they cannot be merged safely", keyStr).
					WithHint("quote ambiguous scalar keys (e.g. \"1\" instead of 1, or \"1.0\" instead of 1.0) so they don't collide after normalization").
					WithContext("key", keyStr).
					Err(),
			})
		}
		result[keyStr] = normalizedVal
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
	// Still resolve any !append wrappers against an empty accumulator — a lone !append
	// has nothing to append to, so it becomes a plain list — otherwise the wrapper would
	// leak into the result instead of being resolved.
	if len(nonEmptyInputs) == 1 {
		copied, err := DeepCopyMap(nonEmptyInputs[0])
		if err != nil {
			return nil, fmt.Errorf("%w: failed to deep copy map: %w", errUtils.ErrMerge, err)
		}
		return processAppendTags(copied, map[string]any{}, appendSlice), nil
	}

	// Standard merge path for multiple non-empty inputs.
	//
	// Strategy: deep-copy the first input to create the initial accumulator, then
	// merge each subsequent input into it using deepMergeNative.  deepMergeNative
	// only copies values that are placed as leaves in the accumulator, so it avoids
	// the full pre-copy that the old mergo-based loop required on every iteration.
	// This reduces the number of full DeepCopyMap calls from N to 1 and eliminates
	// reflection overhead from mergo for every subsequent merge.
	merged, err := DeepCopyMap(nonEmptyInputs[0])
	if err != nil {
		return nil, fmt.Errorf("%w: failed to deep copy map: %w", errUtils.ErrMerge, err)
	}

	// Resolve !append-tagged lists in the base input. At the base there is nothing
	// to append to, so an !append list simply becomes a plain list.
	merged = processAppendTags(merged, map[string]any{}, appendSlice)

	for _, current := range nonEmptyInputs[1:] {
		// Resolve !append-tagged lists against the accumulator before merging.
		// With the global append strategy (appendSlice), processAppendTags returns only
		// the new items so deepMergeNative's append adds them without duplication; otherwise
		// it returns existing+new concatenated so the override replaces with the appended list.
		current = processAppendTags(current, merged, appendSlice)
		if err := deepMergeNativeTopLevel(merged, current, appendSlice, sliceDeepCopy); err != nil {
			return nil, fmt.Errorf("%w: %w", errUtils.ErrMerge, err)
		}
	}

	return merged, nil
}

// processAppendTags handles special !append tagged lists during merging.
// It processes any values wrapped with __atmos_append__ metadata and appends them to existing lists.
// When appendNewOnly is true (global append strategy), it returns only the new items so that
// deepMergeNative's append strategy performs the append without duplication.
func processAppendTags(current map[string]any, merged map[string]any, appendNewOnly bool) map[string]any {
	result := make(map[string]any)

	for key, value := range current {
		result[key] = processValue(key, value, merged, appendNewOnly)
	}

	return result
}

// processValue processes a single value for append tags.
func processValue(key string, value any, merged map[string]any, appendNewOnly bool) any {
	// Check if this is an append-tagged list.
	if list, isAppend := u.ExtractAppendListValue(value); isAppend {
		return processAppendList(key, list, merged, appendNewOnly)
	}

	// Check if this is a nested map.
	if nestedMap, ok := value.(map[string]any); ok {
		return processNestedMap(key, nestedMap, merged, appendNewOnly)
	}

	// Regular value, pass through.
	return value
}

// processAppendList handles appending a list to existing values.
// If appendNewOnly is true, return only the new items so deepMergeNative's append strategy
// can append them to the existing list without duplication.
func processAppendList(key string, list []any, merged map[string]any, appendNewOnly bool) []any {
	if appendNewOnly {
		return list
	}

	var existingList []any
	if existingValue, exists := merged[key]; exists {
		if el, ok := existingValue.([]any); ok {
			existingList = el
		}
	}

	// Create a new slice to avoid modifying the original.
	// Overflow guard: ensure existingLen+newLen stays within int range before computing
	// it, so the make() below cannot overflow. Falling back to just the new list is safe.
	existingLen := len(existingList)
	newLen := len(list)
	if newLen > math.MaxInt-existingLen {
		return list
	}
	totalLen := existingLen + newLen
	// Sanity cap: avoid absurdly large allocations even when within int range.
	if totalLen > maxSliceCapacity {
		return list
	}
	result := make([]any, existingLen, totalLen)
	copy(result, existingList)
	result = append(result, list...)
	return result
}

// processNestedMap recursively processes nested maps for append tags.
func processNestedMap(key string, nestedMap map[string]any, merged map[string]any, appendNewOnly bool) map[string]any {
	var mergedNested map[string]any
	if existingNested, exists := merged[key]; exists {
		if mn, ok := existingNested.(map[string]any); ok {
			mergedNested = mn
		}
	}
	if mergedNested == nil {
		mergedNested = make(map[string]any)
	}
	return processAppendTags(nestedMap, mergedNested, appendNewOnly)
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

// isAtmosYAMLFunction checks if a string is an Atmos YAML function that is processed after merging.
// YAML functions processed after merging (need special handling during merge).
// Functions like !include and !include.raw are processed during YAML loading, so they don't need special handling.
