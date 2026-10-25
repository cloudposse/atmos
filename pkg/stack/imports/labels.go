package imports

import (
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// LabelScope carries earlier import defaults separately from more-specific ancestor labels.
// Keeping them separate lets a child's own labels override defaults without overriding its parent.
type LabelScope struct {
	Before   schema.StackImportLabels
	Override schema.StackImportLabels
}

// ReadLabels extracts literal native metadata.labels without evaluating templates or YAML functions.
// Both stack loading and early authentication discovery use this representation.
func ReadLabels(content string, delimiters []string) schema.StackImportLabels {
	defer perf.Track(nil, "imports.ReadLabels")()

	var document struct {
		Metadata yaml.Node `yaml:"metadata"`
	}
	if err := yaml.Unmarshal([]byte(content), &document); err != nil {
		return schema.StackImportLabels{Reset: true}
	}
	return labelsFromMetadata(&document.Metadata, delimiters)
}

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
	if strings.HasPrefix(value, "!") || strings.Contains(value, "{{") {
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

// Effective returns only known literal labels for the current import condition.
func (scope LabelScope) Effective(imported, local schema.StackImportLabels) map[string]any {
	defer perf.Track(nil, "imports.LabelScope.Effective")()

	merged := MergeLabels(scope.Before, imported, local, scope.Override)
	result := make(map[string]any, len(merged.Values))
	for key, value := range merged.Values {
		if value != nil {
			result[key] = value
		}
	}
	return result
}

// Child snapshots the scope for one import entry. All glob matches share this snapshot.
func (scope LabelScope) Child(imported, local schema.StackImportLabels) LabelScope {
	defer perf.Track(nil, "imports.LabelScope.Child")()

	return LabelScope{
		Before:   MergeLabels(scope.Before, imported),
		Override: MergeLabels(local, scope.Override),
	}
}
