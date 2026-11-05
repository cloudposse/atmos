package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	edit "github.com/cloudposse/atmos/pkg/yaml"
)

// Field is one key assigned inside a mapping. A nil Items means Value is a scalar; otherwise
// the key holds a block sequence of scalars.
type Field struct {
	Key   string
	Value string
	Items []string
}

// errUnsupportedLayout reports a document shape the line editor does not handle. Callers fall
// back to the general YAML editor, which keeps semantics but may normalize layout.
var errUnsupportedLayout = errors.New("unsupported YAML layout for in-place editing")

// SetFields assigns keys inside the mapping at path, creating missing mappings in block style.
// It splices bytes rather than re-encoding the document, so comments, blank lines, quoting,
// indentation and inline-comment spacing outside the touched lines are left byte-identical.
// Layouts it cannot edit safely (flow collections on the path, anchors, multiple documents)
// use the general YAML editor instead.
func SetFields(content []byte, path []string, fields []Field) ([]byte, error) {
	defer perf.Track(nil, "pluginconfig.SetFields")()

	result, err := spliceFields(content, path, fields)
	if err == nil {
		err = verifyFields(result, path, fields)
	}
	if err == nil {
		return result, nil
	}
	return fallbackSet(content, path, fields)
}

// DeleteKey removes the last element of path and its head comment. Deleting a missing key is a no-op.
// An emptied parent mapping is kept as an empty flow mapping so the document stays valid.
func DeleteKey(content []byte, path []string) ([]byte, error) {
	defer perf.Track(nil, "pluginconfig.DeleteKey")()

	if len(path) == 0 {
		return nil, fmt.Errorf("%w: delete requires a key path", errUtils.ErrPluginInvalidConfig)
	}
	result, err := spliceDelete(content, path)
	if err == nil {
		err = verifyDeleted(result, path)
	}
	if err == nil {
		return result, nil
	}
	return edit.Delete(content, strings.Join(path, "."))
}

func fallbackSet(content []byte, path []string, fields []Field) ([]byte, error) {
	result := content
	for _, field := range fields {
		var err error
		prefix := strings.Join(path, ".") + "." + field.Key
		if field.Items != nil {
			encoded, marshalErr := json.Marshal(field.Items)
			if marshalErr != nil {
				return nil, marshalErr
			}
			result, err = edit.SetJSON(result, prefix, encoded)
		} else {
			result, err = edit.Set(result, prefix, field.Value)
		}
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// document is a parsed view over line-oriented text.
type document struct {
	lines []string
	eol   string
	root  *yaml.Node
	step  int
}

func parseDocument(content []byte) (*document, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	var parsed yaml.Node
	err := decoder.Decode(&parsed)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, errors.Join(errUtils.ErrPluginInvalidConfig, err)
	}
	if err == nil {
		var extra yaml.Node
		if next := decoder.Decode(&extra); next == nil {
			return nil, errUnsupportedLayout
		}
	}
	text := string(content)
	eol := "\n"
	if strings.Contains(text, "\r\n") {
		eol = "\r\n"
	}
	result := &document{lines: strings.Split(text, eol), eol: eol, step: 2}
	return documentRoot(result, &parsed)
}

func documentRoot(result *document, parsed *yaml.Node) (*document, error) {
	if parsed.Kind != yaml.DocumentNode || len(parsed.Content) != 1 {
		return result, nil
	}
	root := parsed.Content[0]
	if root.Kind == yaml.ScalarNode && root.Tag == "!!null" {
		return result, nil
	}
	if root.Kind != yaml.MappingNode || root.Style&yaml.FlowStyle != 0 || root.Anchor != "" {
		return nil, errUnsupportedLayout
	}
	result.root = root
	result.step = indentStep(root, 2)
	return result, nil
}

//nolint:lintroller // Trivial interface accessor with no independent I/O.
func (d *document) String() string { return strings.Join(d.lines, d.eol) }

// indentStep infers one nesting level from the first nested block mapping.
func indentStep(mapping *yaml.Node, fallback int) int {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		key, value := mapping.Content[index], mapping.Content[index+1]
		if value.Kind == yaml.MappingNode && value.Style&yaml.FlowStyle == 0 && len(value.Content) > 0 {
			if step := value.Content[0].Column - key.Column; step > 0 {
				return step
			}
		}
	}
	return fallback
}

func lookup(mapping *yaml.Node, key string) (keyNode, value *yaml.Node) {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index], mapping.Content[index+1]
		}
	}
	return nil, nil
}

// lastLine returns the final 1-based line occupied by node's content.
func lastLine(node *yaml.Node) (int, error) {
	if node.Anchor != "" || node.Kind == yaml.AliasNode {
		return 0, errUnsupportedLayout
	}
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 || strings.Contains(node.Value, "\n") {
			return 0, errUnsupportedLayout
		}
		return node.Line, nil
	case yaml.MappingNode, yaml.SequenceNode:
		if node.Style&yaml.FlowStyle != 0 {
			return 0, errUnsupportedLayout
		}
		last := node.Line
		for _, child := range node.Content {
			line, err := lastLine(child)
			if err != nil {
				return 0, err
			}
			last = max(last, line)
		}
		return last, nil
	}
	return 0, errUnsupportedLayout
}

func renderScalar(value string) (string, error) {
	if strings.ContainsAny(value, "\n\r") {
		return "", errUnsupportedLayout
	}
	raw, err := yaml.Marshal(value)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(raw), "\n"), nil
}

func renderField(indent string, step int, field *Field) ([]string, error) {
	key, err := renderScalar(field.Key)
	if err != nil {
		return nil, err
	}
	if field.Items == nil {
		value, err := renderScalar(field.Value)
		if err != nil {
			return nil, err
		}
		return []string{indent + key + ": " + value}, nil
	}
	lines := []string{indent + key + ":"}
	for _, item := range field.Items {
		value, err := renderScalar(item)
		if err != nil {
			return nil, err
		}
		lines = append(lines, indent+strings.Repeat(" ", step)+"- "+value)
	}
	return lines, nil
}

func verifyFields(content []byte, path []string, fields []Field) error {
	var parsed map[string]any
	if err := yaml.Unmarshal(content, &parsed); err != nil {
		return err
	}
	var current any = parsed
	for _, segment := range path {
		mapping, ok := current.(map[string]any)
		if !ok {
			return errUnsupportedLayout
		}
		current = mapping[segment]
	}
	mapping, ok := current.(map[string]any)
	if !ok {
		return errUnsupportedLayout
	}
	for _, field := range fields {
		if field.Items != nil {
			if fmt.Sprint(mapping[field.Key]) != fmt.Sprint(toAnySlice(field.Items)) {
				return errUnsupportedLayout
			}
			continue
		}
		if value, ok := mapping[field.Key].(string); !ok || value != field.Value {
			return errUnsupportedLayout
		}
	}
	return nil
}

func toAnySlice(items []string) []any {
	result := make([]any, len(items))
	for index, item := range items {
		result[index] = item
	}
	return result
}

func verifyDeleted(content []byte, path []string) error {
	var parsed map[string]any
	if err := yaml.Unmarshal(content, &parsed); err != nil {
		return err
	}
	var current any = parsed
	for _, segment := range path[:len(path)-1] {
		mapping, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = mapping[segment]
	}
	if mapping, ok := current.(map[string]any); ok {
		if _, present := mapping[path[len(path)-1]]; present {
			return errUnsupportedLayout
		}
	}
	return nil
}
