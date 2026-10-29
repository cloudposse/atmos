package starlarksource

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/goccy/go-yaml/lexer"
	"github.com/goccy/go-yaml/token"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/function/yamlsource"
	"github.com/cloudposse/atmos/pkg/perf"
)

const literalTag = "!literal"

// Protect hides Starlark source and explicit literal scalars from Go template evaluation. The returned
// restore function reinserts readable source after all template passes finish.
// YAML without protected source is returned unchanged.
func Protect(input, file string) (string, func(string) string, error) {
	defer perf.Track(nil, "starlarksource.Protect")()

	identity := func(s string) string { return s }
	if !strings.Contains(input, Tag) && !strings.Contains(input, literalTag) && !strings.Contains(input, "atmos-yaml-source:") {
		return input, identity, nil
	}
	replacements := make([]string, 0)
	prefix := "ATMOS_STARLARK_SOURCE_"
	for strings.Contains(input, prefix) {
		prefix += "X"
	}
	// Keep the YAML-aware path for complete documents, including literal
	// containers and anchors. Raw templates need scalar protection before their
	// control actions can produce a YAML document.
	var node *yaml.Node
	if strings.Contains(input, literalTag) {
		node = literalDocument(input)
		if node == nil {
			input = protectLiteralTokens(input, file, prefix, &replacements)
		}
	}
	if node == nil {
		if !strings.Contains(input, Tag) && !strings.Contains(input, "atmos-yaml-source:") {
			return input, strings.NewReplacer(replacements...).Replace, nil
		}
		node = &yaml.Node{}
		if err := yaml.Unmarshal([]byte(input), node); err != nil {
			return "", identity, fmt.Errorf("preserving !starlark source in %s: %w", file, err)
		}
	}
	protectNode(node, file, prefix, &replacements)
	if len(replacements) == 0 {
		return input, identity, nil
	}
	data, err := yaml.Marshal(node)
	if err != nil {
		return "", identity, err
	}
	restore := strings.NewReplacer(replacements...)
	return string(data), restore.Replace, nil
}

func protectNode(node *yaml.Node, file, prefix string, replacements *[]string) {
	if node.Tag == literalTag {
		if node.Kind == yaml.ScalarNode {
			value := (yamlsource.Source{Tag: node.Tag, Argument: node.Value, File: file, Line: node.Line, Column: node.Column}).Encode()
			protectScalar(node, value, prefix, replacements)
		}
		return
	}
	if source, ok := yamlsource.Decode(node.Value); ok && source.Tag == literalTag {
		protectScalar(node, node.Value, prefix, replacements)
		return
	}
	if node.Kind == yaml.ScalarNode && (node.Tag == Tag || (node.Tag == "!!str" && IsEncoded(node.Value))) {
		value := node.Value
		if node.Tag == Tag {
			value = FromNode(node, file).Encode()
		}
		protectScalar(node, value, prefix, replacements)
		return
	}
	for _, child := range node.Content {
		protectNode(child, file, prefix, replacements)
	}
}

func protectScalar(node *yaml.Node, value, prefix string, replacements *[]string) {
	node.Tag, node.Value, node.Style = "!!str", sourcePlaceholder(value, prefix, replacements), 0
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
	return Source{Code: node.Value, File: file, Line: int32(line), Column: node.Column}
}

func sourcePlaceholder(value, prefix string, replacements *[]string) string {
	encoded, _ := json.Marshal(value)
	placeholder := fmt.Sprintf("%s%d_END", prefix, len(*replacements))
	*replacements = append(*replacements, placeholder, string(encoded))
	return placeholder
}

// protectLiteralTokens edits only tagged scalar spans, so template control flow,
// comments, indentation, and ordinary strings retain their original spelling.
func protectLiteralTokens(input, file, prefix string, replacements *[]string) string {
	tokens := lexer.Tokenize(input)
	ends := make(map[*token.Token]int, len(tokens))
	cursor := 0
	for _, item := range tokens {
		// Adjacent token origins can share leading/trailing whitespace.
		leading := len(item.Origin) - len(strings.TrimLeft(item.Origin, " \t\r\n"))
		searchStart := max(0, cursor-leading)
		offset := strings.Index(input[searchStart:], item.Origin)
		if offset < 0 {
			return input
		}
		cursor = searchStart + offset + len(item.Origin)
		ends[item] = cursor
	}
	var output strings.Builder
	copied := 0
	for _, item := range tokens {
		if item.Type != token.TagType || item.Value != literalTag {
			continue
		}
		value := literalScalarToken(item.Next)
		if value == nil {
			continue
		}
		start := ends[item] - len(item.Origin) + strings.Index(item.Origin, literalTag)
		end := ends[value] - len(value.Origin) + len(strings.TrimRight(value.Origin, " \t\r\n"))
		if start < copied || end < start {
			continue
		}
		output.WriteString(input[copied:start])
		source := yamlsource.Source{Tag: literalTag, Argument: value.Value, File: file, Line: item.Position.Line, Column: item.Position.Column}
		output.WriteString(sourcePlaceholder(source.Encode(), prefix, replacements))
		copied = end
	}
	output.WriteString(input[copied:])
	return output.String()
}

func literalScalarToken(value *token.Token) *token.Token {
	if value != nil && (value.Type == token.LiteralType || value.Type == token.FoldedType) {
		value = value.Next
		for value != nil && value.Type == token.CommentType {
			value = value.Next
		}
	}
	if value == nil || value.NextType() == token.MappingValueType {
		return nil
	}
	switch value.Type {
	case token.StringType, token.SingleQuoteType, token.DoubleQuoteType,
		token.NullType, token.BoolType, token.IntegerType, token.BinaryIntegerType,
		token.OctetIntegerType, token.HexIntegerType, token.FloatType, token.InfinityType, token.NanType:
		return value
	default:
		return nil
	}
}

// A decoder must reach EOF: Unmarshal alone accepts an initial flow-style
// template action as a document and silently leaves the remaining input unread.
func literalDocument(input string) *yaml.Node {
	decoder := yaml.NewDecoder(strings.NewReader(input))
	var node, extra yaml.Node
	if decoder.Decode(&node) != nil || !errors.Is(decoder.Decode(&extra), io.EOF) {
		return nil
	}
	return &node
}
