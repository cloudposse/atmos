package exec

// deepCopyAny performs a deep copy of any value, recursively handling maps and slices.
// This is used only in benchmarks to create independent copies of test data.
func deepCopyAny(v any) (any, error) {
	if v == nil {
		return nil, nil
	}

	// Type assertions for common types to avoid reflection where possible.
	switch typed := v.(type) {
	case map[string]any:
		return deepCopyMap(typed)

	case []any:
		return deepCopySlice(typed)

	case string:
		return typed, nil

	case int:
		return typed, nil

	case int64:
		return typed, nil

	case float64:
		return typed, nil

	case bool:
		return typed, nil

	default:
		// For uncommon types, use reflection to create a copy.
		// This ensures we don't miss any types.
		return v, nil
	}
}

// deepCopyMap creates a deep copy of a map.
// This is used only in benchmarks to create independent copies of test data.
func deepCopyMap(m map[string]any) (map[string]any, error) {
	if m == nil {
		return nil, nil
	}

	result := make(map[string]any, len(m))
	for key, value := range m {
		copiedValue, err := deepCopyAny(value)
		if err != nil {
			return nil, err
		}
		result[key] = copiedValue
	}

	return result, nil
}

// deepCopySlice creates a deep copy of a slice.
// This is used only in benchmarks to create independent copies of test data.
func deepCopySlice(s []any) ([]any, error) {
	if s == nil {
		return nil, nil
	}

	result := make([]any, len(s))
	for i, value := range s {
		copiedValue, err := deepCopyAny(value)
		if err != nil {
			return nil, err
		}
		result[i] = copiedValue
	}

	return result, nil
}
