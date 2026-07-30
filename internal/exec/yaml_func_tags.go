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

// processTagTags processes the !tags YAML function.
// It returns the current component's own metadata.tags as a []string.
// The function takes no parameters and returns an empty list if metadata.tags is unset.
//
// Usage in YAML:
//
//	tags: !tags
func processTagTags(atmosConfig *schema.AtmosConfiguration, _ string, stackInfo *schema.ConfigAndStacksInfo) any {
	defer perf.Track(atmosConfig, "exec.processTagTags")()

	metadata := componentMetadata(stackInfo)
	return tags.ToStringSlice(metadata["tags"])
}

// processTagLabels processes the !labels YAML function.
// It returns the current component's own metadata.labels as a map[string]string.
// The function takes no parameters and returns an empty map if metadata.labels is unset.
//
// Usage in YAML:
//
//	labels: !labels
func processTagLabels(atmosConfig *schema.AtmosConfiguration, _ string, stackInfo *schema.ConfigAndStacksInfo) any {
	defer perf.Track(atmosConfig, "exec.processTagLabels")()

	metadata := componentMetadata(stackInfo)
	return tags.ToStringMap(metadata["labels"])
}

// processTagLabelsKeys processes the !labels.keys YAML function.
// It returns the current component's own metadata.labels keys as a sorted []string.
// The function takes no parameters and returns an empty list if metadata.labels is unset.
//
// Usage in YAML:
//
//	label_keys: !labels.keys
func processTagLabelsKeys(atmosConfig *schema.AtmosConfiguration, _ string, stackInfo *schema.ConfigAndStacksInfo) any {
	defer perf.Track(atmosConfig, "exec.processTagLabelsKeys")()

	metadata := componentMetadata(stackInfo)
	return tags.SortedKeys(tags.ToStringMap(metadata["labels"]))
}

// processTagLabelsValues processes the !labels.values YAML function.
// It returns the current component's own metadata.labels values as a
// []string, ordered by key for deterministic output.
// The function takes no parameters and returns an empty list if metadata.labels is unset.
//
// Usage in YAML:
//
//	label_values: !labels.values
func processTagLabelsValues(atmosConfig *schema.AtmosConfiguration, _ string, stackInfo *schema.ConfigAndStacksInfo) any {
	defer perf.Track(atmosConfig, "exec.processTagLabelsValues")()

	metadata := componentMetadata(stackInfo)
	return tags.SortedValues(tags.ToStringMap(metadata["labels"]))
}
