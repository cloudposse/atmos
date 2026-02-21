package yaml

import (
	"bytes"
	"strings"
	"sync"

	goyaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/perf"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// bufferPool reuses bytes.Buffer objects to reduce allocations in YAML encoding.
var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

// ConvertToYAMLPreservingDelimiters converts data to YAML while ensuring that custom template
// delimiter characters are preserved literally in the output. When custom delimiters contain
// single-quote characters, the default yaml.v3 encoder may use single-quoted style for certain
// values (like those starting with '!'), which escapes internal single quotes by doubling them.
// This breaks template processing because the delimiter pattern is altered. This function forces
// double-quoted YAML style for affected scalar values to preserve delimiters.
func ConvertToYAMLPreservingDelimiters(data any, delimiters []string, opts ...u.YAMLOptions) (string, error) {
	defer perf.Track(nil, "yaml.ConvertToYAMLPreservingDelimiters")()

	// If no delimiters or delimiters don't contain single quotes, use standard encoding.
	if !DelimiterConflictsWithYAMLQuoting(delimiters) {
		return u.ConvertToYAML(data, opts...)
	}

	// Marshal Go value to yaml.Node tree so we can control quoting styles.
	var node goyaml.Node
	if err := node.Encode(data); err != nil {
		return "", err
	}

	// Walk the node tree and force double-quoted style for scalar values
	// that contain single quotes (which would conflict with YAML's single-quote escaping).
	EnsureDoubleQuotedForDelimiterSafety(&node)

	// Encode the modified node tree to YAML string.
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()

	defer func() {
		buf.Reset()
		bufferPool.Put(buf)
	}()

	encoder := goyaml.NewEncoder(buf)

	indent := DefaultIndent
	if len(opts) > 0 && opts[0].Indent > 0 {
		indent = opts[0].Indent
	}
	encoder.SetIndent(indent)

	if err := encoder.Encode(&node); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// DelimiterConflictsWithYAMLQuoting checks if any custom delimiter contains a single-quote
// character that would conflict with YAML's single-quoted string escaping.
func DelimiterConflictsWithYAMLQuoting(delimiters []string) bool {
	defer perf.Track(nil, "yaml.DelimiterConflictsWithYAMLQuoting")()

	if len(delimiters) < 2 {
		return false
	}
	return strings.ContainsRune(delimiters[0], '\'') || strings.ContainsRune(delimiters[1], '\'')
}

// EnsureDoubleQuotedForDelimiterSafety recursively walks a yaml.Node tree and forces
// double-quoted style for scalar nodes whose values contain single-quote characters.
// This prevents YAML's single-quote escaping (two consecutive single quotes) from
// interfering with template delimiters that contain single quotes.
func EnsureDoubleQuotedForDelimiterSafety(node *goyaml.Node) {
	defer perf.Track(nil, "yaml.EnsureDoubleQuotedForDelimiterSafety")()

	if node == nil {
		return
	}

	switch node.Kind {
	case goyaml.ScalarNode:
		// Only change scalar nodes that contain single quotes.
		// These are the values that yaml.v3 would single-quote encode, causing doubled
		// single-quote escaping that breaks template delimiters containing single quotes.
		if strings.ContainsRune(node.Value, '\'') {
			node.Style = goyaml.DoubleQuotedStyle
		}
	case goyaml.DocumentNode, goyaml.MappingNode, goyaml.SequenceNode:
		for _, child := range node.Content {
			EnsureDoubleQuotedForDelimiterSafety(child)
		}
	}
}
