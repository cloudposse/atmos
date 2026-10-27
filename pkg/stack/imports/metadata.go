package imports

import (
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/condition"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const metadataTagsKey = "tags"

// unavailableTags is an opaque merge value, distinct from an explicitly empty/null list.
type unavailableTags struct{}

// MetadataScope keeps earlier defaults separate from the importing manifests' explicit metadata.
// Overrides remain ordered layers so structural !append tags retain their meaning in nested imports.
type MetadataScope struct {
	Before    schema.StackImportMetadata
	Overrides []schema.StackImportMetadata
	Config    *schema.AtmosConfiguration
}

// ReadMetadata extracts literal stack-wide labels and tags without running templates or functions.
func ReadMetadata(content string, delimiters []string) schema.StackImportMetadata {
	defer perf.Track(nil, "imports.ReadMetadata")()

	var document struct {
		Metadata yaml.Node `yaml:"metadata"`
	}
	if yaml.Unmarshal([]byte(content), &document) != nil {
		return schema.StackImportMetadata{Labels: schema.StackImportLabels{Reset: true}, Tags: map[string]any{metadataTagsKey: unavailableTags{}}}
	}
	return schema.StackImportMetadata{
		Labels: labelsFromMetadata(&document.Metadata, delimiters),
		Tags:   tagsFromMetadata(&document.Metadata, delimiters),
	}
}

func tagsFromMetadata(metadata *yaml.Node, delimiters []string) map[string]any {
	if metadata.Kind == 0 {
		return nil
	}
	var fields map[string]yaml.Node
	if metadata.ShortTag() != "!!map" || metadata.Decode(&fields) != nil {
		return map[string]any{metadataTagsKey: unavailableTags{}}
	}
	tags, exists := fields[metadataTagsKey]
	if !exists {
		return nil
	}
	return map[string]any{metadataTagsKey: literalTags(&tags, delimiters)}
}

func literalTags(tags *yaml.Node, delimiters []string) any {
	if tags.ShortTag() == "!!null" {
		return nil
	}
	if tags.ShortTag() != "!!seq" && tags.ShortTag() != "!append" {
		return unavailableTags{}
	}
	var nodes []yaml.Node
	if tags.Decode(&nodes) != nil {
		return unavailableTags{}
	}
	values := make([]any, len(nodes))
	for i := range nodes {
		value, ok := literalLabelValue(&nodes[i], delimiters).(string)
		// An unknown member makes the list unavailable, rather than falling back to inherited tags.
		if !ok {
			return unavailableTags{}
		}
		values[i] = value
	}
	if tags.ShortTag() == "!append" {
		return u.WrapWithAppendTag(values)
	}
	return values
}

// Merge combines metadata using the same list strategy as stack loading.
func (scope MetadataScope) Merge(layers ...schema.StackImportMetadata) (schema.StackImportMetadata, error) {
	defer perf.Track(nil, "imports.MetadataScope.Merge")()

	var result schema.StackImportMetadata
	var tagLayers []map[string]any
	for _, layer := range layers {
		result.Labels = MergeLabels(result.Labels, layer.Labels)
		if len(layer.Tags) > 0 {
			tagLayers = append(tagLayers, layer.Tags)
		}
	}
	if len(tagLayers) == 0 {
		return result, nil
	}
	config := scope.Config
	if config == nil {
		config = &schema.AtmosConfiguration{}
	}
	var err error
	result.Tags, err = m.Merge(config, tagLayers)
	if err == nil && tagsUnavailable(tagLayers, config.Settings.ListMergeStrategy) {
		result.Tags[metadataTagsKey] = unavailableTags{}
	}
	return result, err
}

// Activate supplies the literal metadata available to the current condition.
func (scope MetadataScope) Activate(ctx *condition.Context, imported, local schema.StackImportMetadata) error {
	defer perf.Track(nil, "imports.MetadataScope.Activate")()

	layers := append([]schema.StackImportMetadata{scope.Before, imported, local}, scope.Overrides...)
	merged, err := scope.Merge(layers...)
	if err != nil {
		return err
	}
	ctx.Labels = knownLabels(merged.Labels)
	ctx.Tags = []string{}
	_, ctx.TagsUnavailable = merged.Tags[metadataTagsKey].(unavailableTags)
	if tags, ok := merged.Tags[metadataTagsKey].([]any); ok {
		for _, tag := range tags {
			if value, ok := tag.(string); ok {
				ctx.Tags = append(ctx.Tags, value)
			}
		}
	}
	return nil
}

// Child snapshots the scope before an import entry; all glob matches share this snapshot.
func (scope MetadataScope) Child(imported, local schema.StackImportMetadata) (MetadataScope, error) {
	defer perf.Track(nil, "imports.MetadataScope.Child")()

	before, err := scope.Merge(scope.Before, imported)
	if err != nil {
		return MetadataScope{}, err
	}
	return MetadataScope{Before: before, Overrides: append([]schema.StackImportMetadata{local}, scope.Overrides...), Config: scope.Config}, nil
}

// Unknown list contents remain unknown when appending/merging. Only an explicit
// replacement or null can establish a fully known list again.
func tagsUnavailable(layers []map[string]any, strategy string) bool {
	unknown := false
	for _, layer := range layers {
		value, exists := layer[metadataTagsKey]
		if !exists {
			continue
		}
		if _, unavailable := value.(unavailableTags); unavailable {
			unknown = true
		} else if value == nil || (strategy != m.ListMergeStrategyAppend && strategy != m.ListMergeStrategyMerge && !u.HasAppendTag(value)) {
			unknown = false
		}
	}
	return unknown
}
