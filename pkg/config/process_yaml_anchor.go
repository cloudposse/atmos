package config

import (
	"fmt"

	"go.yaml.in/yaml/v3"

	errUtils "github.com/cloudposse/atmos/errors"
)

// maxAliasExpansion bounds the values that every alias in one decoded document may copy, counted as
// mappings, sequences, and scalars. An alias copies the whole value of its anchor, so a compact
// document of nested aliases can otherwise expand exponentially (the "billion laughs" shape).
// A million values is far above any real configuration, which aliases a few hundred values at most,
// and keeps the worst-case allocation of a hostile document to tens of megabytes.
const maxAliasExpansion = 1_000_000

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
	// size is the number of values an alias of this anchor copies.
	size int64
	// remote is the first remote include evaluated while the anchor was evaluated.
	remote *includeOrigin
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
	if err := d.chargeAliasExpansion(node.Alias, entry); err != nil {
		return nil, err
	}
	d.replayOrigins(entry.origins)
	d.nodeOrigin = entry.include
	d.noteRemote(entry.remote)
	return cloneDecodedValue(entry.value), nil
}

// chargeAliasExpansion adds the size of an alias's copy to the decoder's budget before the copy is
// made, so an alias that would cross the limit never allocates.
func (d *configYamlDecoder) chargeAliasExpansion(anchor *yaml.Node, entry *anchorEntry) error {
	limit := d.aliasLimit
	if limit == 0 {
		limit = maxAliasExpansion
	}
	if entry.size > limit-d.aliasExpanded {
		return fmt.Errorf("%w: %w: alias %q copies %d values after %d were already copied, over the limit of %d",
			ErrExecuteYamlFunctions, errUtils.ErrYamlAliasExpansionLimit, anchor.Anchor, entry.size, d.aliasExpanded, limit)
	}
	d.aliasExpanded += entry.size
	return nil
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
	outerTaint := d.remoteTaint
	d.remoteTaint = nil
	value, err := d.decodeNode(node, sourceFile)
	entryTaint := d.remoteTaint
	d.remoteTaint = outerTaint
	d.noteRemote(entryTaint)
	if err != nil {
		return nil, err
	}
	entry := &anchorEntry{value: value, origins: records, remote: entryTaint, size: countDecodedValues(value)}
	if node.Kind == yaml.ScalarNode && isIncludeTag(node.Tag) {
		entry.include = d.nodeOrigin
	}
	d.nodeOrigin = nil
	d.anchors[node] = entry
	return entry, nil
}

// noteRemote records the first remote include seen since the taint was last cleared.
func (d *configYamlDecoder) noteRemote(origin *includeOrigin) {
	if origin != nil && origin.remote && d.remoteTaint == nil {
		d.remoteTaint = origin
	}
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

// mergeSource is one decoded merge source and the origins its values were decoded with.
type mergeSource struct {
	value   map[string]interface{}
	origins []originRecord
}

// expandMergeKeys copies the entries named by every "<<" key of the mapping into result. Entries
// the mapping declares itself win over merged ones, and earlier merge sources win over later ones,
// as the YAML merge-key rules require. The origins of a merge source are held back while it is
// decoded and reported only for the keys that are actually copied, each at the mapping's own path,
// so a discarded default never overwrites the origin of an explicit key or of an earlier source.
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
			for key, value := range source.value {
				if _, exists := result[key]; exists {
					continue
				}
				result[key] = value
				d.replayKeyOrigins(source.origins, key)
			}
		}
	}
	return nil
}

// replayKeyOrigins reports the held-back origins of one merged key, relative to the current path.
func (d *configYamlDecoder) replayKeyOrigins(records []originRecord, key string) {
	if d.origin == nil {
		return
	}
	for _, record := range records {
		if len(record.rel) > 0 && record.rel[0] == key {
			d.replayOrigins([]originRecord{record})
		}
	}
}

// mergeSources decodes the value of a merge key: a mapping, an alias of one, or a sequence of them.
// The origins each source reports are collected on the source instead of being forwarded.
func (d *configYamlDecoder) mergeSources(value *yaml.Node, sourceFile string) ([]mergeSource, error) {
	nodes := []*yaml.Node{value}
	if value.Kind == yaml.SequenceNode {
		nodes = value.Content
	}
	sources := make([]mergeSource, 0, len(nodes))
	for _, child := range nodes {
		decoded, records, err := d.decodeCollectingOrigins(child, sourceFile)
		if err != nil {
			return nil, err
		}
		source, ok := decoded.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%w: merge key %q requires a mapping or a sequence of mappings, got %T", ErrExecuteYamlFunctions, "<<", decoded)
		}
		sources = append(sources, mergeSource{value: source, origins: records})
	}
	return sources, nil
}

// decodeCollectingOrigins decodes a node and returns the origins it reported, relative to the
// current path, without forwarding them to the decoder's origin callback.
func (d *configYamlDecoder) decodeCollectingOrigins(node *yaml.Node, sourceFile string) (any, []originRecord, error) {
	previous := d.origin
	if previous == nil {
		value, err := d.decode(node, sourceFile)
		return value, nil, err
	}
	base := len(d.path)
	var records []originRecord
	d.origin = func(path []string, file string, remote bool) {
		if len(path) > base {
			records = append(records, originRecord{rel: append([]string(nil), path[base:]...), file: file, remote: remote})
		}
	}
	defer func() { d.origin = previous }()
	value, err := d.decode(node, sourceFile)
	return value, records, err
}

// countDecodedValues counts the mappings, sequences, and scalars of a decoded value.
func countDecodedValues(value any) int64 {
	count := int64(1)
	switch typed := value.(type) {
	case map[string]interface{}:
		for _, child := range typed {
			count += countDecodedValues(child)
		}
	case []interface{}:
		for _, child := range typed {
			count += countDecodedValues(child)
		}
	}
	return count
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
