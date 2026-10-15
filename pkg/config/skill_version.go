package config

import (
	"fmt"

	"go.yaml.in/yaml/v3"
)

// rejectSequenceVersionTags rejects version tags anywhere in a sequence subtree.
// Skill-source refs are mapping values, so no sequence descendant is a valid
// location, even when decoding that descendant would otherwise discard its tag.
func rejectSequenceVersionTags(node *yaml.Node, path string) error {
	if node.Tag == "!version" {
		return unsupportedAtmosYamlTagError(node.Tag, path)
	}

	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			childPath := path + "." + node.Content[i].Value
			for _, child := range node.Content[i : i+2] {
				if err := rejectSequenceVersionTags(child, childPath); err != nil {
					return err
				}
			}
		}
	case yaml.SequenceNode:
		for i, child := range node.Content {
			if err := rejectSequenceVersionTags(child, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	}
	return nil
}
