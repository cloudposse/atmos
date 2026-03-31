package pro

import "fmt"

// stripToProSettings extracts only the "pro" key from a settings map,
// returning a new map with just that key. If settings is nil or has no
// "pro" key, returns nil.
//
// The "pro" value is sanitized to ensure all nested maps use string keys
// (converting map[interface{}]interface{} from YAML to map[string]interface{})
// so the result is safe for JSON marshaling.
func stripToProSettings(settings map[string]any) map[string]any {
	if settings == nil {
		return nil
	}

	pro, hasPro := settings["pro"]
	if !hasPro {
		return nil
	}

	return map[string]any{
		"pro": sanitizeValue(pro),
	}
}

// sanitizeValue recursively converts map[interface{}]interface{} to
// map[string]interface{} for JSON compatibility.
func sanitizeValue(v any) any {
	switch val := v.(type) {
	case map[interface{}]interface{}:
		m := make(map[string]interface{}, len(val))
		for k, v := range val {
			m[fmt.Sprintf("%v", k)] = sanitizeValue(v)
		}
		return m
	case map[string]interface{}:
		m := make(map[string]interface{}, len(val))
		for k, v := range val {
			m[k] = sanitizeValue(v)
		}
		return m
	case []interface{}:
		s := make([]interface{}, len(val))
		for i, v := range val {
			s[i] = sanitizeValue(v)
		}
		return s
	default:
		return v
	}
}
