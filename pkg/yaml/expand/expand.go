// Package expand provides YAML key delimiter expansion for yaml.Node trees.
// It walks mapping nodes and expands unquoted keys containing a configurable
// delimiter into nested map structures, modeled after Viper's deepSearch().
package expand

import (
	"strings"

	goyaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/perf"
)

// KeyDelimiters walks a yaml.Node tree and expands unquoted mapping keys
// containing the delimiter into nested mapping structures. Quoted keys
// (single or double) are preserved as literal keys.
//
// For example, with delimiter ".", the unquoted key "metadata.component: vpc-base"
// becomes the nested structure "metadata: { component: vpc-base }".
//
// This is modeled after Viper's deepSearch() approach (viper@v1.21.0/util.go).
func KeyDelimiters(node *goyaml.Node, delimiter string) {
	defer perf.Track(nil, "yaml.expand.KeyDelimiters")()

	if node == nil || delimiter == "" {
		return
	}

	keyDelimitersRecursive(node, delimiter)
}

// keyDelimitersRecursive is the recursive implementation.
// Separated from the public entry point so perf.Track fires only once.
func keyDelimitersRecursive(node *goyaml.Node, delimiter string) {
	if node == nil {
		return
	}

	switch node.Kind {
	case goyaml.DocumentNode:
		for _, child := range node.Content {
			keyDelimitersRecursive(child, delimiter)
		}
	case goyaml.MappingNode:
		expandMappingKeys(node, delimiter)
	case goyaml.SequenceNode:
		for _, child := range node.Content {
			keyDelimitersRecursive(child, delimiter)
		}
	}
}

// expandMappingKeys processes a single MappingNode, expanding unquoted delimited keys
// into nested structures.
func expandMappingKeys(node *goyaml.Node, delimiter string) {
	// First, recurse into all value nodes so nested maps are expanded bottom-up.
	for i := 1; i < len(node.Content); i += 2 {
		keyDelimitersRecursive(node.Content[i], delimiter)
	}

	// Collect expanded entries: for each expandable key, build nested nodes.
	// Non-expandable keys pass through unchanged.
	var newContent []*goyaml.Node

	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]

		if shouldExpand(keyNode, delimiter) {
			parts := strings.Split(keyNode.Value, delimiter)
			nested := buildNestedNodes(parts, valueNode)
			// Merge the expanded key-value pair into newContent.
			mergeIntoContent(&newContent, nested[0], nested[1])
		} else {
			// Non-expandable: merge as-is (handles duplicate keys by last-wins).
			mergeIntoContent(&newContent, keyNode, valueNode)
		}
	}

	node.Content = newContent
}

// shouldExpand returns true if the key node should be expanded:
// - Must be a scalar node.
// - Must be unquoted (Style == 0).
// - Must contain the delimiter.
// - Must not have leading, trailing, or consecutive delimiters.
func shouldExpand(keyNode *goyaml.Node, delimiter string) bool {
	if keyNode.Kind != goyaml.ScalarNode {
		return false
	}

	// Quoted keys are never expanded.
	if keyNode.Style == goyaml.DoubleQuotedStyle || keyNode.Style == goyaml.SingleQuotedStyle {
		return false
	}

	value := keyNode.Value
	if !strings.Contains(value, delimiter) {
		return false
	}

	// Reject malformed patterns: leading, trailing, or consecutive delimiters.
	if strings.HasPrefix(value, delimiter) || strings.HasSuffix(value, delimiter) {
		return false
	}
	if strings.Contains(value, delimiter+delimiter) {
		return false
	}

	// All parts must be non-empty after splitting.
	parts := strings.Split(value, delimiter)
	for _, part := range parts {
		if part == "" {
			return false
		}
	}

	return true
}

// buildNestedNodes creates a chain of nested MappingNodes from the key parts.
// For parts ["a", "b", "c"] and a value node, it creates:
//
//	a: { b: { c: value } }
//
// Returns [keyNode, valueNode] where valueNode may be a nested MappingNode.
func buildNestedNodes(parts []string, valueNode *goyaml.Node) [2]*goyaml.Node {
	// Build from innermost to outermost.
	currentValue := valueNode
	for i := len(parts) - 1; i >= 1; i-- {
		innerKey := &goyaml.Node{
			Kind:  goyaml.ScalarNode,
			Tag:   "!!str",
			Value: parts[i],
		}
		innerMap := &goyaml.Node{
			Kind:    goyaml.MappingNode,
			Tag:     "!!map",
			Content: []*goyaml.Node{innerKey, currentValue},
		}
		currentValue = innerMap
	}

	outerKey := &goyaml.Node{
		Kind:  goyaml.ScalarNode,
		Tag:   "!!str",
		Value: parts[0],
	}

	return [2]*goyaml.Node{outerKey, currentValue}
}

// mergeIntoContent adds a key-value pair to the content slice.
// If the key already exists and both old and new values are MappingNodes,
// the entries are merged (new entries win on conflict).
// Otherwise, the old entry is replaced (last-wins semantics).
func mergeIntoContent(content *[]*goyaml.Node, keyNode, valueNode *goyaml.Node) {
	// Look for an existing key with the same value.
	for i := 0; i < len(*content); i += 2 {
		existingKey := (*content)[i]
		if existingKey.Kind == goyaml.ScalarNode && existingKey.Value == keyNode.Value {
			existingValue := (*content)[i+1]

			// If both are mappings, merge the entries.
			if existingValue.Kind == goyaml.MappingNode && valueNode.Kind == goyaml.MappingNode {
				mergeMappingNodes(existingValue, valueNode)
				return
			}

			// Otherwise, replace (last-wins).
			(*content)[i+1] = valueNode
			return
		}
	}

	// Key not found: append.
	*content = append(*content, keyNode, valueNode)
}

// mergeMappingNodes merges entries from src into dst.
// If a key exists in both and both values are MappingNodes, they are recursively merged.
// Otherwise, src wins (last-wins).
func mergeMappingNodes(dst, src *goyaml.Node) {
	for i := 0; i < len(src.Content); i += 2 {
		srcKey := src.Content[i]
		srcValue := src.Content[i+1]
		mergeIntoContent(&dst.Content, srcKey, srcValue)
	}
}
