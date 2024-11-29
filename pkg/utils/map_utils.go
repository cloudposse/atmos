package utils

import (
	"sort"

	"github.com/samber/lo"
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

// SortMapByKeysAndValuesUniq sorts the provided map by the keys, sorts the map values (lists of strings), and makes the values unique
func SortMapByKeysAndValuesUniq(m map[string][]string) map[string][]string {
	keys := lo.Keys(m)
	sort.Strings(keys)
	res := make(map[string][]string)
	for _, k := range keys {
		res[k] = lo.Uniq(m[k])
		sort.Strings(res[k])
	}
	return res
}

func MapOfInterfacesToMapOfStrings(input map[any]any) map[string]any {
	converted := make(map[string]any)
	for key, value := range input {
		strKey, ok := key.(string)
		if ok {
			converted[strKey] = value
		}
	}
	return converted
}
