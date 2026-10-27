package imports

import (
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

func labelsFromMetadata(metadata *yaml.Node, delimiters []string) schema.StackImportLabels {
	if metadata.Kind == 0 {
		return schema.StackImportLabels{}
	}
	var fields map[string]yaml.Node
	if metadata.ShortTag() != "!!map" || metadata.Decode(&fields) != nil {
		return schema.StackImportLabels{Reset: true}
	}
	labels, exists := fields["labels"]
	if !exists {
		return schema.StackImportLabels{}
	}
	var values map[string]yaml.Node
	if labels.ShortTag() != "!!map" || labels.Decode(&values) != nil {
		return schema.StackImportLabels{Reset: true}
	}
	result := schema.StackImportLabels{Values: make(map[string]any, len(values))}
	for key := range values {
		value := values[key]
		// Unknown values mask inherited values, rather than accidentally enabling a flag.
		result.Values[key] = literalLabelValue(&value, delimiters)
	}
	return result
}

func literalLabelValue(value *yaml.Node, delimiters []string) any {
	switch value.ShortTag() {
	case "!!bool":
		var flag bool
		if value.Decode(&flag) == nil {
			return flag
		}
	case "!!str":
		var text string
		if value.Decode(&text) == nil && !isLabelTemplate(text, delimiters) {
			return text
		}
	}
	return nil
}

func isLabelTemplate(value string, delimiters []string) bool {
	if strings.Contains(value, "{{") {
		return true
	}
	return len(delimiters) == 2 && delimiters[0] != "" && strings.Contains(value, delimiters[0])
}

// MergeLabels merges label layers in increasing precedence without mutating any input.
func MergeLabels(layers ...schema.StackImportLabels) schema.StackImportLabels {
	defer perf.Track(nil, "imports.MergeLabels")()

	var result schema.StackImportLabels
	for _, layer := range layers {
		if layer.Reset {
			result = schema.StackImportLabels{Reset: true}
		}
		for key, value := range layer.Values {
			if result.Values == nil {
				result.Values = make(map[string]any)
			}
			result.Values[key] = value
		}
	}
	return result
}

// knownLabels excludes values unavailable before template/function evaluation.
func knownLabels(labels schema.StackImportLabels) map[string]any {
	result := make(map[string]any, len(labels.Values))
	for key, value := range labels.Values {
		if value != nil {
			result[key] = value
		}
	}
	return result
}
