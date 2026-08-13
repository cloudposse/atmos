package templatefuncs

import (
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// CollectKeys is the "collectKeys" template function. With no extra
// argument it returns v's top-level keys, sorted for deterministic output.
// With a nestedKey argument, it treats v as a map of maps and instead
// collects nestedKey's own keys from every one of v's values, flattening
// and deduplicating across all of them -- e.g. collectKeys(environments,
// "regions") returns every region that appears in any environment.
//
// Sprig ships its own "keys", but it takes multiple maps (returning their
// keys' union, unsorted, with duplicates), and has no nested-key mode --
// a different function for a different purpose, not a replacement for
// this one. CollectKeys is registered under its own name precisely so it
// never shadows Sprig's.
func CollectKeys(v any, nestedKey ...string) ([]string, error) {
	defer perf.Track(nil, "templatefuncs.CollectKeys")()

	m, err := toStringKeyedMap(v)
	if err != nil {
		return nil, err
	}

	if len(nestedKey) == 0 {
		return sortedKeys(m), nil
	}

	return flattenNestedKeys(m, nestedKey[0]), nil
}

// toStringKeyedMap asserts v is a map keyed by string, the shape YAML-decoded
// data (scaffold answers, stack vars, and similar) always uses.
func toStringKeyedMap(v any) (map[string]interface{}, error) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, errUtils.Build(errUtils.ErrCollectKeysNotMap).
			WithExplanationf("collectKeys: expected a map, got %T", v).
			Err()
	}
	return m, nil
}

// sortedKeys returns m's keys in sorted order.
func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// flattenNestedKeys collects nestedKey's own keys from every value in m,
// flattening and deduplicating across all of them. A value that isn't a map,
// or doesn't have nestedKey, is silently skipped -- callers with
// heterogeneous data shapes get a best-effort result rather than an error.
func flattenNestedKeys(m map[string]interface{}, nestedKey string) []string {
	seen := make(map[string]struct{})
	for _, entry := range m {
		entryMap, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		nested, ok := entryMap[nestedKey]
		if !ok {
			continue
		}
		nestedMap, ok := nested.(map[string]interface{})
		if !ok {
			continue
		}
		for k := range nestedMap {
			seen[k] = struct{}{}
		}
	}

	result := make([]string, 0, len(seen))
	for k := range seen {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}
