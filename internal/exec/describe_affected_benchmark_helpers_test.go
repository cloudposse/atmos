package exec

// deepCopyAny performs a deep copy of any value, recursively handling maps and slices.
// This is used only in benchmarks to create independent copies of test data.
// Uncommon types are returned as-is (benchmark-only helper).
func deepCopyAny(v any) any {
	if v == nil {
		return nil
	}

	// Type assertions for common types to avoid reflection overhead.
	switch typed := v.(type) {
	case map[string]any:
		return deepCopyMap(typed)

	case []any:
		return deepCopySlice(typed)

	case string, int, int64, float64, bool:
		// Immutable primitives can be returned as-is.
		return typed

	default:
		// Uncommon types: return as-is (bench-only helper).
		return v
	}
}

// deepCopyMap creates a deep copy of a map.
// This is used only in benchmarks to create independent copies of test data.
func deepCopyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}

	result := make(map[string]any, len(m))
	for key, value := range m {
		result[key] = deepCopyAny(value)
	}

	return result
}

// deepCopySlice creates a deep copy of a slice.
// This is used only in benchmarks to create independent copies of test data.
func deepCopySlice(s []any) []any {
	if s == nil {
		return nil
	}

	result := make([]any, len(s))
	for i, value := range s {
		result[i] = deepCopyAny(value)
	}

	return result
}
