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
	includes     map[string]bool
	path         []string
	origin       func([]string, string, bool)
	remoteSource string

	// anchors holds each anchored node's single evaluation; evaluating guards against alias cycles.
	anchors    map[*yaml.Node]*anchorEntry
	evaluating map[*yaml.Node]bool

	// aliasLimit overrides maxAliasExpansion when set; aliasExpanded is the number of values the
	// aliases decoded so far have copied.
	aliasLimit    int64
	aliasExpanded int64

	// nodeOrigin is the file the include or alias just decoded took its value from. The mapping
	// entry that owns the value reads it so the setting's origin is the included file.
	nodeOrigin *includeOrigin

	// remoteTaint is the first remote include evaluated since it was last cleared. An include whose
	// result is not a mapping (a scalar or a list) has no per-key origin records of its own, so a
	// remote include nested anywhere inside it marks the whole result as remote-supplied.
	remoteTaint *includeOrigin
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
