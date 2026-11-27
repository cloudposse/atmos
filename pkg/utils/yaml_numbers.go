package utils

import (
	"crypto/rand"
	"encoding/json"

	yaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/yaml/scalar"
)

// decodeVariableNumbers lets yaml.v3 handle aliases, merges, duplicate keys and expansion limits.
// During a dynamic decode it carries otherwise lossy explicit numbers through
// the native decoder as private strings and restores them before returning.
func decodeVariableNumbers[T any](node *yaml.Node, dst *T) error {
	switch any(dst).(type) {
	case *map[string]any, *[]any, *any:
	default:
		return node.Decode(dst)
	}
	if !hasExactNumber(node) {
		return node.Decode(dst)
	}
	// The parsed tree can be shared or cached. Rewrite only this private copy.
	node = deepCopyYAMLNode(node)
	var nodes []*yaml.Node
	collectExactNumbers(node, &nodes)
	prefix := rand.Text()
	numbers := make(map[string]scalar.Number, len(nodes))
	for _, n := range nodes {
		text := n.Value
		n.Value = prefix + text
		numbers[n.Value] = scalar.Number(text)
		n.Tag = "!!str"
	}
	err := node.Decode(dst)
	if err == nil && any(*dst) != nil {
		*dst = restoreNumbers(*dst, numbers).(T)
	}
	return err
}

func hasExactNumber(node *yaml.Node) bool {
	if isExactNumber(node) {
		return true
	}
	for _, child := range node.Content {
		if hasExactNumber(child) {
			return true
		}
	}
	return false
}

func collectExactNumbers(node *yaml.Node, nodes *[]*yaml.Node) {
	if isExactNumber(node) {
		*nodes = append(*nodes, node)
		return
	}
	for i, child := range node.Content {
		if node.Kind != yaml.MappingNode || i%2 == 1 {
			collectExactNumbers(child, nodes)
		}
	}
}

func restoreNumbers(v any, numbers map[string]scalar.Number) any {
	switch value := v.(type) {
	case string:
		if n, ok := numbers[value]; ok {
			return n
		}
	case map[string]any:
		for k, c := range value {
			value[k] = restoreNumbers(c, numbers)
		}
	case map[any]any:
		for k, c := range value {
			value[k] = restoreNumbers(c, numbers)
		}
	case []any:
		for i, c := range value {
			value[i] = restoreNumbers(c, numbers)
		}
	}
	return v
}

// isExactNumber reports whether decoding this explicitly typed numeric scalar into
// native YAML types would alter its JSON value.
func isExactNumber(n *yaml.Node) bool {
	if n.Kind != yaml.ScalarNode || n.Style&yaml.TaggedStyle == 0 || (n.Tag != "!!float" && n.Tag != "!!int") {
		return false
	}
	var number json.Number
	if len(n.Value) == 0 || n.Value[0] == '"' || json.Unmarshal([]byte(n.Value), &number) != nil || number == "" {
		return false
	}
	_, exact := scalar.NativeNumber(number).(scalar.Number)
	return exact
}
