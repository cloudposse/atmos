package utils

import (
	"fmt"
	"sort"

	"github.com/samber/lo"

	"github.com/cloudposse/atmos/pkg/perf"
)

// StringKeysFromMap returns a slice of sorted string keys from the provided map
func StringKeysFromMap(m map[string]any) []string {
	defer perf.Track(nil, "utils.StringKeysFromMap")()

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
	defer perf.Track(nil, "utils.SortMapByKeysAndValuesUniq")()

	keys := lo.Keys(m)
	sort.Strings(keys)
	res := make(map[string][]string)
	for _, k := range keys {
		res[k] = lo.Uniq(m[k])
		sort.Strings(res[k])
	}
	return res
}

// MapOfInterfacesToMapOfStrings converts map[string]any to map[string]string
func MapOfInterfacesToMapOfStrings(input map[string]any) map[string]string {
	defer perf.Track(nil, "utils.MapOfInterfacesToMapOfStrings")()

	return lo.MapEntries(input, func(key string, value any) (string, string) {
		return key, fmt.Sprintf("%v", value)
	})
}

// MapOfInterfaceKeysToMapOfStringKeys converts map[any]any to map[string]any
func MapOfInterfaceKeysToMapOfStringKeys(input map[any]any) map[string]any {
	defer perf.Track(nil, "utils.MapOfInterfaceKeysToMapOfStringKeys")()

	converted := make(map[string]any, len(input))
	for key, value := range input {
		strKey, ok := key.(string)
		if ok {
			converted[strKey] = value
		}
	}
	return converted
}
