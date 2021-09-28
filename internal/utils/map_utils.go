package utils

import "sort"

// StringKeysFromMap returns a slice of sorted string keys from the provided map
func StringKeysFromMap(m map[string]interface{}) []string {
	keys := []string{}
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
