package templatesource

import (
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var scalarProperty = regexp.MustCompile(`^(?:&[^\s]+|!<[^>]+>|![^\s]*)\s+`)

// Projection can hide every marker in an incomplete control-only scalar.
// Inspect bounded quoted/block source and input-local plain-scalar markers,
// including alias targets, without extending into later flow entries.
func maskedScalarTemplate(source *sourceIndex, node *yaml.Node) bool {
	if node.Kind != yaml.ScalarNode {
		return false
	}
	if node.Style&(yaml.SingleQuotedStyle|yaml.DoubleQuotedStyle|yaml.LiteralStyle|yaml.FoldedStyle) == 0 {
		// Plain scalars do not decode escapes, so an input-absent marker proves
		// this scalar owned a projected action without scanning later flow entries.
		return source.controlMarker != 0 && strings.ContainsRune(node.Value, source.controlMarker)
	}
	var start, end int
	if node.Style&(yaml.SingleQuotedStyle|yaml.DoubleQuotedStyle) != 0 {
		start = quotedScalarStart(source, node)
		if start < 0 {
			return false
		}
		end = quotedEnd(source.text, start)
	} else {
		start, end = scalarSpan(source, node)
	}
	return start >= 0 && end >= start && strings.Contains(source.text[start:end], templateOpening)
}

func quotedScalarStart(source *sourceIndex, node *yaml.Node) int {
	start := scalarStart(source, node)
	if start < 0 {
		return -1
	}
	for start < len(source.text) {
		rest := source.text[start:]
		start += len(rest) - len(strings.TrimLeft(rest, " \t\r\n"))
		if start >= len(source.text) {
			return -1
		}
		if source.text[start] == '#' {
			end := strings.IndexByte(source.text[start:], '\n')
			if end < 0 {
				return -1
			}
			start += end + 1
			continue
		}
		if source.text[start] == '\'' || source.text[start] == '"' {
			return start
		}
		property := scalarProperty.FindStringIndex(source.text[start:])
		if property == nil {
			return -1
		}
		start += property[1]
	}
	return -1
}
