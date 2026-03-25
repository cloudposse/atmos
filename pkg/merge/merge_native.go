package merge

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
)

// safeCap returns a capacity hint of a+b, clamped to maxCapHint to prevent
// OOM panics from oversize make() calls.
// The hint is only used for make() capacity; append() will grow the backing array as needed.
const maxCapHint = 1 << 24 // 16 M entries — realistic upper bound for atmos configs

func safeCap(a, b int) int {
	// Guard against integer overflow: if b > maxCapHint-a, then a+b > maxCapHint.
	// This single check is sufficient; no second guard is needed.
	if b > maxCapHint-a {
		return maxCapHint
	}
	return a + b
}

// deepMergeNative performs a deep merge of src into dst in place.
//
// It is semantically equivalent to mergo.Merge(dst, src, mergo.WithOverride)
// but avoids reflection for the hot-path map[string]any/[]any types and does not require
// pre-copying the entire src map.
// Values from src are only copied when they are stored as leaves in dst, preventing
// corruption of the caller's src map through shared pointers in dst.
//
// Behavior summary (defined contract for native merge):
//   - Both values are map[string]any: recurse, merge in place (no container allocation).
//   - Typed maps (e.g., map[string]schema.Provider): normalized to map[string]any via deepCopyValue and recursed.
//   - appendSlice=true and both are slices: append src elements to dst.
//   - sliceDeepCopy=true and both are slices: element-wise deep-merge.
//   - Otherwise: src value overrides dst value (deep-copied to isolate src from dst).
//     This includes type changes (list→map, list→scalar, list→nil, etc.).
//
// A nil src is safe: ranging over a nil map is a no-op in Go, so no keys are visited.
// Dst must not be nil; the function returns an error if it is.
func deepMergeNative(dst, src map[string]any, appendSlice, sliceDeepCopy bool) error { //nolint:gocognit,revive,cyclop,funlen // Core merge function with unavoidable branching.
	if dst == nil {
		return errUtils.ErrMergeNilDst
	}
	for k, srcVal := range src {
		dstVal, exists := dst[k]
		if !exists {
			// Key only in src: deep copy to prevent dst from aliasing src internals.
			dst[k] = deepCopyValue(srcVal)
			continue
		}

		// Key exists in both dst and src.

		// Fast path: both are maps — recurse without allocating a new container.
		if srcMap, srcIsMap := srcVal.(map[string]any); srcIsMap {
			if dstMap, dstIsMap := dstVal.(map[string]any); dstIsMap {
				if err := deepMergeNative(dstMap, srcMap, appendSlice, sliceDeepCopy); err != nil {
					return fmt.Errorf("merge key %q: %w", k, err)
				}
				continue
			}
			// Type mismatch (map vs non-map): src overrides dst.
			dst[k] = deepCopyValue(srcVal)
			continue
		}

		// Handle typed maps (e.g., map[string]schema.Provider): normalize to map[string]any and recurse.
		// deepCopyValue handles this via reflection-based normalizeValueReflect for non-map[string]any types.
		if normalizedSrcMap, ok := deepCopyValue(srcVal).(map[string]any); ok {
			if dstMap, dstIsMap := dstVal.(map[string]any); dstIsMap {
				if err := deepMergeNative(dstMap, normalizedSrcMap, appendSlice, sliceDeepCopy); err != nil {
					return fmt.Errorf("merge key %q: %w", k, err)
				}
				continue
			}
			// Type mismatch (map vs non-map): src overrides dst.
			dst[k] = normalizedSrcMap
			continue
		}

		// Slice strategies when both sides are slices.
		// sliceDeepCopy takes precedence over appendSlice, matching the old mergo behavior where
		// WithSliceDeepCopy was checked before WithAppendSlice.
		if sliceDeepCopy || appendSlice { //nolint:nestif // Slice strategy dispatch requires nested type checks.
			if dstSlice, dstIsSlice := dstVal.([]any); dstIsSlice {
				if srcSlice, ok := toAnySlice(srcVal); ok {
					if sliceDeepCopy {
						// sliceDeepCopy: element-wise merge (propagate flags for nested slices of maps).
						var err error
						dst[k], err = mergeSlicesNative(dstSlice, srcSlice, appendSlice, sliceDeepCopy)
						if err != nil {
							return err
						}
					} else {
						// appendSlice: append src elements to dst.
						dst[k] = appendSlices(dstSlice, srcSlice)
					}
					continue
				}
			}
		}

		// Default: src overrides dst (deep copy to isolate src).
		// WithOverride semantics: src always wins regardless of type differences.
		// This handles list→map, list→scalar, list→nil, and all other type overrides.
		dst[k] = deepCopyValue(srcVal)
	}
	return nil
}

