package yaml

import (
	"fmt"
	"sort"

	"github.com/cloudposse/atmos/pkg/perf"
	goyaml "gopkg.in/yaml.v3"
)

// PathEntry describes an addressable YAML value discovered while flattening a
// document into Atmos dot-notation paths.
type PathEntry struct {
	Path  string
	Type  string
	Value string
}

// ListPathEntries parses a YAML document and returns all addressable child
// values as sorted Atmos dot-notation paths. Mapping keys are quoted with the
// same rules used by the config/stack/vendor editors.
func ListPathEntries(content []byte) ([]PathEntry, error) {
	defer perf.Track(nil, "yaml.ListPathEntries")()

	var doc goyaml.Node
	if err := goyaml.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidYAMLExpression, err)
	}

	if len(doc.Content) == 0 {
		return nil, nil
	}

	var entries []PathEntry
	collectPathEntries(doc.Content[0], "", &entries)

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})

	return entries, nil
}

func collectPathEntries(node *goyaml.Node, path string, entries *[]PathEntry) {
	if node == nil {
		return
	}

	effectiveNode := node
	if node.Kind == goyaml.AliasNode && node.Alias != nil {
		effectiveNode = node.Alias
	}

	if path != "" {
		*entries = append(*entries, PathEntry{
			Path:  path,
			Type:  yamlPathType(effectiveNode),
			Value: yamlPathValue(effectiveNode),
		})
	}

	switch effectiveNode.Kind {
	case goyaml.MappingNode:
		for i := 0; i+1 < len(effectiveNode.Content); i += 2 {
			key := effectiveNode.Content[i]
			value := effectiveNode.Content[i+1]
			childPath := joinPathSegment(path, QuotePathSegment(key.Value))
			collectPathEntries(value, childPath, entries)
		}
	case goyaml.SequenceNode:
		for i, child := range effectiveNode.Content {
			childPath := fmt.Sprintf("%s[%d]", path, i)
			collectPathEntries(child, childPath, entries)
		}
	}
}

func joinPathSegment(parent, segment string) string {
	if parent == "" {
		return segment
	}
	return parent + "." + segment
}

func yamlPathType(node *goyaml.Node) string {
	switch node.Kind {
	case goyaml.MappingNode:
		return "object"
	case goyaml.SequenceNode:
		return "array"
	case goyaml.ScalarNode:
		switch node.Tag {
		case "!!bool":
			return "bool"
		case "!!int", "!!float":
			return "number"
		case "!!null":
			return "null"
		default:
			return "string"
		}
	default:
		return "string"
	}
}

func yamlPathValue(node *goyaml.Node) string {
	switch node.Kind {
	case goyaml.MappingNode:
		return fmt.Sprintf("{%d keys}", len(node.Content)/2)
	case goyaml.SequenceNode:
		return fmt.Sprintf("[%d items]", len(node.Content))
	case goyaml.ScalarNode:
		if node.Tag == "!!null" {
			return "null"
		}
		return node.Value
	default:
		return ""
	}
}
