package utils

import (
	"sort"
)

// StringKeysFromMap returns a slice of sorted string keys from the provided map
func StringKeysFromMap(m map[string]any) []string {
	keys := []string{}
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// MapKeyExists checks if a key already defined in a map
func MapKeyExists(m map[string]any, key string) bool {
	_, ok := m[key]
	return ok
}