// toAnySlice tries to return v as []any without allocating when possible.
// Typed slices (e.g. []string) are normalised via deepCopyValue.
func toAnySlice(v any) ([]any, bool) {
	if s, ok := v.([]any); ok {
		return s, true
	}
	// Normalise typed slices ([]string, []int, etc.) to []any.
	if normalised, ok := deepCopyValue(v).([]any); ok {
		return normalised, true
	}
	return nil, false
}

// appendSlices returns a new slice containing deep copies of all elements of dst
// followed by deep copies of all elements of src.
// Both dst and src elements are deep-copied to prevent the result from aliasing
// the accumulator's elements across multiple merge passes.
func appendSlices(dst, src []any) []any {
	result := make([]any, 0, safeCap(len(dst), len(src)))
	for _, v := range dst {
		result = append(result, deepCopyValue(v))
	}
	for _, v := range src {
		result = append(result, deepCopyValue(v))
	}
	return result
}

// mergeSlicesNative performs an element-wise deep merge of src into dst.
//
// Rules:
//   - For each position i that exists in both dst and src: if both elements are
//     map[string]any they are deep-merged (propagating appendSlice/sliceDeepCopy);
//     otherwise the dst element is deep-copied (kept).
//   - Positions that exist only in dst (dst longer than src) are deep-copied (preserved).
//   - Positions that exist only in src (src longer than dst) are deep-copied and appended.
//     This matches mergo's WithSliceDeepCopy behavior, which extends the result slice
//     when src has more elements than dst.
func mergeSlicesNative(dst, src []any, appendSlice, sliceDeepCopy bool) ([]any, error) {
	// Result length is max(len(dst), len(src)) — src can extend the slice.
	resultLen := len(dst)
	if len(src) > resultLen {
		resultLen = len(src)
	}
	result := make([]any, resultLen)

	// Merge overlapping positions (both dst and src have elements).
	overlap := len(dst)
	if len(src) < overlap {
		overlap = len(src)
	}
	for i := 0; i < overlap; i++ {
		srcMap, srcIsMap := src[i].(map[string]any)
		if !srcIsMap {
			// Non-map src element: dst[i] is preserved (mergo keeps dst for scalars).
			// Deep-copy to prevent the result from aliasing the accumulator's element.
			result[i] = deepCopyValue(dst[i])
			continue
		}
		dstMap, dstIsMap := dst[i].(map[string]any)
		if !dstIsMap {
			// Type mismatch: dst element is preserved.
			// Deep-copy to prevent aliasing.
			result[i] = deepCopyValue(dst[i])
			continue
		}
		// Both are maps: deep-merge into a new container so neither src nor dst is aliased.
		merged := make(map[string]any, safeCap(len(dstMap), len(srcMap)))
		for k, v := range dstMap {
			merged[k] = deepCopyValue(v)
		}
		if err := deepMergeNative(merged, srcMap, appendSlice, sliceDeepCopy); err != nil {
			return nil, fmt.Errorf("merge slice index %d: %w", i, err)
		}
		result[i] = merged
	}

	// Deep-copy dst tail elements (positions beyond src length).
	for i := len(src); i < len(dst); i++ {
		result[i] = deepCopyValue(dst[i])
	}

	// Deep-copy src tail elements (positions beyond dst length).
	// This ensures src can extend the result slice — e.g., an overlay stack
	// adding new node_groups beyond what the base stack defines.
	for i := len(dst); i < len(src); i++ {
		result[i] = deepCopyValue(src[i])
	}

	return result, nil
}
