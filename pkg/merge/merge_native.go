package merge

import (
	"fmt"
)

// safeCap returns a capacity hint of a+b, clamped to maxCapHint to prevent
// OOM panics from oversize make() calls.
// The hint is only used for make() capacity; append() will grow the backing array as needed.
const maxCapHint = 1 << 24 // 16 M entries — realistic upper bound for atmos configs

func safeCap(a, b int) int {
	// Guard against integer overflow in the sum itself.
	if b > maxCapHint-a {
		return maxCapHint
	}
	sum := a + b
	if sum > maxCapHint {
		return maxCapHint
	}
	return sum
}

// deepMergeNative performs a deep merge of src into dst in place.
//
// It is semantically equivalent to mergo.Merge(dst, src, mergo.WithOverride, mergo.WithTypeCheck)
// but avoids reflection for the hot-path map[string]any/[]any types and does not require
// pre-copying the entire src map.
// Values from src are only copied when they are stored as leaves in dst, preventing
// corruption of the caller's src map through shared pointers in dst.
//
// Behaviour summary (matches observed mergo behaviour):
//   - Both values are map[string]any: recurse, merge in place (no container allocation).
//   - Typed maps (e.g., map[string]schema.Provider): normalized to map[string]any via deepCopyValue and recursed.
//   - appendSlice=true and both are slices: append src elements to dst.
//   - sliceDeepCopy=true and both are slices: element-wise deep-merge.
//   - dst is []any but src is not a slice: return type mismatch error (WithTypeCheck).
//   - Otherwise: src value overrides dst value (deep-copied to isolate src from dst).
func deepMergeNative(dst, src map[string]any, appendSlice, sliceDeepCopy bool) error {
	if dst == nil {
		return fmt.Errorf("deepMergeNative: dst must not be nil")
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
					return err
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
					return err
				}
				continue
			}
			// Type mismatch (map vs non-map): src overrides dst.
			dst[k] = normalizedSrcMap
			continue
		}

		// Slice strategies when both sides are slices.
		// sliceDeepCopy takes precedence over appendSlice, matching the old mergo behaviour where
		// WithSliceDeepCopy was checked before WithAppendSlice.
		if sliceDeepCopy || appendSlice {
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

		// Type check (matches mergo.WithTypeCheck): if dst holds a slice but src is
		// not a slice, refuse the override to prevent silent data corruption.
		if _, dstIsSlice := dstVal.([]any); dstIsSlice {
			if _, srcIsSlice := srcVal.([]any); !srcIsSlice {
				// Attempt normalization once: maybe srcVal is a typed slice (e.g. []string).
				normalized := deepCopyValue(srcVal)
				if _, normalizedIsSlice := normalized.([]any); !normalizedIsSlice {
					return fmt.Errorf("cannot override two slices with different type")
				}
				// Normalized typed slice → use the result we already computed.
				dst[k] = normalized
				continue
			}
		}

		// Default: src overrides dst (deep copy to isolate src).
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

// mergeSlicesNative performs an element-wise deep merge of src into dst, matching
// the behaviour of mergo.WithSliceDeepCopy + WithOverride + WithTypeCheck:
//   - The result length equals the dst length (extra src elements are ignored).
//   - For each position i that exists in both dst and src: if both elements are
//     map[string]any they are deep-merged (propagating appendSlice/sliceDeepCopy);
//     otherwise the dst element is deep-copied (kept).
//   - Positions that exist only in dst are deep-copied (preserved as-is).
func mergeSlicesNative(dst, src []any, appendSlice, sliceDeepCopy bool) ([]any, error) {
	result := make([]any, len(dst))
	// Do NOT shallow-copy dst into result here; every position is overwritten by
	// the two loops below, so the copy() call would only create transient shallow
	// aliases that are immediately replaced — latent aliasing risk with no benefit.

	// Merge src elements into the result up to the length of dst.
	for i := 0; i < len(src) && i < len(dst); i++ {
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
		// Use combined length as capacity hint to avoid reallocations when src adds new keys.
		// Deep-copy dstMap values so that deepMergeNative cannot mutate shared inner maps
		// (which would corrupt the accumulator in multi-input merges).
		merged := make(map[string]any, safeCap(len(dstMap), len(srcMap)))
		for k, v := range dstMap {
			merged[k] = deepCopyValue(v)
		}
		if err := deepMergeNative(merged, srcMap, appendSlice, sliceDeepCopy); err != nil {
			return nil, err
		}
		result[i] = merged
	}

	// Deep-copy tail elements (positions beyond src length) to fully isolate the result
	// from the accumulator.  Without this, result[i] and the accumulator's slice element
	// alias the same map, so a later merge pass could mutate data visible to callers.
	for i := len(src); i < len(dst); i++ {
		result[i] = deepCopyValue(dst[i])
	}

	return result, nil
}
