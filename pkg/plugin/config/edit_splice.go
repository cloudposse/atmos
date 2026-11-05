package config

import (
	"strings"

	"gopkg.in/yaml.v3"
)

type locationKind int

const (
	locationFound locationKind = iota
	locationEmptyKey
	locationMissing
)

// location says where the mapping at a path is, or what is missing before it.
type location struct {
	kind      locationKind
	mapping   *yaml.Node // locationFound: the target block mapping.
	owner     *yaml.Node // locationEmptyKey: the key whose value is null or {}; locationFound: the key owning mapping.
	value     *yaml.Node // locationEmptyKey: the null or {} value.
	container *yaml.Node // locationMissing: the mapping that lacks path[depth], or nil for an empty document.
	depth     int
}

// locate walks path through block mappings.
func (d *document) locate(path []string) (location, error) {
	if d.root == nil {
		return location{kind: locationMissing}, nil
	}
	current := d.root
	var owner *yaml.Node
	for depth, segment := range path {
		key, value := lookup(current, segment)
		if key == nil {
			return location{kind: locationMissing, container: current, depth: depth}, nil
		}
		switch {
		case isEmptyValue(value):
			if depth != len(path)-1 {
				return location{kind: locationEmptyKey, owner: key, value: value, depth: depth}, nil
			}
			return location{kind: locationEmptyKey, owner: key, value: value, depth: depth}, nil
		case value.Kind == yaml.MappingNode && value.Style&yaml.FlowStyle == 0 && value.Anchor == "":
			current, owner = value, key
		default:
			return location{}, errUnsupportedLayout
		}
	}
	return location{kind: locationFound, mapping: current, owner: owner}, nil
}

// isEmptyValue matches `key:` and `key: {}`.
func isEmptyValue(value *yaml.Node) bool {
	if value.Anchor != "" {
		return false
	}
	if value.Kind == yaml.ScalarNode {
		return value.Tag == "!!null" && value.Value == "" && value.Style == 0
	}
	return value.Kind == yaml.MappingNode && value.Style&yaml.FlowStyle != 0 && len(value.Content) == 0
}

func spliceFields(content []byte, path []string, fields []Field) ([]byte, error) {
	current := content
	for index := range fields {
		next, err := spliceField(current, path, &fields[index])
		if err != nil {
			return nil, err
		}
		current = next
	}
	return current, nil
}

func spliceField(content []byte, path []string, field *Field) ([]byte, error) {
	current := content
	for range len(path) + 2 {
		doc, err := parseDocument(current)
		if err != nil {
			return nil, err
		}
		place, err := doc.locate(path)
		if err != nil {
			return nil, err
		}
		switch place.kind {
		case locationFound:
			return doc.setInMapping(place.mapping, field)
		case locationEmptyKey:
			done, err := doc.fillEmptyKey(&place, path, field)
			if err != nil {
				return nil, err
			}
			if done {
				return []byte(doc.String()), nil
			}
		default:
			if err := doc.addChain(&place, path); err != nil {
				return nil, err
			}
		}
		current = []byte(doc.String())
	}
	return nil, errUnsupportedLayout
}

// endOfDocument is the 1-based line after which appended top-level lines go.
func (d *document) endOfDocument() int {
	if len(d.lines) > 0 && d.lines[len(d.lines)-1] == "" {
		return len(d.lines) - 1
	}
	return len(d.lines)
}

func (d *document) insertAfter(line int, text ...string) {
	line = min(max(line, 0), len(d.lines))
	d.lines = append(d.lines[:line], append(append([]string{}, text...), d.lines[line:]...)...)
}

// addChain appends the first missing mapping key (as an empty key); locate then descends into it.
func (d *document) addChain(place *location, path []string) error {
	key, err := renderScalar(path[place.depth])
	if err != nil {
		return err
	}
	if place.container == nil {
		d.insertAfter(d.endOfDocument(), key+":")
		return nil
	}
	after, err := lastLine(place.container)
	if err != nil {
		return err
	}
	d.insertAfter(after, strings.Repeat(" ", place.container.Content[0].Column-1)+key+":")
	return nil
}

// fillEmptyKey writes under `key:` or `key: {}`: the field when key is the target, otherwise
// the next missing path segment (done is false and the caller locates again).
func (d *document) fillEmptyKey(place *location, path []string, field *Field) (done bool, err error) {
	line := place.owner.Line - 1
	if place.value.Kind == yaml.MappingNode {
		runes := []rune(d.lines[line])
		start := place.value.Column - 1
		if start+2 > len(runes) || string(runes[start:start+2]) != "{}" {
			return false, errUnsupportedLayout
		}
		d.lines[line] = strings.TrimRight(string(runes[:start]), " \t") + string(runes[start+2:])
	}
	indent := strings.Repeat(" ", place.owner.Column-1+d.step)
	var rendered []string
	if place.depth == len(path)-1 {
		rendered, err = renderField(indent, d.step, field)
		done = true
	} else {
		var next string
		next, err = renderScalar(path[place.depth+1])
		rendered = []string{indent + next + ":"}
	}
	if err != nil {
		return false, err
	}
	d.insertAfter(place.owner.Line, rendered...)
	return done, nil
}

