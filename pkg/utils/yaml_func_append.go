package utils

import (
	"fmt"

	log "github.com/cloudposse/atmos/pkg/logger"
)

// AppendTagMetadataKey is the reserved map key used to wrap a list that carries the
// !append directive. The YAML parsing phase (atmos.yaml via handleAppend and stack
// manifests via processCustomTagsInner) wraps !append-tagged sequences as
// map[string]any{AppendTagMetadataKey: list}; the merge phase (pkg/merge) detects this
// wrapper and appends the list to the inherited value instead of replacing it.
const AppendTagMetadataKey = "__atmos_append__"

// ProcessAppendTag is a marker function that identifies a list should be appended during merging.
// The actual append logic is handled during the merge phase in pkg/merge.
// This function simply validates that the tag is being used correctly.
func ProcessAppendTag(node any) error {
	log.Debug("Processing !append tag", "node_type", fmt.Sprintf("%T", node))

	// The !append tag should only be used on sequence nodes (lists).
	// This validation will be done at the YAML parsing level.
	// This function serves as a marker for the merge logic.
	return nil
}

// IsAppendTag checks if a string contains the append tag.
func IsAppendTag(tag string) bool {
	return tag == AtmosYamlFuncAppend
}

// HasAppendTag checks if a value has the append tag metadata.
// This is used during merging to determine if a list should be appended.
func HasAppendTag(value any) bool {
	// Check if the value is a map with append metadata.
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
