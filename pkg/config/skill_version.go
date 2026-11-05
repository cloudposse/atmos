package config

import (
	"fmt"

	"go.yaml.in/yaml/v3"
)

// rejectSequenceVersionTags rejects version tags anywhere in a sequence subtree.
// Skill-source refs are mapping values, so no sequence descendant is a valid
// location, even when decoding that descendant would otherwise discard its tag.
func rejectSequenceVersionTags(node *yaml.Node, path string) error {
	return rejectSequenceVersionTagsSeen(node, path, make(map[*yaml.Node]bool))
}

// rejectSequenceVersionTagsSeen follows aliases once per node to avoid cycles
// and repeated traversal of shared anchors while retaining the reference path.
func rejectSequenceVersionTagsSeen(node *yaml.Node, path string, seen map[*yaml.Node]bool) error {
	if node == nil || seen[node] {
		return nil
	}
	seen[node] = true

	if node.Tag == "!version" {
		return unsupportedAtmosYamlTagError(node.Tag, path)
	}

	switch node.Kind {
	case yaml.AliasNode:
		return rejectSequenceVersionTagsSeen(node.Alias, path, seen)
	case yaml.MappingNode:
		return rejectSequenceVersionMappingTags(node, path, seen)
	case yaml.SequenceNode:
		for i, child := range node.Content {
			if err := rejectSequenceVersionTagsSeen(child, fmt.Sprintf("%s[%d]", path, i), seen); err != nil {
				return err
			}
		}
	}
	return nil
}

// rejectSequenceVersionMappingTags validates both keys and values of a mapping
// reached within a sequence, sharing the traversal's alias cycle protection.
func rejectSequenceVersionMappingTags(node *yaml.Node, path string, seen map[*yaml.Node]bool) error {
	for i := 0; i < len(node.Content); i += 2 {
		childPath := path + "." + node.Content[i].Value
		for _, child := range node.Content[i : i+2] {
			if err := rejectSequenceVersionTagsSeen(child, childPath, seen); err != nil {
				return err
			}
		}
	}
	return nil
}