func (d *document) setInMapping(mapping *yaml.Node, field *Field) ([]byte, error) {
	key, value := lookup(mapping, field.Key)
	indent := strings.Repeat(" ", mapping.Content[0].Column-1)
	rendered, err := renderField(indent, d.step, field)
	if err != nil {
		return nil, err
	}
	if key == nil {
		after, err := lastLine(mapping)
		if err != nil {
			return nil, err
		}
		d.insertAfter(after, rendered...)
		return []byte(d.String()), nil
	}
	if field.Items == nil && value.Kind == yaml.ScalarNode && value.Tag != "!!null" {
		if value.Value == field.Value {
			return []byte(d.String()), nil
		}
		return d.replaceScalar(value, field.Value)
	}
	last, err := lastLine(value)
	if err != nil {
		return nil, err
	}
	d.removeLines(key.Line, max(last, key.Line))
	d.insertAfter(key.Line-1, rendered...)
	return []byte(d.String()), nil
}

func (d *document) removeLines(first, last int) {
	d.lines = append(d.lines[:first-1], d.lines[last:]...)
}

// replaceScalar swaps one scalar token and leaves the rest of its line, including any trailing comment, intact.
func (d *document) replaceScalar(node *yaml.Node, value string) ([]byte, error) {
	rendered, err := renderScalar(value)
	if err != nil {
		return nil, err
	}
	line := node.Line - 1
	runes := []rune(d.lines[line])
	start := node.Column - 1
	if node.Anchor != "" || start >= len(runes) {
		return nil, errUnsupportedLayout
	}
	end, err := scalarEnd(runes, start, node)
	if err != nil {
		return nil, err
	}
	d.lines[line] = string(runes[:start]) + rendered + string(runes[end:])
	return []byte(d.String()), nil
}

func scalarEnd(runes []rune, start int, node *yaml.Node) (int, error) {
	switch {
	case node.Style&yaml.DoubleQuotedStyle != 0:
		return quotedScalarEnd(runes, start, '"')
	case node.Style&yaml.SingleQuotedStyle != 0:
		return quotedScalarEnd(runes, start, '\'')
	case node.Style == 0:
		return plainScalarEnd(runes, start, node.Value)
	default:
		return 0, errUnsupportedLayout
	}
}

func quotedScalarEnd(runes []rune, start int, quote rune) (int, error) {
	for index := start + 1; index < len(runes); index++ {
		if quote == '"' && runes[index] == '\\' {
			index++
			continue
		}
		if runes[index] != quote {
			continue
		}
		if quote == '\'' && index+1 < len(runes) && runes[index+1] == quote {
			index++
			continue
		}
		return index + 1, nil
	}
	return 0, errUnsupportedLayout
}

func plainScalarEnd(runes []rune, start int, value string) (int, error) {
	end := len(runes)
	for index := start + 1; index < len(runes); index++ {
		if runes[index] == '#' && (runes[index-1] == ' ' || runes[index-1] == '\t') {
			end = index
			break
		}
	}
	for end > start && strings.ContainsRune(" \t\r", runes[end-1]) {
		end--
	}
	if string(runes[start:end]) != value {
		return 0, errUnsupportedLayout
	}
	return end, nil
}

func spliceDelete(content []byte, path []string) ([]byte, error) {
	doc, err := parseDocument(content)
	if err != nil {
		return nil, err
	}
	if doc.root == nil || len(path) == 0 {
		return content, nil
	}
	place, err := doc.locate(path[:len(path)-1])
	if err != nil {
		return nil, err
	}
	if place.kind != locationFound {
		return content, nil
	}
	key, value := lookup(place.mapping, path[len(path)-1])
	if key == nil {
		return content, nil
	}
	last, err := lastLine(value)
	if err != nil {
		return nil, err
	}
	doc.removeLines(doc.firstCommentLine(key), max(last, key.Line))
	if err := doc.markEmptyParent(&place); err != nil {
		return nil, err
	}
	return []byte(doc.String()), nil
}

// markEmpty writes `{}` after `key:` once its last child is removed.
func (d *document) markEmpty(owner *yaml.Node) error {
	line := owner.Line - 1
	runes := []rune(d.lines[line])
	colon := owner.Column - 1 + len([]rune(owner.Value))
	if owner.Style != 0 || colon >= len(runes) || runes[colon] != ':' {
		return errUnsupportedLayout
	}
	d.lines[line] = string(runes[:colon+1]) + " {}" + string(runes[colon+1:])
	return nil
}

func (d *document) firstCommentLine(key *yaml.Node) int {
	first := key.Line
	if key.HeadComment == "" {
		return first
	}
	for first > 1 && strings.HasPrefix(strings.TrimSpace(d.lines[first-2]), "#") {
		first--
	}
	return first
}

func (d *document) markEmptyParent(place *location) error {
	if len(place.mapping.Content) == 2 && place.owner != nil {
		return d.markEmpty(place.owner)
	}
	return nil
}
