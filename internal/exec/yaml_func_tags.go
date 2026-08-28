package exec

import (
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/tags"
)

// componentMetadata returns the current component's metadata section, or nil
// if stackInfo/ComponentSection/metadata isn't set. Shared by the !tags/!labels
// family of YAML functions below.
func componentMetadata(stackInfo *schema.ConfigAndStacksInfo) map[string]any {
	if stackInfo == nil {
		return nil
	}
	metadata, _ := stackInfo.ComponentSection[cfg.MetadataSectionName].(map[string]any)
	return metadata
}

// anySlice converts a []string to []any. The deferred-merge machinery's isSlice/mergeSlices
// (pkg/merge/merge_yaml_functions.go) only recognize []any — the same generic representation every
// other post-merge YAML function's JSON-decoded result naturally has — so a resolved !tags/
// !labels.keys/!labels.values value must be in that shape for deep-merging against a concrete
// override at the same path to work (see #2888: a []string value fell into the "simple type,
// highest precedence wins" branch instead, discarding the function's own contribution).
func anySlice(s []string) []any {
	result := make([]any, len(s))
	for i, v := range s {
		result[i] = v
	}
	return result
}

// anyMap converts a map[string]string to map[string]any. See anySlice's doc comment: the
// deferred-merge machinery's isMap/mergeDeferredMaps only recognize map[string]any.
func anyMap(m map[string]string) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

// processTagTags processes the !tags YAML function.
// It returns the current component's own metadata.tags as a []any of strings.
// The function takes no parameters and returns an empty list if metadata.tags is unset.
//
// Usage in YAML:
//
//	tags: !tags
func processTagTags(atmosConfig *schema.AtmosConfiguration, _ string, stackInfo *schema.ConfigAndStacksInfo) any {
	defer perf.Track(atmosConfig, "exec.processTagTags")()

	metadata := componentMetadata(stackInfo)
	return anySlice(tags.ToStringSlice(metadata["tags"]))
}

// processTagLabels processes the !labels YAML function.
// It returns the current component's own metadata.labels as a map[string]any of strings.
// The function takes no parameters and returns an empty map if metadata.labels is unset.
//
// Usage in YAML:
//
//	labels: !labels
func processTagLabels(atmosConfig *schema.AtmosConfiguration, _ string, stackInfo *schema.ConfigAndStacksInfo) any {
	defer perf.Track(atmosConfig, "exec.processTagLabels")()

	metadata := componentMetadata(stackInfo)
	return anyMap(tags.ToStringMap(metadata["labels"]))
}

// processTagLabelsKeys processes the !labels.keys YAML function.
// It returns the current component's own metadata.labels keys as a sorted []any of strings.
// The function takes no parameters and returns an empty list if metadata.labels is unset.
//
// Usage in YAML:
//
//	label_keys: !labels.keys
func processTagLabelsKeys(atmosConfig *schema.AtmosConfiguration, _ string, stackInfo *schema.ConfigAndStacksInfo) any {
	defer perf.Track(atmosConfig, "exec.processTagLabelsKeys")()

	metadata := componentMetadata(stackInfo)
	return anySlice(tags.SortedKeys(tags.ToStringMap(metadata["labels"])))
}

// processTagLabelsValues processes the !labels.values YAML function.
// It returns the current component's own metadata.labels values as a
// []any of strings, ordered by key for deterministic output.
// The function takes no parameters and returns an empty list if metadata.labels is unset.
//
// Usage in YAML:
//
//	label_values: !labels.values
func processTagLabelsValues(atmosConfig *schema.AtmosConfiguration, _ string, stackInfo *schema.ConfigAndStacksInfo) any {
	defer perf.Track(atmosConfig, "exec.processTagLabelsValues")()

	metadata := componentMetadata(stackInfo)
	return anySlice(tags.SortedValues(tags.ToStringMap(metadata["labels"])))
}
