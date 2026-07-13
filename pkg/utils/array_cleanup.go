package utils

import (
	"regexp"
)

// arrayIndexPattern matches array index patterns like "steps[0]", "steps[1]", etc.
var arrayIndexPattern = regexp.MustCompile(`^(.+)\[(\d+)\]$`)

// CleanupArrayIndexKeys removes duplicate indexed keys from maps that also contain arrays.
// Viper sometimes creates both array entries and indexed map keys (e.g., both "steps" array
// and "steps[0]", "steps[1]" keys). This function removes the indexed keys when an array exists.
//
// This function only processes map[string]interface{} types and preserves all other types as-is.
// It does NOT convert structs to maps, which would lose YAML features like anchors and tags.
func CleanupArrayIndexKeys(data interface{}) interface{} {
	// Handle different data types
	switch v := data.(type) {
	case map[string]interface{}:
		return cleanupMapArrayKeys(v)
	case []interface{}:
		// Recursively clean array elements
		cleaned := make([]interface{}, len(v))
		for i, item := range v {
			cleaned[i] = CleanupArrayIndexKeys(item)
		}
		return cleaned
	default:
		// Return all other types as-is (including structs)
		// This preserves the original structure and avoids JSON conversion
		return data
	}
}

// cleanupMapArrayKeys removes indexed keys when corresponding arrays exist.
func cleanupMapArrayKeys(m map[string]interface{}) map[string]interface{} {
	cleaned := make(map[string]interface{})
	arrayFields := make(map[string]bool)

	// First pass: identify array fields and copy regular fields
	for key, value := range m {
		// Check if this is an indexed key
		if matches := arrayIndexPattern.FindStringSubmatch(key); matches != nil {
			// Mark the base field name as having indexed entries
			arrayFields[matches[1]] = true
		} else {
			// Recursively clean the value
			cleaned[key] = CleanupArrayIndexKeys(value)
		}
	}

	// Second pass: remove indexed keys for fields that have arrays
	for key, value := range m {
		if matches := arrayIndexPattern.FindStringSubmatch(key); matches != nil {
			baseField := matches[1]
			// Only skip if the base field exists and is an array/slice
			if _, exists := cleaned[baseField]; exists {
				if isArrayOrSlice(cleaned[baseField]) {
					// Skip this indexed key as we have the array
					continue
				}
			}
			// Keep the indexed key if no array exists (might be a map with indexed keys)
			cleaned[key] = CleanupArrayIndexKeys(value)
		}
	}

	return cleaned
}

// isArrayOrSlice checks if the value is an array or slice type.
func isArrayOrSlice(v interface{}) bool {
	switch v.(type) {
	case []interface{}, []string, []int, []float64, []bool:
		return true
	default:
		return false
	}
}
