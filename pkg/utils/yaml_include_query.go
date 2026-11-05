package utils

import (
	yaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
)

// evaluateIncludeQuery uses the block collection style of the legacy decoded-data
// path, while retaining tags and scalar types. Otherwise a query like .[] over
// [{first: A}, {second: B}] prints adjacent flow mappings; yaml.Unmarshal accepts
// only the first and silently drops the remaining results. Block mappings retain
// both keys, and duplicate keys still fail when the result is decoded.
func evaluateIncludeQuery(atmosConfig *schema.AtmosConfiguration, text, query string) (string, error) {
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(text), &node); err != nil {
		return "", err
	}
	includeQueryBlockStyle(&node)
	encoded, err := yaml.Marshal(&node)
	if err != nil {
		return "", err
	}
	return evaluateYqToYAML(atmosConfig, string(encoded), query)
}

// includeQueryBlockStyle normalizes collections without changing scalar quoting,
// tags, or aliases, which are needed for typed data and explicit include evaluation.
func includeQueryBlockStyle(node *yaml.Node) {
	if node.Kind == yaml.MappingNode || node.Kind == yaml.SequenceNode {
		node.Style &^= yaml.FlowStyle
	}
	for _, child := range node.Content {
		includeQueryBlockStyle(child)
	}
}
