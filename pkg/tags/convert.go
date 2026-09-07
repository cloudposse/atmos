package tags

import (
	"sort"

	"github.com/cloudposse/atmos/pkg/perf"
)

// ToStringSlice coerces a YAML-decoded value (typically []any) into a
// []string, skipping any non-string elements. Returns an empty (non-nil)
// slice for nil/invalid input so callers always get a rangeable value.
func ToStringSlice(v any) []string {
	defer perf.Track(nil, "tags.ToStringSlice")()

	raw, ok := v.([]any)
	if !ok {
		return []string{}
	}

	result := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// ToStringMap coerces a YAML-decoded value (typically map[string]any) into a
// map[string]string, skipping any non-string values. Returns an empty
// (non-nil) map for nil/invalid input so callers always get a rangeable value.
func ToStringMap(v any) map[string]string {
	defer perf.Track(nil, "tags.ToStringMap")()

	raw, ok := v.(map[string]any)
	if !ok {
		return map[string]string{}
	}

	result := make(map[string]string, len(raw))
	for k, item := range raw {
		if s, ok := item.(string); ok {
			result[k] = s
		}
	}
	return result
}

// SortedKeys returns the sorted keys of a map[string]string.
func SortedKeys(m map[string]string) []string {
	defer perf.Track(nil, "tags.SortedKeys")()

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// SortedValues returns the values of a map[string]string, ordered by key for
// deterministic output (map iteration order is randomized in Go).
func SortedValues(m map[string]string) []string {
	defer perf.Track(nil, "tags.SortedValues")()

	keys := SortedKeys(m)
	values := make([]string, 0, len(keys))
	for _, k := range keys {
		values = append(values, m[k])
	}
	return values
}
