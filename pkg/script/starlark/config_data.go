package starlark

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
	"go.yaml.in/yaml/v3"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	configMaxDepth = 100
	configMaxNodes = 100000
)

// configRecord keeps mapping operations and JSON encoding while giving fields dot access.
// The underlying dictionary and every child are frozen before being returned to scripts.
type configRecord struct{ *starlark.Dict }

func (r *configRecord) Type() string {
	defer perf.Track(nil, "starlark.configRecord.Type")()
	return "config"
}

func (r *configRecord) Attr(name string) (starlark.Value, error) {
	defer perf.Track(nil, "starlark.configRecord.Attr")()

	value, found, err := r.Get(starlark.String(name))
	if !found {
		return nil, err
	}
	return value, err
}

func (r *configRecord) AttrNames() []string {
	defer perf.Track(nil, "starlark.configRecord.AttrNames")()

	names := make([]string, 0, r.Len())
	for _, key := range r.Keys() {
		names = append(names, string(key.(starlark.String)))
	}
	return names
}

type (
	configLocation struct{ line, column int }
	configDocument struct {
		value     starlark.Value
		locations map[string]configLocation
	}
)

type configProblem struct{ path, message string }

func (e *configProblem) Error() string {
	defer perf.Track(nil, "starlark.configProblem.Error")()
	return configFieldPath(e.path) + ": " + e.message
}

func configIssue(path, format string, args ...any) error {
	return &configProblem{path: path, message: fmt.Sprintf(format, args...)}
}

func configPointer(parent, key string) string {
	return parent + "/" + strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
}

func configFieldPath(pointer string) string {
	if pointer == "" {
		return "$"
	}
	parts := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	for i := range parts {
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(parts[i], "~1", "/"), "~0", "~")
	}
	return strings.Join(parts, ".")
}

// YAML nodes preserve source locations for both formats. JSON syntax is checked first
// so a .json file cannot silently accept YAML. Conversion bounds aliases and rejects
// duplicate keys, custom tags, non-string keys, and non-finite numbers.
func parseConfig(ctx context.Context, data []byte, format string) (*configDocument, error) {
	if format == "json" && !json.Valid(data) {
		var value any
		err := json.Unmarshal(data, &value)
		return nil, invalidArg("invalid JSON: %s", err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var root yaml.Node
	if err := decoder.Decode(&root); err != nil {
		return nil, invalidArg("invalid %s: %s", format, err)
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, invalidArg("expected one %s document", format)
	}
	doc := &configDocument{locations: make(map[string]configLocation)}
	budget := configMaxNodes
	value, err := doc.nodeValue(ctx, &root, "", 0, &budget)
	doc.value = value
	return doc, err
}

func (d *configDocument) nodeValue(ctx context.Context, node *yaml.Node, path string, depth int, budget *int) (starlark.Value, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	*budget--
	if depth > configMaxDepth || *budget < 0 {
		return nil, configIssue(path, "configuration exceeds nesting or size limit")
	}
	if node.Kind == yaml.DocumentNode {
		return d.nodeValue(ctx, node.Content[0], path, depth, budget)
	}
	d.locations[path] = configLocation{node.Line, node.Column}
	switch node.Kind {
	case yaml.AliasNode:
		return d.nodeValue(ctx, node.Alias, path, depth+1, budget)
	case yaml.MappingNode:
		return d.mappingValue(ctx, node, path, depth, budget)
	case yaml.SequenceNode:
		return d.sequenceValue(ctx, node, path, depth, budget)
	case yaml.ScalarNode:
		return configScalar(node, path)
	default:
		return nil, configIssue(path, "unsupported YAML node")
	}
}

func (d *configDocument) mappingValue(ctx context.Context, node *yaml.Node, path string, depth int, budget *int) (starlark.Value, error) {
	if node.Tag != "!!map" {
		return nil, configIssue(path, "unsupported YAML tag %s", node.Tag)
	}
	result := starlark.NewDict(len(node.Content) / 2)
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		if key.Tag != "!!str" {
			return nil, configIssue(path, "mapping keys must be strings (line %d)", key.Line)
		}
		field := configPointer(path, key.Value)
		if _, found, _ := result.Get(starlark.String(key.Value)); found {
			d.locations[field] = configLocation{key.Line, key.Column}
			return nil, configIssue(field, "duplicate key")
		}
		value, err := d.nodeValue(ctx, node.Content[i+1], field, depth+1, budget)
		if err != nil {
			return nil, err
		}
		_ = result.SetKey(starlark.String(key.Value), value)
	}
	return result, nil
}

func configScalar(node *yaml.Node, path string) (starlark.Value, error) {
	if number := configInteger(node); number != nil {
		return number, nil
	}
	switch node.Tag {
	case "!!str", "!!timestamp":
		return starlark.String(node.Value), nil
	case "!!null":
		return starlark.None, nil
	case "!!bool":
		return starlark.Bool(strings.EqualFold(node.Value, "true")), nil
	case "!!float":
		var number float64
		if err := node.Decode(&number); err == nil && !math.IsInf(number, 0) && !math.IsNaN(number) {
			return starlark.Float(number), nil
		}
	}
	return nil, configIssue(path, "unsupported scalar %s; use finite numbers and plain data", node.Tag)
}

// yaml's resolver falls back to float or string for integers beyond machine size.
// Recover the exact integer without interpreting quoted strings or explicit tags.
func configInteger(node *yaml.Node) starlark.Value {
	if node.Tag != "!!int" && (node.Style != 0 || (node.Tag != "!!float" && node.Tag != "!!str")) {
		return nil
	}
	number, ok := new(big.Int).SetString(strings.ReplaceAll(node.Value, "_", ""), 0)
	if !ok {
		return nil
	}
	return starlark.MakeBigInt(number)
}

func (d *configDocument) sequenceValue(ctx context.Context, node *yaml.Node, path string, depth int, budget *int) (starlark.Value, error) {
	if node.Tag != "!!seq" {
		return nil, configIssue(path, "unsupported YAML tag %s", node.Tag)
	}
	values := make([]starlark.Value, len(node.Content))
	for i, child := range node.Content {
		value, err := d.nodeValue(ctx, child, configPointer(path, fmt.Sprint(i)), depth+1, budget)
		if err != nil {
			return nil, err
		}
		values[i] = value
	}
	return starlark.NewList(values), nil
}

func (r *configRecord) CompareSameType(op syntax.Token, other starlark.Value, depth int) (bool, error) {
	defer perf.Track(nil, "starlark.configRecord.CompareSameType")()
	return r.Dict.CompareSameType(op, other.(*configRecord).Dict, depth)
}
