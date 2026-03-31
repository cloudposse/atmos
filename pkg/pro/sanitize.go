package pro

import "fmt"

// sanitizeForJSON recursively converts map[interface{}]interface{} (produced by
// yaml.v2 unmarshaling) to map[string]interface{} so the value is compatible
// with encoding/json.Marshal.
func sanitizeForJSON(v any) any {
	switch val := v.(type) {
	case map[interface{}]interface{}:
		m := make(map[string]interface{}, len(val))
		for k, v := range val {
			m[fmt.Sprintf("%v", k)] = sanitizeForJSON(v)
		}
		return m
	case map[string]interface{}:
		m := make(map[string]interface{}, len(val))
		for k, v := range val {
			m[k] = sanitizeForJSON(v)
		}
		return m
	case []interface{}:
		for i, v := range val {
			val[i] = sanitizeForJSON(v)
		}
		return val
	default:
		return v
	}
}

// sanitizeMapForJSON sanitizes a map[string]any so all nested maps are
// JSON-marshalable. Returns nil if the input is nil.
func sanitizeMapForJSON(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = sanitizeForJSON(v)
	}
	return result
}
