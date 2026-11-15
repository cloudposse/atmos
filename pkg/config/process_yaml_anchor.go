package config

import (
	"fmt"

	"go.yaml.in/yaml/v3"
)

// mergeKeyTag is the resolved tag of the YAML merge key "<<".
const mergeKeyTag = "!!merge"

// includeOrigin is the file an include (or an alias of an anchored include) produced its value from.
type includeOrigin struct {
	file   string
	remote bool
}

// originRecord is one origin notification, with its path relative to the node that produced it.
type originRecord struct {
	rel    []string
	file   string
	remote bool
}

// anchorEntry is the one evaluation of an anchored node. Aliases and merge keys that refer to the
// anchor reuse it instead of evaluating the node's function tags again: an `!exec`, `!env`, or
// `!random` behind an anchor runs once, at the anchor, and every alias sees that result.
type anchorEntry struct {
	value   any
	origins []originRecord
	include *includeOrigin
}

// decodeAnchored evaluates an anchored node once and caches the result for its aliases.
func (d *configYamlDecoder) decodeAnchored(node *yaml.Node, sourceFile string) (any, error) {
	entry, err := d.evaluateAnchor(node, sourceFile)
	if err != nil {
		return nil, err
	}
	d.nodeOrigin = entry.include
	return entry.value, nil
}

// decodeAlias returns a copy of the anchor's evaluated value. The anchor is evaluated on first use
// when its definition was not visited yet, for example when it sits outside the decoded subtree.
// The origins the anchor recorded are replayed below the alias's own path, so a path-valued setting
// reached through an alias keeps the file that declared it.
func (d *configYamlDecoder) decodeAlias(node *yaml.Node, sourceFile string) (any, error) {
	if node.Alias == nil {
		return nil, fmt.Errorf("%w: alias %q has no anchor", ErrExecuteYamlFunctions, node.Value)
	}
	entry, err := d.evaluateAnchor(node.Alias, sourceFile)
	if err != nil {
		return nil, err
	}
	d.replayOrigins(entry.origins)
	d.nodeOrigin = entry.include
	return cloneDecodedValue(entry.value), nil
}

// evaluateAnchor evaluates an anchored node at most once per decoder. An alias that refers back to
// its own anchor is a cycle and is rejected.
func (d *configYamlDecoder) evaluateAnchor(node *yaml.Node, sourceFile string) (*anchorEntry, error) {
	if entry, ok := d.anchors[node]; ok {
		return entry, nil
	}
	if d.evaluating[node] {
		return nil, fmt.Errorf("%w: YAML alias cycle at anchor %q", ErrExecuteYamlFunctions, node.Anchor)
	}
	if d.anchors == nil {
		d.anchors = make(map[*yaml.Node]*anchorEntry)
		d.evaluating = make(map[*yaml.Node]bool)
	}
	d.evaluating[node] = true
	defer delete(d.evaluating, node)

	var records []originRecord
	previous := d.origin
	if previous != nil {
		base := len(d.path)
		d.origin = func(path []string, file string, remote bool) {
			if len(path) > base {
				records = append(records, originRecord{rel: append([]string(nil), path[base:]...), file: file, remote: remote})
			}
			previous(path, file, remote)
		}
		defer func() { d.origin = previous }()
	}
	d.nodeOrigin = nil
	value, err := d.decodeNode(node, sourceFile)
	if err != nil {
		return nil, err
	}
	entry := &anchorEntry{value: value, origins: records}
	if node.Kind == yaml.ScalarNode && isIncludeTag(node.Tag) {
		entry.include = d.nodeOrigin
	}
	d.nodeOrigin = nil
	d.anchors[node] = entry
	return entry, nil
}

// replayOrigins reports the origins an anchor recorded, relative to the current path.
func (d *configYamlDecoder) replayOrigins(records []originRecord) {
	if d.origin == nil {
		return
	}
	for _, record := range records {
		path := append(append([]string(nil), d.path...), record.rel...)
		d.origin(path, record.file, record.remote)
	}
}

// isMergeKey reports whether a mapping key is the YAML merge key "<<".
func isMergeKey(key *yaml.Node) bool {
	return key.Kind == yaml.ScalarNode && key.Tag == mergeKeyTag
}

// expandMergeKeys copies the entries named by every "<<" key of the mapping into result. Entries
// the mapping declares itself win over merged ones, and earlier merge sources win over later ones,
// as the YAML merge-key rules require. The merge value is decoded at the mapping's own path, so the
// origins of merged settings are recorded where the merged repository sees them.
func (d *configYamlDecoder) expandMergeKeys(node *yaml.Node, result map[string]interface{}, sourceFile string) error {
	for i := 0; i < len(node.Content); i += 2 {
		if !isMergeKey(node.Content[i]) {
			continue
		}
		sources, err := d.mergeSources(node.Content[i+1], sourceFile)
		if err != nil {
			return err
		}
		for _, source := range sources {
			for key, value := range source {
				if _, exists := result[key]; !exists {
					result[key] = value
				}
			}
		}
	}
	return nil
}

// mergeSources decodes the value of a merge key: a mapping, an alias of one, or a sequence of them.
func (d *configYamlDecoder) mergeSources(value *yaml.Node, sourceFile string) ([]map[string]interface{}, error) {
	nodes := []*yaml.Node{value}
	if value.Kind == yaml.SequenceNode {
		nodes = value.Content
	}
	sources := make([]map[string]interface{}, 0, len(nodes))
	for _, child := range nodes {
		decoded, err := d.decode(child, sourceFile)
		if err != nil {
			return nil, err
		}
		source, ok := decoded.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%w: merge key %q requires a mapping or a sequence of mappings, got %T", ErrExecuteYamlFunctions, "<<", decoded)
		}
		sources = append(sources, source)
	}
	return sources, nil
}

// cloneDecodedValue deep-copies the mappings and sequences of a decoded value, so an alias never
// shares mutable structure with its anchor.
func cloneDecodedValue(value any) any {
	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, child := range typed {
			out[key] = cloneDecodedValue(child)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(typed))
		for index, child := range typed {
			out[index] = cloneDecodedValue(child)
		}
		return out
	default:
		return value
	}
}
