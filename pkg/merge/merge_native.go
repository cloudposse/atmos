package merge

import "math"

// safeAdd returns a+b, clamped to math.MaxInt on overflow.
// This prevents integer overflow in size computations passed to make().
func safeAdd(a, b int) int {
	if b > math.MaxInt-a {
		return math.MaxInt
	}
	return a + b
}

// deepMergeNative performs a deep merge of src into dst in place.
//
// It is semantically equivalent to mergo.Merge(dst, src, mergo.WithOverride, mergo.WithTypeCheck)
// but uses no reflection and does not require pre-copying the entire src map.
// Values from src are only copied when they are stored as leaves in dst, preventing
// corruption of the caller's src map through shared pointers in dst.
//
// Behaviour summary (matches observed mergo behaviour):
//   - Both values are map[string]any: recurse, merge in place (no container allocation).
//   - appendSlice=true and both are slices: append src elements to dst.
//   - sliceDeepCopy=true and both are slices: element-wise deep-merge.
//   - Otherwise: src value overrides dst value (deep-copied to isolate src from dst).
func deepMergeNative(dst, src map[string]any, appendSlice, sliceDeepCopy bool) {
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
				deepMergeNative(dstMap, srcMap, appendSlice, sliceDeepCopy)
				continue
			}
			// Type mismatch (map vs non-map): src overrides dst.
			dst[k] = deepCopyValue(srcVal)
			continue
		}

		// Slice strategies when both sides are slices.
		if appendSlice || sliceDeepCopy {
			if dstSlice, dstIsSlice := dstVal.([]any); dstIsSlice {
				if srcSlice, ok := toAnySlice(srcVal); ok {
					if appendSlice {
						dst[k] = appendSlices(dstSlice, srcSlice)
					} else {
						// sliceDeepCopy: element-wise merge.
						dst[k] = mergeSlicesNative(dstSlice, srcSlice)
					}
					continue
				}
			}
		}

		// Default: src overrides dst (deep copy to isolate src).
		dst[k] = deepCopyValue(srcVal)
	}
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

// appendSlices returns a new slice containing all elements of dst followed by
// deep copies of all elements of src.
func appendSlices(dst, src []any) []any {
	result := make([]any, len(dst), safeAdd(len(dst), len(src)))
	copy(result, dst)
	for _, v := range src {
		result = append(result, deepCopyValue(v))
	}
	return result
}

// mergeSlicesNative performs an element-wise deep merge of src into dst, matching
// the behaviour of mergo.WithSliceDeepCopy + WithOverride + WithTypeCheck:
//   - The result length equals the dst length (extra src elements are ignored).
//   - For each position i that exists in both dst and src: if both elements are
//     map[string]any they are deep-merged; otherwise the dst element is kept.
//   - Positions that exist only in dst are preserved as-is.
func mergeSlicesNative(dst, src []any) []any {
	result := make([]any, len(dst))
	copy(result, dst)

	// Merge src elements into the result up to the length of dst.
	for i := 0; i < len(src) && i < len(dst); i++ {
		srcMap, srcIsMap := src[i].(map[string]any)
		if !srcIsMap {
			// Non-map src element: dst[i] is preserved (mergo keeps dst for scalars).
			continue
		}
		dstMap, dstIsMap := dst[i].(map[string]any)
		if !dstIsMap {
			// Type mismatch: dst element is preserved.
			continue
		}
		// Both are maps: deep-merge into a new container so src is not aliased.
		// Use combined length as capacity hint to avoid reallocations when src adds new keys.
		merged := make(map[string]any, safeAdd(len(dstMap), len(srcMap)))
		for k, v := range dstMap {
			merged[k] = v
		}
		deepMergeNative(merged, srcMap, false, false)
		result[i] = merged
	}
	return result
}
