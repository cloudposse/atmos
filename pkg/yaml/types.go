package yaml

import (
	"github.com/cloudposse/atmos/pkg/perf"
	goyaml "gopkg.in/yaml.v3"
)

// DefaultIndent is the default indentation for YAML output.
const DefaultIndent = 2

// Options configures YAML encoding behavior.
type Options struct {
	Indent int
}

// LongString is a string type that encodes as a YAML folded scalar (>).
// This is used to wrap long strings across multiple lines for better readability.
type LongString string

// MarshalYAML implements yaml.Marshaler to encode as a folded scalar.
func (s LongString) MarshalYAML() (interface{}, error) {
	defer perf.Track(nil, "yaml.LongString.MarshalYAML")()

	node := &goyaml.Node{
		Kind:  goyaml.ScalarNode,
		Style: goyaml.FoldedStyle, // Use > style for folded scalar.
		Value: string(s),
	}
	return node, nil
}
