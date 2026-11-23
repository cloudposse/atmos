package exec

import (
	"fmt"
	"strconv"

	errUtils "github.com/cloudposse/atmos/errors"
)

// findNullMapKey returns the dotted path of the first mapping that has a null key (YAML `null:`
// or `~:`), or false when there is none. YAML decodes such a mapping into map[any]any with a nil
// key, which cannot be named, merged, or converted to JSON. Only map[any]any can hold a nil key,
// so other maps are only descended into.
func findNullMapKey(value any, path string) (string, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return findNullMapKeyIn(typed, path, func(key string) string { return key })
	case map[any]any:
		if _, ok := typed[nil]; ok {
			return path, true
		}
		return findNullMapKeyIn(typed, path, func(key any) string { return fmt.Sprint(key) })
	case []any:
		for i, child := range typed {
			if found, ok := findNullMapKey(child, path+"["+strconv.Itoa(i)+"]"); ok {
				return found, true
			}
		}
	}
	return "", false
}

// findNullMapKeyIn descends into the values of a mapping, naming each child by its key.
func findNullMapKeyIn[K comparable](typed map[K]any, path string, name func(K) string) (string, bool) {
	for key, child := range typed {
		if found, ok := findNullMapKey(child, joinManifestPath(path, name(key))); ok {
			return found, true
		}
	}
	return "", false
}

func joinManifestPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// checkStackManifestNullKeys rejects a stack manifest containing a null mapping key with a
// validation error instead of letting later JSON Schema validation or merging panic on it.
func checkStackManifestNullKeys(manifest map[string]any, relativeFilePath string) error {
	path, found := findNullMapKey(manifest, "")
	if !found {
		return nil
	}
	return errUtils.Build(errUtils.ErrInvalidStackManifest).
		WithExplanationf("The stack manifest `%s` has a null key under `%s`. YAML reads an unquoted `null:` or `~:` key as a null value, not as a name.", relativeFilePath, path).
		WithHint("Quote the key (for example, `\"null\":`) to use `null` as a component or key name, or remove it.").
		WithContext("file", relativeFilePath).
		WithContext("path", path).
		Err()
}
