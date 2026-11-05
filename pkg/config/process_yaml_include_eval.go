package config

import (
	"go.yaml.in/yaml/v3"
	legacyyaml "gopkg.in/yaml.v3"
)

// Bound active includes as well as detecting repeated paths, since equivalent
// paths can have different spellings. Sibling includes do not consume depth.
const maxConfigIncludeDepth = 100

// configYamlDecoder keeps recursive include evaluation within one config read.
// The shared include resolver supplies the correct file context for each subtree.
type configYamlDecoder struct {
	includes map[string]bool
}

// evaluateInclude resolves an included subtree using its file context and
// replaces function tags with plain values to prevent duplicate evaluation.
func (d *configYamlDecoder) evaluateInclude(node *legacyyaml.Node, sourceFile string) error {
	// Bridge the two YAML packages without decoding away custom tags first.
	content, err := legacyyaml.Marshal(node)
	if err != nil {
		return err
	}
	var included yaml.Node
	if err := yaml.Unmarshal(content, &included); err != nil {
		return err
	}
	value, err := d.decode(&included, sourceFile)
	if err != nil {
		return err
	}
	// Encoding the evaluated value clears function tags, so the caller decodes
	// plain data and never evaluates an environment or command result twice.
	return node.Encode(value)
}
