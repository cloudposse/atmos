package starlarksource

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Protect hides Starlark scalar source from Go template evaluation. The returned
// restore function reinserts readable source after all template passes finish.
// Existing YAML without Starlark is returned unchanged.
func Protect(input, file string) (string, func(string) string, error) {
	defer perf.Track(nil, "starlarksource.Protect")()

	identity := func(s string) string { return s }
	if !strings.Contains(input, Tag) {
		return input, identity, nil
	}
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(input), &node); err != nil {
		return "", identity, fmt.Errorf("preserving !starlark source in %s: %w", file, err)
	}
	replacements := make([]string, 0)
	prefix := "ATMOS_STARLARK_SOURCE_"
	for strings.Contains(input, prefix) {
		prefix += "X"
	}
	protectNode(&node, file, prefix, &replacements)
	if len(replacements) == 0 {
		return input, identity, nil
	}
	data, err := yaml.Marshal(&node)
	if err != nil {
		return "", identity, err
	}
	restore := strings.NewReplacer(replacements...)
	return string(data), restore.Replace, nil
}

func protectNode(node *yaml.Node, file, prefix string, replacements *[]string) {
	if node.Tag == "!literal" {
		return
	}
	if node.Kind == yaml.ScalarNode && (node.Tag == Tag || (node.Tag == "!!str" && IsEncoded(node.Value))) {
		value := node.Value
		if node.Tag == Tag {
			value = FromNode(node, file).Encode()
		}
		encoded, _ := json.Marshal(value)
		token := fmt.Sprintf("%s%d_END", prefix, len(*replacements))
		*replacements = append(*replacements, token, string(encoded))
		node.Tag, node.Value, node.Style = "!!str", token, 0
		return
	}
	for _, child := range node.Content {
		protectNode(child, file, prefix, replacements)
	}
}

// FromNode records the first source line, accounting for YAML's block indicator.
func FromNode(node *yaml.Node, file string) Source {
	defer perf.Track(nil, "starlarksource.FromNode")()

	line := node.Line
	if line < 1 || line >= math.MaxInt32 {
		line = 1
	}
	if node.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
		line++
	}
	if file == "" {
		file = "!starlark"
	}
	return Source{Code: node.Value, File: file, Line: int32(line)}
}
