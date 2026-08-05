package utils

// AppendTagMetadataKey is the reserved map key used to wrap a list that carries the
// !append directive. The YAML parsing phase (atmos.yaml via handleAppend and stack
// manifests via processCustomTagsInner) wraps !append-tagged sequences as
// map[string]any{AppendTagMetadataKey: list}; the merge phase (pkg/merge) detects this
// wrapper and appends the list to the inherited value instead of replacing it.
const AppendTagMetadataKey = "__atmos_append__"

// HasAppendTag reports whether a value carries the !append metadata wrapper produced
// during YAML parsing.
func HasAppendTag(value any) bool {
	if m, ok := value.(map[string]any); ok {
		if _, hasAppend := m[AppendTagMetadataKey]; hasAppend {
			return true
		}
	}
	return false
}

// ExtractAppendListValue extracts the actual list value from an append-tagged structure.
func ExtractAppendListValue(value any) ([]any, bool) {
	if m, ok := value.(map[string]any); ok {
		if listValue, hasAppend := m[AppendTagMetadataKey]; hasAppend {
			if list, isList := listValue.([]any); isList {
				return list, true
			}
		}
	}
	return nil, false
}

// WrapWithAppendTag wraps a list value with append metadata.
// This metadata is used during merging to identify lists that should be appended.
func WrapWithAppendTag(list []any) map[string]any {
	return map[string]any{
		AppendTagMetadataKey: list,
	}
}
